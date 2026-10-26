// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.dmshlabs.dictshelf;

import android.app.Activity;
import android.app.ActivityManager;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Environment;
import android.os.Process;
import android.provider.DocumentsContract;
import android.widget.Toast;
import java.io.ByteArrayOutputStream;
import java.io.BufferedReader;
import java.io.File;
import java.io.FileReader;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.HashSet;
import java.util.Locale;
import java.util.Set;
import org.json.JSONObject;

final class SystemLogExport {
    private static final int REQUEST = 0x574c;
    private static final Object EXPORT_LOCK = new Object();

    static void start(Activity a) {
        SystemLog.record("System Log export picker opened");
        String name = "DictShelf_SystemLog_" + new SimpleDateFormat("yyyy_MM_dd-HH-mm", Locale.ROOT).format(new Date()) + ".log";
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
                    byte[] goLog;
                    try {
                        goLog = goSnapshot();
                    } catch (Exception e) {
                        SystemLog.warn("wudict", "Go system log unavailable during export", e);
                        goLog = ("DictShelf System Log\nGo system log unavailable: " + e.getClass().getSimpleName() + "\n").getBytes(StandardCharsets.UTF_8);
                    }
                    snapshot.write(withAndroidEnvironment(goLog, a));
                    snapshot.write("\n--- Android events (UTC) ---\n".getBytes(StandardCharsets.UTF_8));
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

    private static byte[] withAndroidEnvironment(byte[] goLog, Activity activity) {
        String text = new String(goLog, StandardCharsets.UTF_8);
        String serverMarker = "\n\n--- Server ---\n";
        int serverAt = text.indexOf(serverMarker);
        String prefix = serverAt < 0 ? text : text.substring(0, serverAt);
        String server = serverAt < 0 ? "\n\n--- Server ---\n" : text.substring(serverAt);
        String environment = "\n\n--- Android environment ---\n"
                + "App: " + BuildConfig.VERSION_NAME + "\n"
                + "Android: " + Build.VERSION.RELEASE + " (SDK " + Build.VERSION.SDK_INT + ")\n"
                + "Android build: " + Build.DISPLAY + "\n"
                + "Device: " + Build.MANUFACTURER + " " + Build.MODEL + "\n"
                + "CPU hardware: " + Build.HARDWARE + "\n"
                + "Supported ABIs: " + String.join(", ", Build.SUPPORTED_ABIS) + "\n"
                + "Process bitness: " + (Process.is64Bit() ? "64-bit" : "32-bit") + "\n"
                + "Logical processors (JVM): " + Runtime.getRuntime().availableProcessors() + "\n"
                + "Online processors (sysfs): " + ShellPrefs.availableCpuCores() + "\n"
                + "Physical CPU cores (sysfs topology): " + physicalCpuCoreCount() + "\n"
                + androidMemoryInfo(activity);
        if (!server.isEmpty()) {
            server = "\n--- Server ---\n" + server.substring(serverMarker.length());
        }
        return (prefix + environment + server).getBytes(StandardCharsets.UTF_8);
    }

    private static String androidMemoryInfo(Activity activity) {
        ActivityManager manager = (ActivityManager) activity.getSystemService(Context.ACTIVITY_SERVICE);
        if (manager == null) return "Physical memory: unavailable\n";
        ActivityManager.MemoryInfo info = new ActivityManager.MemoryInfo();
        manager.getMemoryInfo(info);
        return "Physical memory total: " + formatMemory(info.totalMem) + "\n"
                + "Physical memory available: " + formatMemory(info.availMem) + "\n"
                + "App memory class: " + manager.getMemoryClass() + " MiB\n"
                + "App large memory class: " + manager.getLargeMemoryClass() + " MiB\n";
    }

    private static String formatMemory(long bytes) {
        return String.format(Locale.ROOT, "%.1f GiB (%d bytes)", bytes / (1024.0 * 1024.0 * 1024.0), bytes);
    }

    private static int physicalCpuCoreCount() {
        File cpuRoot = new File("/sys/devices/system/cpu");
        File[] cpus = cpuRoot.listFiles((dir, name) -> name.matches("cpu[0-9]+"));
        if (cpus == null) return -1;
        Set<String> cores = new HashSet<>();
        for (File cpu : cpus) {
            File topology = new File(cpu, "topology");
            String core = readTrimmed(new File(topology, "core_id"));
            if (core == null) continue;
            String cluster = readTrimmed(new File(topology, "cluster_id"));
            String physicalPackage = readTrimmed(new File(topology, "physical_package_id"));
            cores.add((physicalPackage == null ? "0" : physicalPackage) + ":"
                    + (cluster == null ? "" : cluster) + ":" + core);
        }
        return cores.isEmpty() ? -1 : cores.size();
    }

    private static String readTrimmed(File file) {
        try (BufferedReader reader = new BufferedReader(new FileReader(file))) {
            String value = reader.readLine();
            return value == null || value.trim().isEmpty() ? null : value.trim();
        } catch (Exception ignored) {
            return null;
        }
    }
}
