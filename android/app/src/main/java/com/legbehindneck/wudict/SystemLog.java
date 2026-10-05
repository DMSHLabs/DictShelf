// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.content.Context;
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
    private static File root;

    static void initialize(Context app) {
        synchronized (LOCK) {
            if (root != null) return;
            root = new File(app.getFilesDir(), "system-log");
        }
        record("application start build=" + BuildConfig.VERSION_NAME);
        Thread.UncaughtExceptionHandler previous = Thread.getDefaultUncaughtExceptionHandler();
        if (previous != null) Thread.setDefaultUncaughtExceptionHandler((thread, error) -> {
            try {
                record("uncaught thread=" + thread.getName() + " " + Log.getStackTraceString(error));
            } catch (Throwable ignored) {
                // The platform must still receive the original crash, including OOM.
            } finally {
                previous.uncaughtException(thread, error);
            }
        });
    }

    static void warn(String tag, String message) { warn(tag, message, null); }
    static void warn(String tag, String message, Throwable error) {
        record("warning " + tag + ": " + message + (error == null ? "" : " " + Log.getStackTraceString(error)));
        Log.w(tag, message, error);
    }

    static void record(String message) {
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
                    File previous = new File(root, "android.log.1");
                    if (previous.exists() && !previous.delete()) return;
                    if (!current.renameTo(previous)) return;
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
            for (String name : new String[]{"android.log.1", "android.log"}) {
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
