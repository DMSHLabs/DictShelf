// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Runs the wudict server binary that ships inside the APK as
// lib/arm64-v8a/libwudict.so - named like a library so the package manager
// extracts it to the filesystem (with extractNativeLibs=true) and it can be
// exec'd (D52). This is the same pattern Syncthing-Fork and InviZible Pro
// ship in production; the alternative (JNI) would force an NDK and glue code
// for no benefit to a localhost server.
//
// The binary is the same program `make android-go` cross-compiles; only its
// environment is arranged here. HOME and the prepared library come from
// AppDirs - the app's own external files dir, so that a phone with no root
// can still reach wudict.toml and the db dir (D62). TMPDIR stays on internal
// storage. Which dictionary folders exist is the flavour's decision and
// belongs to Storage, not here.
package com.legbehindneck.wudict;

import android.content.Context;
import android.util.Log;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

class ServerProcess {

    /**
     * The address everything in this shell CONNECTS to. Always loopback, and
     * deliberately not the address the child BINDS to: a platform override can
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
     * The port to TALK to: the one the live child is on, or - when none is up -
     * the one a spawn would use ({@link ShellPrefs#port}).
     *
     * <p>The two differ whenever a stored port has changed under a running app,
     * and the difference is deliberate: a value from the settings takes effect
     * at the NEXT start (that is what every row there says), so until then the
     * server this app can reach is the one it already has, at the port it was
     * exec'd with. Asking the configured port instead sent every window built
     * in that window of time - a lookup popup, a handed-over search, the
     * PowerSignal ping - to an address where nothing was listening yet.
     * {@link #livePort()} is that fact, and this is the rule that reads it.
     * What a spawn WOULD use is asked of {@link ShellPrefs#port} directly: the
     * settings window's own note about the listen address, and the port row's
     * hint on either screen, are sentences about the next start and not about
     * now, so they name the configured one on purpose.
     *
     * <p>Adoption is the other half of the same coin: a child left over from a
     * previous app process is adopted and keeps its port, so an app started
     * after a port change still talks to what is really there.
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

    // The port the LIVE child was actually exec'd with, or 0 when none is up.
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

    private static final String BINARY = "libwudict.so";
    private static final String TAG = "wudict";

    // ── the one child, and who is holding it ─────────────────────────────
    // One server per app process, shared across activity recreation: the child
    // holds this app's port, so a second spawn would lose the port to it.
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
    private static int generation; // invalidates callbacks from a child we stopped

    /**
     * Runs {@code l} against a ready server, starting one if needed. A second
     * caller during STARTING queues rather than exec'ing a second child; READY
     * calls back immediately; a previous FAILED is retried, because the reason
     * (a folder that was not there yet, a port a stale child was still holding)
     * may well be gone by the next attempt.
     *
     * <p>Callbacks arrive on whichever thread settles the start - the
     * activities hop to the main thread themselves, as they must anyway for the
     * adopt path, which answers inline.
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
            if (gen != generation) return; // this child was stopped; nobody is waiting on it
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
    }

    /**
     * A window is gone. {@code mayStop} is the caller's answer to "is this
     * window's own reason for the server ending too?" - MainActivity passes
     * {@code isFinishing()}, the lookup popup always passes false: a popup
     * closing must never stop the server (D67), it drops to
     * {@link PowerSignal#BACKGROUND} instead, so the next lookup from any app
     * is instant. The child is killed only when the last holder says so.
     */
    static synchronized void release(boolean mayStop) {
        if (holders > 0) holders--;
        if (!mayStop || holders > 0) return;
        generation++; // any callback still in flight from this child is now stale
        if (shared != null) shared.stop();
        shared = null;
        livePort = 0; // nothing is listening now
        state = IDLE;
        waiting.clear();
    }

    private final Context app;
    private Process process;
    // written by the log thread, read by the start thread when the child dies
    private volatile String lastLine;
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
        // A previous run's child can outlive the app process: Android kills
        // the app, but an exec'd child is reparented to init and keeps
        // running - still holding this app's port, still serving the same library. Only
        // a clean finish reaches onDestroy and stop(). Spawning a second
        // server then means a bind failure and a misleading wait, when a
        // perfectly good one is already there, so adopt it instead.
        int port = port(app);
        // Primed here, before anything can talk to a server: adoptRunningServer
        // and PowerSignal are static and have no Context of their own, exactly
        // as with the port above.
        ShellPrefs.token(app);
        if (adoptRunningServer(app, port)) {
            Log.i(TAG, "adopted a wudict server already listening on " + port);
            synchronized (this) {
                if (stopped) return;
                livePort = port; // the port the adopted child is really on
            }
            listener.onReady();
            return;
        }

        String bin = app.getApplicationInfo().nativeLibraryDir + "/" + BINARY;
        if (!new File(bin).canExecute()) {
            listener.onFailed("server binary not extracted: " + bin);
            return;
        }

