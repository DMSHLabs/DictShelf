// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Environment;
import android.provider.DocumentsContract;
import android.widget.Toast;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.Locale;

final class SystemLogExport {
    private static final int REQUEST = 0x574c;
    private static final Object EXPORT_LOCK = new Object();

    static void start(Activity a) {
        SystemLog.record("System Log export picker opened");
        String name = "wuDict2_SystemLog_" + new SimpleDateFormat("yyyy_MM_dd-HH-mm", Locale.ROOT).format(new Date()) + ".log";
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT)
                .addCategory(Intent.CATEGORY_OPENABLE)
                .setType("text/plain")
                .putExtra(Intent.EXTRA_TITLE, name)
                .putExtra(DocumentsContract.EXTRA_INITIAL_URI,
                        DocumentsContract.buildDocumentUri("com.android.externalstorage.documents",
                                "primary:" + Environment.DIRECTORY_DOWNLOADS));
        a.startActivityForResult(intent, REQUEST);
    }

    static void result(Activity a, int request, int code, Intent data) {
        if (request != REQUEST || code != Activity.RESULT_OK || data == null || data.getData() == null) return;
        Uri destination = data.getData();
        SystemLog.record("System Log export destination selected");
        Toast.makeText(a, UiLanguage.resources(a, a.getResources()).getString(R.string.system_log_saving), Toast.LENGTH_SHORT).show();
        new Thread(() -> {
            // Serialize exports, including an accidental repeated result for one URI.
            // Neither logger's writer lock is held during network or provider I/O.
            synchronized (EXPORT_LOCK) {
                try {
                    ByteArrayOutputStream snapshot = new ByteArrayOutputStream();
                    try {
                        snapshot.write(serverSnapshot(a));
                    } catch (Exception e) {
                        SystemLog.warn("wudict", "server log unavailable during export", e);
                        snapshot.write(("wuDict2 System Log\nServer log unavailable: " + e.getClass().getSimpleName() + "\n").getBytes(StandardCharsets.UTF_8));
                    }
                    snapshot.write("\n--- Android (UTC) ---\n".getBytes(StandardCharsets.UTF_8));
                    snapshot.write(SystemLog.snapshot());
                    byte[] output = snapshot.toByteArray();
                    if (output.length == 0) throw new java.io.IOException("Diagnostic snapshot is empty");
                    try (OutputStream out = a.getContentResolver().openOutputStream(destination, "wt")) {
                        if (out == null) throw new java.io.IOException("Cannot open destination");
                        out.write(output);
                        out.flush();
                    }
                    long stored = 0;
                    try (InputStream in = a.getContentResolver().openInputStream(destination)) {
                        if (in == null) throw new java.io.IOException("Cannot verify destination");
                        byte[] buffer = new byte[8192];
                        for (int n; (n = in.read(buffer)) != -1;) stored += n;
                    }
                    if (stored != output.length) throw new java.io.IOException("Saved " + stored + " of " + output.length + " bytes");
                    SystemLog.record("System Log export completed bytes=" + stored);
                    a.runOnUiThread(() -> Toast.makeText(a, UiLanguage.resources(a, a.getResources()).getString(R.string.system_log_saved), Toast.LENGTH_LONG).show());
                } catch (Exception e) {
                    SystemLog.warn("wudict", "System Log export failed", e);
                    a.runOnUiThread(() -> Toast.makeText(a, UiLanguage.resources(a, a.getResources()).getString(R.string.system_log_failed, e.getMessage()), Toast.LENGTH_LONG).show());
                }
            }
        }, "wudict-log-export").start();
    }

    private static byte[] serverSnapshot(Activity a) throws Exception {
        HttpURLConnection connection = (HttpURLConnection) new URL("http://127.0.0.1:" + ServerProcess.port(a) + "/api/system-log").openConnection();
        try {
            connection.setConnectTimeout(5000);
            connection.setReadTimeout(15000);
            ShellPrefs.token(a);
            ShellPrefs.authorize(connection);
            if (connection.getResponseCode() != 200) throw new java.io.IOException("HTTP " + connection.getResponseCode());
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            try (InputStream in = connection.getInputStream()) {
                byte[] buffer = new byte[8192];
                for (int n; (n = in.read(buffer)) != -1;) {
                    if (bytes.size() + n > 5 * 1024 * 1024) throw new java.io.IOException("Server log exceeds size limit");
                    bytes.write(buffer, 0, n);
                }
            }
            return bytes.toByteArray();
        } finally {
            connection.disconnect();
        }
    }
}
