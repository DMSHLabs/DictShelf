// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Environment;
import android.provider.DocumentsContract;
import android.widget.Toast;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.Locale;
import org.json.JSONObject;

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
                        snapshot.write(goSnapshot());
                    } catch (Exception e) {
                        SystemLog.warn("wudict", "Go system log unavailable during export", e);
                        snapshot.write(("wuDict2 System Log\nGo system log unavailable: " + e.getClass().getSimpleName() + "\n").getBytes(StandardCharsets.UTF_8));
                    }
                    snapshot.write(("\n--- Android environment ---\n"
                            + "App: " + BuildConfig.VERSION_NAME + "\n"
                            + "Android: " + Build.VERSION.RELEASE + " (SDK " + Build.VERSION.SDK_INT + ")\n"
                            + "Build: " + Build.DISPLAY + "\n"
                            + "Device: " + Build.MANUFACTURER + " " + Build.MODEL + "\n"
                            + "\n--- Android events (UTC) ---\n").getBytes(StandardCharsets.UTF_8));
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

    private static byte[] goSnapshot() throws Exception {
        JSONObject result = new JSONObject(GoBridge.systemLogSnapshot());
        if (!result.optBoolean("ok", false))
            throw new java.io.IOException(result.optString("error", "Could not read Go system log"));
        JSONObject value = result.optJSONObject("value");
        if (value == null) throw new java.io.IOException("Go system log response is empty");
        byte[] bytes = value.optString("content", "").getBytes(StandardCharsets.UTF_8);
        if (bytes.length > 5 * 1024 * 1024) throw new java.io.IOException("Go system log exceeds size limit");
        return bytes;
    }
}