        // Where these live is AppDirs' business (D62: the app's external files
        // dir, so an unrooted phone can still reach the config and the db
        // dir), and WHICH dictionary folders exist is the flavour's (Storage).
        File home = AppDirs.home(app);
        File dbDir = AppDirs.dbDir(app);
        File[] dicts = Storage.dictDirs(app);
        seedConfig(home, dicts);

        ProcessBuilder pb = new ProcessBuilder(bin, "serve",
                "--no-browser",                  // a WebView, not a browser tab
                // The BIND address, which an override may widen to the local
                // network; HOST above stays the connect address regardless.
                "--ip", ShellPrefs.bindIp(app),
                "--port", String.valueOf(port),
                "--db-dir", dbDir.getAbsolutePath(),
                // deliberately NO --dict-dir: see seedConfig
                "--use-cached");                 // list what dbDir already holds
        Map<String, String> env = pb.environment();
        env.put("HOME", home.getAbsolutePath()); // ~/.wudict/wudict.toml lands in app storage
        env.put("TMPDIR", app.getCacheDir().getAbsolutePath());
        // Give freed pages back to the kernel immediately, and be seen to.
        // The Go runtime defaults to MADV_DONTNEED only when GOOS == "linux"
        // (runtime1.go, parseRuntimeDebugVars); GOOS == "android" misses that
        // branch and keeps MADV_FREE, which leaves every reclaimed page counted
        // in RSS until the kernel is short of memory. Measured on device: after
        // shedding a 464k-headword preview backend the Go heap was empty and
        // VmRSS still read 184 MB. On Android the number is the outcome - the
        // low-memory killer, the vendor "RAM hog" watchdogs and the user's own
        // battery screen all read RSS/PSS, so memory we have released but are
        // still charged for is memory we did not release. Costs a page-fault
        // per page on reuse; that is the cheaper side of the trade here.
        env.put("GODEBUG", "madvdontneed=1");
        // Ask the server to announce work a person is waiting on, so this shell
        // can hold a foreground service across it (IndexService). Opt-in per
        // process rather than a config setting: the markers are a private
        // protocol between these two processes and are noise anywhere else.
        env.put("WUDICT_BUSY_LINES", "1");
        // Platform overrides (D101), delivered on the environment because that
        // layer outranks wudict.toml and is truthfully reported as origin
        // "env". The keys are disjoint from the four above by construction -
        // they are wudict config keys, these are process environment - and an
        // override that is not set contributes nothing at all, so an install
        // that has never opened the Advanced section spawns a byte-for-byte
        // identical child.
        env.putAll(ShellPrefs.env(app));
        // The access key, on the environment rather than in argv: every
        // process on the device can read /proc/<pid>/cmdline, and none but
        // this uid can read /proc/<pid>/environ.
        env.putAll(ShellPrefs.authEnv(app));
        pb.directory(home);
        pb.redirectErrorStream(true);
        try {
            synchronized (this) {
                if (stopped) return;
                process = pb.start();
            }
        } catch (IOException e) {
            listener.onFailed(String.valueOf(e.getMessage()));
            return;
        }
        logOutput(process.getInputStream());

        if (awaitPort(app, process, port)) {
            synchronized (this) {
                if (stopped) return;
                livePort = port; // and the one this child was exec'd with
            }
            listener.onReady();
            return;
        }

        // The start failed, and anything the child had announced died with it.
        // The stdout reader cannot do this and neither can stop(), which is not
        // on this path: a child that is killed, or that exits, closes the stream
        // without ever emitting the closing marker, so an ingest that had opened
        // one would leave the foreground service and its notification up for the
        // life of the app - and SettingsActivity's "no ingest in flight" guard
        // false forever with it.
        IndexService.busy(app, false);

