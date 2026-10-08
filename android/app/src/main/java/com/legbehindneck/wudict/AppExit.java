// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.app.Activity;
import android.app.Application;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.Set;

/** Process-local window registry. Exit never disables external lookup intents. */
public final class AppExit extends Application implements Application.ActivityLifecycleCallbacks {
    private final Set<Activity> windows = new HashSet<>();
    private final Handler main = new Handler(Looper.getMainLooper());
    private boolean pending;
    private boolean closing;
    // Android may restore the base activity of a task whose other windows
    // are closing. Suppress only those tasks, never a later launcher task.
    private final Set<Integer> exitedTasks = new HashSet<>();

    @Override public void onCreate() {
        super.onCreate();
        SystemLog.initialize(this);
        ProcessExitDiagnostics.recordRecent(this);
        registerActivityLifecycleCallbacks(this);
    }

    static void request(Activity activity) {
        AppExit app = (AppExit) activity.getApplication();
        if (app.pending) return;
        app.pending = true;
        app.check(activity, false);
    }

    static boolean shouldSuppressRestart(Activity activity) {
        return ((AppExit) activity.getApplication()).exitedTasks.contains(activity.getTaskId());
    }

    private void check(Activity owner, boolean waiting) {
        new Thread(() -> {
            boolean busy = ServerProcess.workInFlight() || IndexService.hasWork();
            main.post(() -> {
                if (!pending) return;
                if (!busy) {
                    closing = true;
                    for (Activity window : windows) {
                        if (window instanceof MainActivity) exitedTasks.add(window.getTaskId());
                    }
                    // Finish every window before stopping its connection. Removing a task
                    // while its other windows are still live can bring one back to the top.
                    for (Activity window : new ArrayList<>(windows)) {
                        window.finish(); // also preserves a reader task hosting Lookup
                    }
                    stopWhenClosed();
                } else if (waiting) {
                    main.postDelayed(() -> check(owner, true), 500);
                } else if (!owner.isFinishing() && !owner.isDestroyed()) {
                    new BackgroundDialogBuilder(owner)
                            .setTitle(R.string.exit_title)
                            .setMessage(R.string.exit_busy)
                            .setPositiveButton(R.string.exit_wait, (dialog, which) -> check(owner, true))
                            .setNegativeButton(R.string.settings_cancel, (dialog, which) -> pending = false)
                            .setOnCancelListener(dialog -> pending = false)
                            .show();
                } else pending = false;
            });
        }, "wudict-exit-check").start();
    }

    private void stopWhenClosed() {
        if (!closing || !windows.isEmpty()) return;
        ServerProcess.stopAny(this);
        closing = false;
        pending = false;
    }

    @Override public void onActivityCreated(Activity a, Bundle state) {
        windows.add(a);
        // super.onCreate dispatches this callback before MainActivity's restart
        // guard. A restored closing task must not revoke its own shutdown.
        // A new launcher task or external lookup does revoke it, so the old
        // windows' destruction cannot stop the new window's server.
        if (closing && !(a instanceof MainActivity && shouldSuppressRestart(a))) {
            closing = false;
            pending = false;
        }
    }
    @Override public void onActivityDestroyed(Activity a) {
        windows.remove(a);
        stopWhenClosed();
    }
    @Override public void onActivityStarted(Activity a) {}
    @Override public void onActivityResumed(Activity a) {}
    @Override public void onActivityPaused(Activity a) {}
    @Override public void onActivityStopped(Activity a) {}
    @Override public void onActivitySaveInstanceState(Activity a, Bundle state) {}
}
