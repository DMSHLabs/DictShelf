// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.app.ActivityManager;
import android.app.ApplicationExitInfo;
import android.content.Context;
import android.os.Build;
import java.time.Instant;
import java.util.List;

/** Records system-owned exit reasons after a crash, when our process can write again. */
final class ProcessExitDiagnostics {
    private ProcessExitDiagnostics() {}

    static void recordRecent(Context app) {
        if (Build.VERSION.SDK_INT < 30 || !SystemLog.enabled()) return;
        new Thread(() -> {
            try {
                if (!SystemLog.enabled()) return;
                ActivityManager manager = (ActivityManager) app.getSystemService(Context.ACTIVITY_SERVICE);
                if (manager == null) return;
                List<ApplicationExitInfo> exits = manager.getHistoricalProcessExitReasons(null, 0, 10);
                long newest = app.getSharedPreferences("system-log", Context.MODE_PRIVATE)
                        .getLong("last-exit", 0);
                long seen = newest;
                for (int i = exits.size() - 1; i >= 0; i--) {
                    ApplicationExitInfo exit = exits.get(i);
                    if (!app.getPackageName().equals(exit.getProcessName()) || exit.getTimestamp() <= newest) continue;
                    seen = Math.max(seen, exit.getTimestamp());
                    int reason = exit.getReason();
                    if (reason != ApplicationExitInfo.REASON_CRASH
                            && reason != ApplicationExitInfo.REASON_CRASH_NATIVE
                            && reason != ApplicationExitInfo.REASON_ANR) continue;
                    SystemLog.recordFailure("previous process exit time=" + Instant.ofEpochMilli(exit.getTimestamp())
                            + " reason=" + reasonName(reason) + " pid=" + exit.getPid()
                            + " status=" + exit.getStatus() + " importance=" + exit.getImportance());
                }
                if (seen > newest && SystemLog.enabled()) app.getSharedPreferences("system-log", Context.MODE_PRIVATE)
                        .edit().putLong("last-exit", seen).apply();
            } catch (RuntimeException e) {
                SystemLog.warn("wudict", "cannot read previous process exits", e);
            }
        }, "wudict-exit-diagnostics").start();
    }

    private static String reasonName(int reason) {
        if (reason == ApplicationExitInfo.REASON_CRASH_NATIVE) return "native-crash";
        if (reason == ApplicationExitInfo.REASON_CRASH) return "java-crash";
        return "anr";
    }
}