        if (process.isAlive()) {
            // It bound nothing, but it is still running and still holds
            // whatever it did open. Left alive it outlives this app process -
            // reparented to init, invisible to stop(), and a later adopt would
            // take it for a healthy server of the current generation - and it
            // denies the retry the very port the retry is about to ask for.
            // A hard kill is safe for the same reason it is in stop():
            // SQLite commits are transactional.
            process.destroy();
            listener.onFailed("port " + port + " never opened");
        } else {
            // The child is gone, so its last line of output is the diagnosis -
            // a bad --db-dir, a port already held, a permission refusal. Saying
            // "port never opened" instead would send the user hunting for a
            // network problem that does not exist.
            listener.onFailed(lastLine == null
                    ? "server exited immediately (" + process.exitValue() + ")"
                    : lastLine);
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
            Log.w(TAG, "could not seed " + cfg, e);
        }
    }

    // adoptRunningServer reports whether a wudict server is already answering
    // on the port. Loopback is shared with every other app on the device, so
    // an open socket is not proof: /api/config must name this app's library.
    // process stays null, so stop() will not kill something this instance
    // never started.
    private static boolean adoptRunningServer(Context app, int port) {
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(
                    "http://" + HOST + ":" + port + "/api/config").openConnection();
            c.setConnectTimeout(700);
            c.setReadTimeout(700);
            // A 401 here means a wudict IS there and holds a different key -
            // this install's switch was toggled while it ran, or another app
            // shipped one. Not ours to adopt and not ours to kill: it is
            // reported as "not us", which leaves the bind failure below to say
            // the port is taken.
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

    // The server's out-of-band markers, read off the same stream as its log
    // (WUDICT_BUSY_LINES above). "1" means an ingest a person is waiting on has
    // started, "0" that the last one finished; they are refcounted on the
    // server side, so this sees one of each and not one per dictionary.
    private static final String BUSY_ON = "@wudict busy 1";
    private static final String BUSY_OFF = "@wudict busy 0";

    private void logOutput(InputStream in) {
        Thread t = new Thread(() -> {
            try (BufferedReader r = new BufferedReader(new InputStreamReader(in))) {
                for (String line; (line = r.readLine()) != null; ) {
                    Log.d(TAG, line);
                    String marker = line.trim();
                    if (BUSY_ON.equals(marker) || BUSY_OFF.equals(marker)) {
                        IndexService.busy(app, BUSY_ON.equals(marker));
                        continue; // a marker is not a diagnosis; keep lastLine
                    }
                    if (!marker.isEmpty()) lastLine = line; // not isBlank(): API 35+
                }
            } catch (IOException ignored) {
                // process ended; nothing more to log
            }
        }, "wudict-log");
        t.setDaemon(true);
        t.start();
    }

    // Waits for the server to accept a connection. Watching the child as well
    // as the port matters: a server that dies on startup - the common failure,
    // since it is the run that creates the config and the library folders -
    // would otherwise hold the "Starting…" screen for the full minute before
    // reporting the wrong thing.
    private static boolean awaitPort(Context app, Process child, int port) {
        long deadline = System.nanoTime() + 60_000_000_000L; // 60 s: first run writes its config
        while (System.nanoTime() < deadline) {
            // A second installation may already own this port. A socket alone
            // does not establish that our child finished starting.
            if (!child.isAlive()) return false;
            if (adoptRunningServer(app, port) && child.isAlive()) return true;
            try {
                Thread.sleep(200);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return false;
            }
        }
        return false;
    }

    private synchronized void stop() {
        stopped = true;
        // Whatever it was preparing died with it, so the notification and the
        // service holding the app up must go with it too. Doing this from the
        // stdout reader instead would not be enough: a killed child closes the
        // stream without ever emitting the closing marker.
        IndexService.busy(app, false);
        Process p = process;
        if (p != null) {
            // No graceful shutdown: Java's destroy() is a hard kill. That is
            // safe here - SQLite commits are transactional, so the library
            // cannot be corrupted by it.
            p.destroy();
        }
    }

    /**
     * Stops the server whoever started it, and reports whether anything was
     * there to stop. This is the one operation the ordinary lifecycle cannot
     * do: a child reparented to init and later ADOPTED has {@code process ==
     * null}, so {@link #stop()} would find nothing to kill and the settings
     * screen could only say "next time".
     *
     * <p>The caller is responsible for the guard - no live windows, no ingest
     * in flight; see SettingsActivity. A hard kill is safe for the same reason
     * it is in {@link #stop()}: SQLite commits are transactional.
     */
    static synchronized boolean stopAny(Context ctx) {
        generation++;   // any callback still in flight from this child is stale
        boolean owned = shared != null && shared.process != null;
        if (shared != null) shared.stop();
        shared = null;
        livePort = 0;
        state = IDLE;
        waiting.clear();
        if (owned) return true;
        IndexService.busy(ctx, false);
        return killAdopted(ctx);
    }

    /** Reads demanded work even when the server was adopted without stdout. */
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

    /**
     * Kills a server this app process never started, by finding it in /proc.
     * It is our own uid, so a {@code hidepid} mount hides nothing of ours, and
     * matching the full cmdline - not just the pid - makes pid reuse harmless:
     * the only process that can match is one exec'd from our own APK.
     */
    private static boolean killAdopted(Context ctx) {
        String bin = ctx.getApplicationInfo().nativeLibraryDir + "/" + BINARY;
        File[] entries = new File("/proc").listFiles();
        if (entries == null) return false;
        boolean killed = false;
        for (File e : entries) {
            int pid;
            try {
                pid = Integer.parseInt(e.getName());
            } catch (NumberFormatException notAPid) {
                continue;
            }
            if (pid == android.os.Process.myPid()) continue;
            byte[] raw;
            try {
                raw = Files.readAllBytes(new File(e, "cmdline").toPath());
            } catch (IOException | RuntimeException gone) {
                continue; // the process ended, or is not ours to read
            }
            // cmdline is NUL-separated; the first field is argv[0].
            String all = new String(raw, StandardCharsets.UTF_8);
            int nul = all.indexOf('\0');
            String argv0 = nul < 0 ? all : all.substring(0, nul);
            if (!argv0.equals(bin)) continue;
            android.os.Process.killProcess(pid);
            killed = true;
        }
        return killed;
    }
}
