// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Starts the shared Go runtime in this app process. The WebView currently
// reaches it through a local HTTP listener; the runtime itself is a JNI library.
package com.legbehindneck.wudict;

import android.content.Context;
import android.util.Log;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

class ServerProcess {

    /**
     * The address everything in this shell CONNECTS to. Always loopback, and
     * deliberately not the address Go BINDS to: a platform override can
     * open the server to the local network (ShellPrefs.bindIp), and you connect
     * to 127.0.0.1 either way. Keeping the two apart is what stops that
     * override from moving the WebView's origin and orphaning the page prefs
     * saved against it.
     */
    static final String HOST = ShellPrefs.DEFAULT_IP;

    // The port is fixed per install rather than per launch (D52: UI prefs live
    // in localStorage, which is keyed by origin, so a random port would forget
    // them every time), but it is no longer a constant: a device where
    // something else already holds the port can override it (D101).
    //
    // portCache exists for PowerSignal, which is entirely static and has no
    // Context to ask. Every path that can reach the server primes it first -
    // ensure() does, and nothing talks to a server it never asked for - so the
    // worst case is a POST to the default port that nobody answers, which
    // PowerSignal already treats as an ordinary miss.
    private static volatile int portCache = ShellPrefs.DEFAULT_PORT;

    /**
     * The port to TALK to: the live runtime's port, or the configured port.
     *
     * <p>The two differ whenever a stored port has changed under a running app,
     * and the difference is deliberate: a value from the settings takes effect
     * at the NEXT start (that is what every row there says), so until then the
     * server this app can reach is the one it already has, at the port it was
     * started with. Asking the configured port instead sent every window built
     * in that window of time - a lookup popup, a handed-over search, the
     * PowerSignal ping - to an address where nothing was listening yet.
     * {@link #livePort()} is that fact, and this is the rule that reads it.
     * What the next start WOULD use is asked of {@link ShellPrefs#port} directly: the
     * settings window's own note about the listen address, and the port row's
     * hint on either screen, are sentences about the next start and not about
     * now, so they name the configured one on purpose.
     *
     */
    static int port(Context c) {
        int live = livePort;
        if (live != 0) {
            portCache = live;
            return live;
        }
        int p = ShellPrefs.port(c);
        portCache = p;
        return p;
    }

    /** The last port {@link #port(Context)} resolved. For callers with no Context. */
    static int port() {
        return portCache;
    }

    /**
     * Whether the last start attempt failed and nobody has retried it. Read by
     * MainActivity, which retries on a focus gain: the reader has been to the
     * settings screen (a port, a memory cap) or has freed whatever held the
     * port - and nothing else asks again, because ensure() runs in onCreate
     * alone and that activity is singleTask with its config changes
     * intercepted, so returning to it does not recreate it.
     */
    static synchronized boolean failed() {
        return state == FAILED;
    }

    // The live Go runtime's port, or 0 when none is up.
    // Read by port(Context) above, which is the rule that matters, and by the
    // settings screen's probe - a probe of the CONFIGURED port finds nothing
    // there, so the footer would go silent in exactly the case it exists for
    // (found on the emulator, 2026-09-28, with the server on 7003 and the row
    // already saying 7004). The page's own System window has no such problem:
    // it asks the server it is being served by.
    private static volatile int livePort;

