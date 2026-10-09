// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.content.Context;
import android.content.SharedPreferences;
import android.util.Log;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.time.Instant;

/** Android owns these files; the Go child owns a separate log. */
final class SystemLog {
    private static final Object LOCK = new Object();
    private static final int LIMIT = 1024 * 1024;
    private static final int FILES = 5;
    private static final String PREFS = "system-log-options";
    private static File root;
    private static volatile boolean enabled = true;
    private static volatile boolean baseEvents = true;
    private static volatile boolean detailedEvents;

    static void initialize(Context app) {
        synchronized (LOCK) {
            if (root != null) return;
            root = new File(app.getFilesDir(), "system-log");
            SharedPreferences prefs = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
            enabled = prefs.getBoolean("enabled", true);
            baseEvents = prefs.getBoolean("base-events", true);
            detailedEvents = prefs.getBoolean("detailed-events", false);
        }
        record("application start build=" + BuildConfig.VERSION_NAME);
        Thread.UncaughtExceptionHandler previous = Thread.getDefaultUncaughtExceptionHandler();
        if (previous != null) Thread.setDefaultUncaughtExceptionHandler((thread, error) -> {
            try {
                recordFailure("uncaught thread=" + thread.getName() + " " + Log.getStackTraceString(error));
            } catch (Throwable ignored) {
                // The platform must still receive the original crash, including OOM.
            } finally {
                previous.uncaughtException(thread, error);
            }
        });
    }

    static void warn(String tag, String message) { warn(tag, message, null); }
    static void warn(String tag, String message, Throwable error) {
        recordFailure("warning " + tag + ": " + message + (error == null ? "" : " " + Log.getStackTraceString(error)));
        Log.w(tag, message, error);
    }

    static void record(String message) {
        if (!enabled || !baseEvents) return;
        write(message);
    }

    static void recordFailure(String message) {
        if (enabled) write(message);
    }

    static void recordDetailed(String message) {
        if (enabled && detailedEvents) write(message);
    }

    static boolean enabled() { return enabled; }
    static boolean detailedEnabled() { return enabled && detailedEvents; }
    static boolean baseEvents() { return baseEvents; }
    static boolean detailedEvents() { return detailedEvents; }

    static void configure(Context app, boolean on, boolean base, boolean detailed) {
        app.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putBoolean("enabled", on)
                .putBoolean("base-events", base)
                .putBoolean("detailed-events", detailed).apply();
        enabled = on;
        baseEvents = base;
        detailedEvents = detailed;
    }

    private static void write(String message) {
        // URLs may contain access keys or credentials; keep host and path only.
        message = message.replaceAll("(https?://)[^/\\s]+@", "$1")
                .replaceAll("(https?://[^\\s?\"<>#]+)[?#][^\\s\"<>]*", "$1[redacted]")
                .replace("\r", "\\r").replace("\n", "\\n");
        if (message.length() > 4096) message = message.substring(0, 4096) + " [truncated]";
        synchronized (LOCK) {
            if (root == null) return;
            byte[] bytes = (Instant.now() + " " + message + "\n").getBytes(StandardCharsets.UTF_8);
            try {
                if (!root.isDirectory() && !root.mkdirs()) return;
                File current = new File(root, "android.log");
                if (current.length() + bytes.length > LIMIT) {
                    File oldest = new File(root, "android.log." + (FILES - 1));
                    if (oldest.exists() && !oldest.delete()) return;
                    for (int i = FILES - 2; i >= 1; i--) {
                        File previous = new File(root, "android.log." + i);
                        if (previous.exists() && !previous.renameTo(new File(root, "android.log." + (i + 1)))) return;
                    }
                    if (!current.renameTo(new File(root, "android.log.1"))) return;
                }
                try (FileOutputStream out = new FileOutputStream(current, true)) {
                    out.write(bytes);
                    out.getFD().sync();
                }
            } catch (Exception ignored) {
                // Logging cannot mask an original error or log recursively.
            }
        }
    }

    static byte[] snapshot() throws java.io.IOException {
        synchronized (LOCK) {
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            if (root == null) return bytes.toByteArray();
            for (int i = FILES - 1; i >= 0; i--) {
                String name = "android.log" + (i == 0 ? "" : "." + i);
                File file = new File(root, name);
                if (!file.exists()) continue;
                try (FileInputStream in = new FileInputStream(file)) {
                    byte[] buffer = new byte[8192];
                    for (int n; (n = in.read(buffer)) != -1;) bytes.write(buffer, 0, n);
                }
            }
            return bytes.toByteArray();
        }
    }
}
