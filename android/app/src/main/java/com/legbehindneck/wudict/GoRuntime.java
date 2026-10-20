// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Starts the shared Go runtime in this app process. The WebView and app
// operations reach it through JNI; Android does not open an HTTP listener.
package com.legbehindneck.wudict;

import android.content.Context;
import android.util.Log;

import org.json.JSONObject;

import java.io.File;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.List;

class GoRuntime {
    /** Whether the last runtime start failed and nobody has retried it. */
    static synchronized boolean failed() {
        return state == FAILED;
    }

    interface Listener {
        void onReady();
        void onFailed(String message);
    }

    private static final String TAG = "wudict";

    // ── the one runtime, and who is holding it ───────────────────────────
    // One Go runtime per app process, shared across activity recreation.
    //
    // Not a static inside MainActivity read back as "non-null means ready": a
    // lookup popup must work with MainActivity dead (D67), and two activities
    // can ask at the same time - during which non-null means *starting*. So
    // the field lives here as a state machine with a waiting list, and a start
    // in flight is joined rather than duplicated.
    private static final int IDLE = 0, STARTING = 1, READY = 2, FAILED = 3;

    private static int state = IDLE;
    private static GoRuntime shared;
    private static final List<Listener> waiting = new ArrayList<>();
    private static int holders;    // live windows retaining the Go runtime
    private static int generation; // invalidates callbacks from a runtime we stopped
    private static boolean stopWhenIdle;

    /**
     * Runs {@code l} against a ready runtime, starting one if needed. A second
     * caller during STARTING queues rather than starting a second runtime; READY
     * calls back immediately; a previous FAILED is retried, because the reason
     * (a folder that was not there yet)
     * may well be gone by the next attempt.
     *
     * <p>Callbacks arrive on whichever thread settles the start - the
     * activities hop to the main thread themselves, as they must anyway for the
     * ready path, which answers inline.
     */
    static synchronized void ensure(Context ctx, Listener l) {
        if (state == READY) {
            l.onReady();
            return;
        }
        waiting.add(l);
        if (state == STARTING) return;
        state = STARTING;
        final int gen = generation;
        shared = new GoRuntime(ctx.getApplicationContext());
        shared.start(new Listener() {
            @Override public void onReady() { settle(gen, READY, null); }
            @Override public void onFailed(String message) { settle(gen, FAILED, message); }
        });
    }

    private static void settle(int gen, int newState, String message) {
        List<Listener> pending;
        synchronized (GoRuntime.class) {
            if (gen != generation) return; // this runtime was stopped
            state = newState;
            pending = new ArrayList<>(waiting);
            waiting.clear();
        }
        // Outside the lock: a listener runs activity code, and holding the
        // class monitor across it would let a UI callback block the next
        // ensure().
        for (Listener l : pending) {
            if (newState == READY) l.onReady();
            else l.onFailed(message);
        }
    }

    /** How many windows are currently using the Go runtime. */
    static synchronized int holders() {
        return holders;
    }

    /** A window that needs the runtime is alive. Paired with {@link #release}. */
    static synchronized void retain() {
        holders++;
        stopWhenIdle = false;
    }

    /**
     * A window is gone. {@code mayStop} is the caller's answer to "is this
     * window's own reason for the runtime ending too?" - MainActivity passes
     * {@code isFinishing()}, the lookup popup always passes false: a popup
     * closing must never stop the runtime (D67), it drops to
     * {@link PowerSignal#BACKGROUND} instead, so the next lookup from any app
     * is instant. The runtime stops only when the last holder says so.
     */
    static synchronized void release(boolean mayStop) {
        if (holders > 0) holders--;
        if (!mayStop || holders > 0) return;
        // A task swipe finishes its activities just like an explicit exit.
        // Keep the Go runtime alive while its foreground service protects
        // work, then stop it when the final busy marker (or import hold) ends.
        if (IndexService.hasWork()) {
            stopWhenIdle = true;
            return;
        }
        stopRuntime();
    }

    static synchronized void workFinished() {
        if (!stopWhenIdle || holders > 0 || IndexService.hasWork()) return;
        stopRuntime();
    }

    private static void stopRuntime() {
        stopWhenIdle = false;
        generation++; // any callback still in flight is now stale
        boolean nativeRuntime = shared != null && shared.nativeOwned;
        if (shared != null) shared.stop();
        shared = null;
        state = IDLE;
        waiting.clear();
        if (nativeRuntime) android.os.Process.killProcess(android.os.Process.myPid());
    }

    static synchronized boolean nativeRuntimeActive() {
        return shared != null && shared.nativeOwned;
    }

    private final Context app;
    private boolean nativeOwned;
    private boolean stopped;

    private GoRuntime(Context app) {
        this.app = app;
    }

    private void start(Listener listener) {
        Thread t = new Thread(() -> run(listener), "wudict-runtime");
        t.setDaemon(true);
        t.start();
    }