    /** The port the running server listens on, or 0 when none is running. */
    static int livePort() {
        return livePort;
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
    private static ServerProcess shared;
    private static final List<Listener> waiting = new ArrayList<>();
    private static int holders;    // live windows retaining the server
    private static int generation; // invalidates callbacks from a runtime we stopped
    private static boolean stopWhenIdle;

    /**
     * Runs {@code l} against a ready server, starting one if needed. A second
     * caller during STARTING queues rather than starting a second runtime; READY
     * calls back immediately; a previous FAILED is retried, because the reason
     * (a folder that was not there yet, or a port another app held)
     * may well be gone by the next attempt.
     *
     * <p>Callbacks arrive on whichever thread settles the start - the
     * activities hop to the main thread themselves, as they must anyway for the
     * ready path, which answers inline.
     */
    static synchronized void ensure(Context ctx, Listener l) {
        port(ctx); // prime portCache for the Context-less callers
        if (state == READY) {
            l.onReady();
            return;
        }
        waiting.add(l);
        if (state == STARTING) return;
        state = STARTING;
        final int gen = generation;
        shared = new ServerProcess(ctx.getApplicationContext());
        shared.start(new Listener() {
            @Override public void onReady() { settle(gen, READY, null); }
            @Override public void onFailed(String message) { settle(gen, FAILED, message); }
        });
    }

    private static void settle(int gen, int newState, String message) {
        List<Listener> pending;
        synchronized (ServerProcess.class) {
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

    /** How many windows are currently holding the server open. */
    static synchronized int holders() {
        return holders;
    }

    /** A window that needs the server is alive. Paired with {@link #release}. */
    static synchronized void retain() {
        holders++;
        stopWhenIdle = false;
    }

    /**
     * A window is gone. {@code mayStop} is the caller's answer to "is this
     * window's own reason for the server ending too?" - MainActivity passes
     * {@code isFinishing()}, the lookup popup always passes false: a popup
     * closing must never stop the server (D67), it drops to
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
        stopOwnedServer();
    }

    static synchronized void workFinished() {
        if (!stopWhenIdle || holders > 0 || IndexService.hasWork()) return;
        stopOwnedServer();
    }

    private static void stopOwnedServer() {
        stopWhenIdle = false;
        generation++; // any callback still in flight is now stale
        boolean nativeRuntime = shared != null && shared.nativeOwned;
        if (shared != null) shared.stop();
        shared = null;
        livePort = 0; // nothing is listening now
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

    private ServerProcess(Context app) {
        this.app = app;
    }

    private void start(Listener listener) {
        Thread t = new Thread(() -> run(listener), "wudict-server");
        t.setDaemon(true);
        t.start();
    }

    private void run(Listener listener) {
        GoBridge.initialize(app);
        SystemLog.record("server startup requested");
        int bridgeProtocol = GoBridge.protocol();
        SystemLog.record("Go bridge protocol=" + bridgeProtocol);
        Log.i(TAG, "Go bridge protocol=" + bridgeProtocol);
        int port = port(app);
        ShellPrefs.token(app);
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

        int result = GoBridge.startRuntime(app, home, dbDir, port);
            if (result != 1) {
                SystemLog.record("in-process Go startup failed code=" + result);
                Log.e(TAG, "in-process Go startup failed code=" + result);
                listener.onFailed("in-process Go startup failed (" + result + ")");
                return;
            }
            nativeOwned = true;
            if (awaitNative(app, port)) {
                SystemLog.record("in-process Go ready port=" + port);
                Log.i(TAG, "in-process Go ready port=" + port);
                synchronized (this) {
                    if (stopped) return;
                    livePort = port;
                }
                PowerSignal.runtimeReady();
                listener.onReady();
            } else {
                GoBridge.stopRuntime();
                IndexService.busy(app, false);
                Log.e(TAG, "in-process Go did not become ready on port " + port);
                listener.onFailed("in-process Go did not become ready on port " + port);
            }
            return;
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
    // user's, and the server's own first-run template step finds it and leaves
    // it alone. Paths are device-generated (/storage/emulated/0/…, /data/…) so
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
            // Not fatal: the server falls back to $HOME/Dictionaries, which is
            // appPrivate - reachable, just without the shared folder.
            SystemLog.warn(TAG, "could not seed " + cfg, e);
        }
    }

    // Readiness requires a response from this app's own library.
    private static boolean runtimeReady(Context app, int port) {
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(
                    "http://" + HOST + ":" + port + "/api/config").openConnection();
            c.setConnectTimeout(700);
            c.setReadTimeout(700);
            // A 401 here means a wudict IS there and holds a different key -
            // this install's switch was toggled while it ran, or another app
            // shipped one. It is reported as unavailable; startup reports
            // the port conflict separately.
            ShellPrefs.authorize(c);
            if (c.getResponseCode() != 200) return false;
            StringBuilder body = new StringBuilder();
            try (BufferedReader r = new BufferedReader(
                    new InputStreamReader(c.getInputStream(), StandardCharsets.UTF_8))) {
                for (String line; (line = r.readLine()) != null; ) body.append(line);
            }
            JSONObject config = new JSONObject(body.toString());
            String libDir = config.optString("libDir", "");
            return !libDir.isEmpty() && new File(libDir).getCanonicalFile()
                    .equals(AppDirs.dbDir(app).getCanonicalFile());
        } catch (IOException | org.json.JSONException | RuntimeException e) {
            return false; // nothing listening, or not us
        } finally {
            if (c != null) c.disconnect();
        }
    }

    /**
     * Fills {@code values} and {@code origins} from /api/config's "effective"
     * map; false when nothing answered, or answered without one - an older
     * server, which is a silence rather than an error.
     */
    static boolean fetchEffective(int port, Map<String, String> values,
                                  Map<String, String> origins) {
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(
                    "http://" + HOST + ":" + port + "/api/config").openConnection();
            c.setConnectTimeout(700);
            c.setReadTimeout(700);
            ShellPrefs.authorize(c);
            if (c.getResponseCode() != 200) return false;
            StringBuilder body = new StringBuilder();
            try (BufferedReader r = new BufferedReader(
                    new InputStreamReader(c.getInputStream(), StandardCharsets.UTF_8))) {
                for (String line; (line = r.readLine()) != null; ) body.append(line);
            }
            JSONObject eff = new JSONObject(body.toString()).optJSONObject("effective");
            if (eff == null) return false;
            for (ShellPrefs.Override o : ShellPrefs.OVERRIDES) {
                JSONObject e = eff.optJSONObject(o.key);
                if (e == null) continue;
                values.put(o.key, e.optString("value"));
                origins.put(o.key, e.optString("origin"));
            }
            return true;
        } catch (IOException | org.json.JSONException | RuntimeException e) {
            return false; // nothing listening, or not answering like us
        } finally {
            if (c != null) c.disconnect();
        }
    }

    private static boolean awaitNative(Context app, int port) {
        long deadline = System.nanoTime() + 10_000_000_000L;
        while (System.nanoTime() < deadline) {
            if (runtimeReady(app, port)) return true;
            try {
                Thread.sleep(100);
            } catch (InterruptedException error) {
                Thread.currentThread().interrupt();
                return false;
            }
        }
        return false;
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
        livePort = 0;
        state = IDLE;
        waiting.clear();
        IndexService.busy(ctx, false);
        return owned;
    }

    /** Reads demanded work from the in-process Go runtime. */
    static boolean workInFlight() {
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL("http://" + HOST + ":" + port() + "/api/power").openConnection();
            ShellPrefs.authorize(c);
            c.setConnectTimeout(900);
            c.setReadTimeout(900);
            if (c.getResponseCode() != 200) return IndexService.hasWork();
            try (BufferedReader r = new BufferedReader(new InputStreamReader(c.getInputStream(), StandardCharsets.UTF_8))) {
                return new JSONObject(r.readLine()).optBoolean("busy");
            }
        } catch (Exception unavailable) {
            return IndexService.hasWork();
        } finally {
            if (c != null) c.disconnect();
        }
    }

}
