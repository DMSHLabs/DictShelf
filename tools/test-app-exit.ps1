# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later

# Compile the real exit coordinator against a small Android lifecycle double.
# Requires javac/java on PATH; no emulator or APK installation.
param([string]$SourceFile = (Join-Path $PSScriptRoot '../android/app/src/main/java/com/legbehindneck/wudict/AppExit.java'))

$ErrorActionPreference = 'Stop'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('wudict-exit-' + [guid]::NewGuid())
$null = New-Item -ItemType Directory -Path $testRoot
$sources = @{
    'android/os/Bundle.java' = 'package android.os; public class Bundle {}'
    'android/os/Looper.java' = 'package android.os; public class Looper { public static Looper getMainLooper() { return new Looper(); } }'
    'android/os/Handler.java' = @'
package android.os;
import java.util.concurrent.*;
public class Handler {
    static final BlockingQueue<Runnable> queue = new LinkedBlockingQueue<>();
    public Handler(Looper l) {}
    public void post(Runnable r) { queue.add(r); }
    public void postDelayed(Runnable r, long delay) { queue.add(r); }
    public static void next() throws Exception {
        Runnable r = queue.poll(5, TimeUnit.SECONDS);
        if (r == null) throw new AssertionError("missing main-thread callback");
        r.run();
    }
}
'@
    'android/app/Application.java' = @'
package android.app;
import android.os.Bundle;
public class Application {
    public void onCreate() {}
    public void registerActivityLifecycleCallbacks(ActivityLifecycleCallbacks c) {}
    public interface ActivityLifecycleCallbacks {
        void onActivityCreated(Activity a, Bundle b);
        void onActivityDestroyed(Activity a);
        void onActivityStarted(Activity a);
        void onActivityResumed(Activity a);
        void onActivityPaused(Activity a);
        void onActivityStopped(Activity a);
        void onActivitySaveInstanceState(Activity a, Bundle b);
    }
}
'@
    'android/app/Activity.java' = @'
package android.app;
public class Activity {
    public Application app;
    public int task;
    public boolean finished;
    public Application getApplication() { return app; }
    public int getTaskId() { return task; }
    public void finish() { finished = true; }
    public boolean isFinishing() { return finished; }
    public boolean isDestroyed() { return false; }
}
'@
    'com/legbehindneck/wudict/Doubles.java' = @'
package com.legbehindneck.wudict;
import android.app.*;
class MainActivity extends Activity {}
class SystemLog { static void initialize(Application a) {} }
class ProcessExitDiagnostics { static void recordRecent(Application a) {} }
class ServerProcess {
    static int stops;
    static boolean busy;
    static boolean workInFlight() { return busy; }
    static void stopAny(Application a) { stops++; }
}
class IndexService { static boolean hasWork() { return false; } }
class R { static class string { static int exit_title, exit_busy, exit_wait, settings_cancel; } }
class BackgroundDialogBuilder {
    interface Click { void call(Object dialog, int which); }
    interface Cancel { void call(Object dialog); }
    static Click wait, cancel;
    static Cancel dismiss;
    BackgroundDialogBuilder(Activity a) {}
    BackgroundDialogBuilder setTitle(int s) { return this; }
    BackgroundDialogBuilder setMessage(int s) { return this; }
    BackgroundDialogBuilder setPositiveButton(int s, Click c) { wait = c; return this; }
    BackgroundDialogBuilder setNegativeButton(int s, Click c) { cancel = c; return this; }
    BackgroundDialogBuilder setOnCancelListener(Cancel c) { dismiss = c; return this; }
    void show() {}
}
'@
    'com/legbehindneck/wudict/ExitTest.java' = @'
package com.legbehindneck.wudict;
import android.app.*;
import android.os.Handler;
public class ExitTest {
    static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }
    static <T extends Activity> T create(AppExit app, T a, int task) {
        a.app = app; a.task = task;
        // Activity.super.onCreate invokes the lifecycle callback BEFORE the
        // subclass checks whether to build its views and start the server.
        app.onActivityCreated(a, null);
        return a;
    }
    static void exit(MainActivity main) throws Exception {
        AppExit.request(main);
        Handler.next();
    }
    public static void main(String[] args) throws Exception {
        // One Exit closes all windows, either destruction order; a new
        // launcher task works repeatedly without restarting the process.
        for (boolean mainFirst : new boolean[] {false, true}) {
            AppExit app = new AppExit();
            for (int task = 1; task <= 3; task++) {
                MainActivity main = create(app, new MainActivity(), task);
                check(!AppExit.shouldSuppressRestart(main), "fresh launcher blocked");
                Activity settings = create(app, new Activity(), task);
                int before = ServerProcess.stops;
                exit(main);
                check(main.finished && settings.finished, "Exit did not finish all windows");
                check(ServerProcess.stops == before, "server stopped before windows closed");
                app.onActivityDestroyed(mainFirst ? main : settings);
                check(ServerProcess.stops == before, "server stopped with a live window");
                app.onActivityDestroyed(mainFirst ? settings : main);
                check(ServerProcess.stops == before + 1, "server not stopped exactly once");
                MainActivity restored = create(app, new MainActivity(), task);
                check(AppExit.shouldSuppressRestart(restored), "late base restoration accepted");
                app.onActivityDestroyed(restored);
                check(ServerProcess.stops == before + 1, "restoration stopped server twice");
            }
        }
        // A restored launcher while Settings is still being destroyed must
        // neither start a server nor revoke the pending shutdown.
        AppExit app = new AppExit();
        MainActivity main = create(app, new MainActivity(), 10);
        Activity settings = create(app, new Activity(), 10);
        int before = ServerProcess.stops;
        exit(main);
        app.onActivityDestroyed(main);
        MainActivity restored = create(app, new MainActivity(), 10);
        check(AppExit.shouldSuppressRestart(restored), "restoration revoked shutdown");
        app.onActivityDestroyed(settings);
        check(ServerProcess.stops == before, "restored window still registered");
        app.onActivityDestroyed(restored);
        check(ServerProcess.stops == before + 1, "restoration prevented shutdown");

        // A new launcher or external reader lookup can arrive during teardown.
        // Destroying the old windows must not stop the new window's server.
        for (boolean lookup : new boolean[] {false, true}) {
            app = new AppExit();
            main = create(app, new MainActivity(), 20);
            before = ServerProcess.stops;
            exit(main);
            Activity fresh = create(app, lookup ? new Activity() : new MainActivity(), lookup ? 20 : 21);
            if (!lookup) check(!AppExit.shouldSuppressRestart(fresh), "new launcher rejected during Exit");
            app.onActivityDestroyed(main);
            app.onActivityDestroyed(fresh);
            check(ServerProcess.stops == before, "old Exit stopped a fresh window's server");
        }
        // Busy work: Cancel leaves the app open; Wait finishes only after work.
        app = new AppExit();
        main = create(app, new MainActivity(), 30);
        ServerProcess.busy = true;
        exit(main);
        BackgroundDialogBuilder.cancel.call(null, 0);
        check(!main.finished && !AppExit.shouldSuppressRestart(main), "Cancel closed the app");
        exit(main);
        BackgroundDialogBuilder.wait.call(null, 0);
        Handler.next(); // still busy: enqueue another check
        check(!main.finished, "Wait interrupted work");
        ServerProcess.busy = false;
        Handler.next(); // run delayed check
        Handler.next(); // deliver result
        check(main.finished, "Wait did not finish after work");
        app.onActivityDestroyed(main);
        check(!AppExit.shouldSuppressRestart(create(app, new MainActivity(), 31)), "launch after Wait blocked");
        System.out.println("PASS: repeated Exit/relaunch, both window orders, early/late restoration, fresh launch/lookup during teardown, busy Cancel/Wait");
    }
}
'@
}
try {
    foreach ($entry in $sources.GetEnumerator()) {
        $path = Join-Path $testRoot $entry.Key
        $null = New-Item -ItemType Directory -Path (Split-Path $path) -Force
        [IO.File]::WriteAllText($path, $entry.Value)
    }
    Copy-Item -LiteralPath $SourceFile -Destination (Join-Path $testRoot 'com/legbehindneck/wudict/AppExit.java')
    $javaFiles = @(Get-ChildItem -LiteralPath $testRoot -Filter '*.java' -Recurse | ForEach-Object FullName)
    & javac -d $testRoot @javaFiles
    if ($LASTEXITCODE -ne 0) { throw 'Exit test compilation failed' }
    & java -cp $testRoot com.legbehindneck.wudict.ExitTest
    if ($LASTEXITCODE -ne 0) { throw 'Exit lifecycle regression failed' }
} finally {
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    $tempParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (!$resolvedTestRoot.StartsWith($tempParent, [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path $resolvedTestRoot -Leaf) -notlike 'wudict-exit-*') {
        throw 'Refusing cleanup outside the test temporary directory'
    }
    Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
}