    private void run(Listener listener) {
        GoBridge.initialize(app);
        SystemLog.record("Go runtime startup requested");
        int bridgeProtocol = GoBridge.protocol();
        SystemLog.record("Go bridge protocol=" + bridgeProtocol);
        Log.i(TAG, "Go bridge protocol=" + bridgeProtocol);
        if (bridgeProtocol != 1) {
            listener.onFailed("Go bridge is unavailable");
            return;
        }

        // Where these live is AppDirs' business (D62: the app's external files
        // dir, so an unrooted phone can still reach the config and the db
        // dir), and WHICH dictionary folders exist is the flavour's (Storage).
        File home = AppDirs.home(app);
        File dbDir = AppDirs.dbDir(app);
        File[] dicts = Storage.dictDirs(app);
        seedConfig(home, dicts);
        GoBridge.checkConfig(home, dbDir, ShellPrefs.env(app));

        int result = GoBridge.startRuntime(app, home, dbDir);
        if (result != 1) {
            SystemLog.record("in-process Go startup failed code=" + result);
            Log.e(TAG, "in-process Go startup failed code=" + result);
            listener.onFailed("in-process Go startup failed (" + result + ")");
            return;
        }
        nativeOwned = true;
        if (runtimeReady(app)) {
            Shell.prepareOrigin(app);
            SystemLog.record("in-process Go runtime ready");
            Log.i(TAG, "in-process Go runtime ready");
            synchronized (this) {
                if (stopped) return;
            }
            PowerSignal.runtimeReady();
            listener.onReady();
        } else {
            GoBridge.stopRuntime();
            IndexService.busy(app, false);
            Log.e(TAG, "in-process Go runtime did not become ready");
            listener.onFailed("in-process Go runtime did not become ready");
        }
    }

    // seedConfig writes the dictionary folders into ~/.wudict/wudict.toml -
    // HOME is the app's own directory (AppDirs) - instead of passing
    // --dict-dir. The distinction is not cosmetic: a flag is the
    // HIGHEST config layer, so Config.EditableInFile("DICT_DIR") would be
    // false and the ☰ panel would (correctly) refuse to change the folders,
    // on the one platform that has no command line to change them from. Coming
    // from the file, the panel owns them.
    //
    // Written once, only when nothing is there: after that the file is the
    // user's. Paths are device-generated (/storage/emulated/0/…, /data/…) so
    // they need no TOML escaping.
    //
    // Consequence worth stating, since Storage.dictDirs can now answer with a
    // microSD card's folder as well: the volumes present at FIRST launch are
    // the ones seeded. A card inserted later is not added behind the user's
    // back - doing so would mean the shell parsing and rewriting a config file
    // that belongs to the user, in Java, to re-implement what the ☰ panel and
    // /setup already do. Its folder is app-specific external storage, so it is
    // readable in every flavour with no permission: the user pastes the path
    // once, and that is the same act as any other folder they add.
    private static void seedConfig(File home, File[] dicts) {
        File cfg = new File(new File(home, ".wudict"), "wudict.toml");
        if (cfg.exists()) return;
        StringBuilder list = new StringBuilder();
        for (File d : dicts) {
            if (list.length() > 0) list.append(", ");
            list.append('"').append(d.getAbsolutePath()).append('"');
        }
        String toml = "# WuWeiDict - written by the Android shell on first launch.\n"
                + "# Priority: CLI flag > environment variable > this file > default.\n"
                + "# The app passes no --dict-dir, so the ☰ panel can edit this.\n"
                + "\n"
                + "DICT_DIR = [" + list + "]\n";
        try {
            Files.createDirectories(cfg.getParentFile().toPath());
            Files.write(cfg.toPath(), toml.getBytes(StandardCharsets.UTF_8));
        } catch (IOException e) {
            // Not fatal: the runtime falls back to $HOME/Dictionaries, which is
            // appPrivate - reachable, just without the shared folder.
            SystemLog.warn(TAG, "could not seed " + cfg, e);
        }
    }

    // RuntimeStart returns only after the shared Go runtime and library exist.
    // Read its snapshot over JNI so startup does not depend on a TCP listener.
    private static boolean runtimeReady(Context app) {
        try {
            JSONObject config = new JSONObject(GoBridge.configSnapshot());
            String libDir = config.optString("libDir", "");
            return !libDir.isEmpty() && new File(libDir).getCanonicalFile()
                    .equals(AppDirs.dbDir(app).getCanonicalFile());
        } catch (IOException | org.json.JSONException | RuntimeException e) {
            return false; // runtime startup did not publish a usable Go snapshot
        }
    }

    private synchronized void stop() {
        stopped = true;
        IndexService.busy(app, false);
        if (nativeOwned) GoBridge.stopRuntime();
        nativeOwned = false;
    }

    /** Stops the in-process runtime; the caller handles process restart. */
    static synchronized boolean stopAny(Context ctx) {
        stopWhenIdle = false;
        generation++;   // any callback still in flight from this child is stale
        boolean owned = shared != null && shared.nativeOwned;
        if (shared != null) shared.stop();
        shared = null;
        state = IDLE;
        waiting.clear();
        IndexService.busy(ctx, false);
        return owned;
    }

}
