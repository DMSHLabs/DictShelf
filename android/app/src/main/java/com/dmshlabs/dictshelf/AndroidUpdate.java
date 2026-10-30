// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.dmshlabs.dictshelf;

import android.app.Activity;
import android.app.DownloadManager;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.net.Uri;
import android.os.Build;
import android.os.Environment;
import android.provider.Settings;

import org.json.JSONObject;

import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Locale;

/** GitHub/F-Droid APK updater. Kept in src/main but is inert in the Play flavour. */
final class AndroidUpdate {
    private static final String PREFS = "dictshelf_updates";
    private static final String KEY_ID = "download_id";
    private static final String KEY_SHA = "download_sha256";
    private static final String KEY_VERSION = "download_version";
    private static final String KEY_VERIFIED = "download_verified";
    private static final String KEY_PENDING_INSTALL = "pending_install_permission";
    private BroadcastReceiver receiver;

    AndroidUpdate() {}

    static boolean available() {
        return "foss".equals(BuildConfig.FLAVOR);
    }

    static JSONObject request(Activity activity, String payload) throws Exception {
        JSONObject request = new JSONObject(payload);
        switch (request.optString("action", "download")) {
            case "download": return start(activity, payload);
            case "status": return status(activity, request);
            case "install": return install(activity, request);
            case "permission": return openInstallPermissionSettings(activity);
            default: throw new IllegalArgumentException("Unknown Android update action");
        }
    }

    static JSONObject start(Activity activity, String payload) throws Exception {
        if (!available()) throw new SecurityException("Android updates are unavailable in this distribution");
        JSONObject request = new JSONObject(payload);
        String version = request.getString("version");
        if (!version.matches("\\d+\\.\\d+\\.\\d+(?:-[0-9A-Za-z.-]+)?"))
            throw new IllegalArgumentException("Invalid version");
        JSONObject apks = request.optJSONObject("androidApks");
        if (apks == null) throw new IllegalArgumentException("No Android APK is listed for this release");
        JSONObject asset = chooseAsset(apks);
        String url = asset.getString("url");
        Uri uri = Uri.parse(url);
        if (!"https".equals(uri.getScheme()) || !"github.com".equals(uri.getHost())
                || !uri.getPath().startsWith("/DMSHLabs/DictShelf/releases/download/"))
            throw new IllegalArgumentException("Untrusted APK URL");
        String sha = asset.getString("sha256").toLowerCase(Locale.ROOT);
        if (!sha.matches("[0-9a-f]{64}")) throw new IllegalArgumentException("Invalid APK checksum");

        DownloadManager manager = (DownloadManager) activity.getSystemService(Context.DOWNLOAD_SERVICE);
        if (manager == null) throw new IllegalStateException("Downloads are unavailable");
        JSONObject existing = status(activity, request);
        String existingState = existing.optString("state");
        if ("ready".equals(existingState) || "downloading".equals(existingState))
            return existing.put("ok", true);
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        clearDownload(manager, prefs, prefs.getLong(KEY_ID, -1));
        DownloadManager.Request download = new DownloadManager.Request(uri)
                .setTitle(activity.getString(R.string.update_download_title))
                .setDescription(activity.getString(R.string.update_download_description))
                .setMimeType("application/vnd.android.package-archive")
                .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                .setAllowedOverMetered(true)
                .setAllowedOverRoaming(false)
                .setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS,
                        "DictShelf-" + version + "-" + sha.substring(0, 12) + "-"
                                + Long.toString(System.currentTimeMillis(), 36) + ".apk");
        long id = manager.enqueue(download);
        prefs.edit()
                .putLong(KEY_ID, id).putString(KEY_SHA, sha)
                .putString(KEY_VERSION, version).putBoolean(KEY_VERIFIED, false).apply();
        return new JSONObject().put("ok", true).put("downloadId", id).put("state", "downloading");
    }

    private static JSONObject status(Activity activity, JSONObject request) throws Exception {
        if (!available()) throw new SecurityException("Android updates are unavailable in this distribution");
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        long id = prefs.getLong(KEY_ID, -1);
        String version = prefs.getString(KEY_VERSION, "");
        String sha = prefs.getString(KEY_SHA, "");
        if (id < 0) return new JSONObject().put("state", "none");
        DownloadManager manager = (DownloadManager) activity.getSystemService(Context.DOWNLOAD_SERVICE);
        if (manager == null) throw new IllegalStateException("Downloads are unavailable");
        String expectedVersion = request.optString("version", "");
        if (!expectedVersion.isEmpty()) {
            String expectedSha = expectedSha256(request);
            if (!expectedVersion.equals(version) || !expectedSha.equalsIgnoreCase(sha)) {
                clearDownload(manager, prefs, id);
                return new JSONObject().put("state", "none");
            }
        }
        try (android.database.Cursor cursor = manager.query(new DownloadManager.Query().setFilterById(id))) {
            if (cursor == null || !cursor.moveToFirst()) return new JSONObject().put("state", "none");
            int state = cursor.getInt(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS));
            if (state == DownloadManager.STATUS_FAILED) {
                return new JSONObject().put("state", "failed").put("version", version).put("sha256", sha);
            }
            if (state != DownloadManager.STATUS_SUCCESSFUL) {
                long downloaded = cursor.getLong(cursor.getColumnIndexOrThrow(
                        DownloadManager.COLUMN_BYTES_DOWNLOADED_SO_FAR));
                long total = cursor.getLong(cursor.getColumnIndexOrThrow(
                        DownloadManager.COLUMN_TOTAL_SIZE_BYTES));
                int progress = total > 0 ? (int) Math.min(100, downloaded * 100 / total) : -1;
                return new JSONObject().put("state", "downloading").put("version", version).put("sha256", sha)
                        .put("progress", progress);
            }
            Uri apk = manager.getUriForDownloadedFile(id);
            if (apk == null) return new JSONObject().put("state", "failed").put("version", version).put("sha256", sha);
            if (!prefs.getBoolean(KEY_VERIFIED, false)) {
                String actual = sha256(activity, apk);
                if (!actual.equalsIgnoreCase(prefs.getString(KEY_SHA, ""))) {
                    manager.remove(id);
                    prefs.edit().putBoolean(KEY_VERIFIED, false).apply();
                    return new JSONObject().put("state", "failed").put("version", version).put("sha256", sha);
                }
                prefs.edit().putBoolean(KEY_VERIFIED, true).apply();
            }
            return new JSONObject().put("state", "ready").put("version", version).put("sha256", sha);
        }
    }

    private static JSONObject install(Activity activity, JSONObject request) throws Exception {
        activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putBoolean(KEY_VERIFIED, false).apply();
        JSONObject current = status(activity, request);
        if (!"ready".equals(current.optString("state")))
            return new JSONObject().put("error", "The downloaded APK is not ready to install");
        if (Build.VERSION.SDK_INT >= 26
                && !activity.getPackageManager().canRequestPackageInstalls()) {
            return new JSONObject().put("permission", true).put("version", current.optString("version"));
        }
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        long id = prefs.getLong(KEY_ID, -1);
        DownloadManager manager = (DownloadManager) activity.getSystemService(Context.DOWNLOAD_SERVICE);
        Uri apk = manager == null ? null : manager.getUriForDownloadedFile(id);
        if (apk == null) return new JSONObject().put("error", "The downloaded APK is unavailable");
        Intent install = new Intent(Intent.ACTION_VIEW)
                .setDataAndType(apk, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_GRANT_READ_URI_PERMISSION);
        activity.startActivity(install);
        return new JSONObject().put("ok", true);
    }

    private static JSONObject openInstallPermissionSettings(Activity activity) throws Exception {
        if (Build.VERSION.SDK_INT < 26 || activity.getPackageManager().canRequestPackageInstalls())
            return new JSONObject().put("ok", true);
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        prefs.edit().putBoolean(KEY_PENDING_INSTALL, true).apply();
        Intent settings = new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                Uri.parse("package:" + activity.getPackageName()));
        try {
            activity.startActivity(settings);
            return new JSONObject().put("ok", true);
        } catch (Exception error) {
            prefs.edit().putBoolean(KEY_PENDING_INSTALL, false).apply();
            throw error;
        }
    }

    void onResume(MainActivity activity) {
        if (!available()) return;
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        if (!prefs.getBoolean(KEY_PENDING_INSTALL, false)) return;
        prefs.edit().putBoolean(KEY_PENDING_INSTALL, false).apply();
        if (Build.VERSION.SDK_INT >= 26
                && !activity.getPackageManager().canRequestPackageInstalls()) {
            activity.notifyAndroidInstallPermissionMissing();
            return;
        }
        try {
            JSONObject result = install(activity, new JSONObject().put("action", "install"));
            if (result.has("error")) activity.notifyAndroidUpdateFailed();
            else activity.notifyAndroidInstallStarted();
        } catch (Exception error) {
            SystemLog.warn("update", "could not install Android update after permission", error);
            activity.notifyAndroidUpdateFailed();
        }
    }

    private static String expectedSha256(JSONObject request) throws Exception {
        JSONObject apks = request.optJSONObject("androidApks");
        if (apks == null) throw new IllegalArgumentException("No Android APK is listed for this release");
        String sha = chooseAsset(apks).optString("sha256", "").toLowerCase(Locale.ROOT);
        if (!sha.matches("[0-9a-f]{64}")) throw new IllegalArgumentException("Invalid APK checksum");
        return sha;
    }

    private static void clearDownload(DownloadManager manager,
            android.content.SharedPreferences prefs, long id) {
        if (id >= 0) manager.remove(id);
        prefs.edit().remove(KEY_ID).remove(KEY_SHA).remove(KEY_VERSION)
                .remove(KEY_VERIFIED).apply();
    }

    private static JSONObject chooseAsset(JSONObject apks) throws Exception {
        boolean supportsArm = false;
        for (String abi : Build.SUPPORTED_ABIS) {
            if ("arm64-v8a".equals(abi) || "armeabi-v7a".equals(abi)) {
                supportsArm = true;
                if (apks.has(abi)) return apks.getJSONObject(abi);
            }
        }
        if (supportsArm && apks.has("universal")) return apks.getJSONObject("universal");
        throw new IllegalArgumentException("No APK matches this device");
    }

    void onStart(MainActivity activity) {
        if (!available()) return;
        if (receiver == null) {
            receiver = new BroadcastReceiver() {
                @Override public void onReceive(Context context, Intent intent) {
                    if (DownloadManager.ACTION_DOWNLOAD_COMPLETE.equals(intent.getAction()))
                        handleCompleted(activity, intent.getLongExtra(DownloadManager.EXTRA_DOWNLOAD_ID, -1));
                }
            };
        }
        IntentFilter filter = new IntentFilter(DownloadManager.ACTION_DOWNLOAD_COMPLETE);
        if (Build.VERSION.SDK_INT >= 33) activity.registerReceiver(receiver, filter, Context.RECEIVER_EXPORTED);
        else activity.registerReceiver(receiver, filter);
        long id = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getLong(KEY_ID, -1);
        if (id >= 0) handleCompleted(activity, id);
    }

    void onStop(MainActivity activity) {
        if (receiver == null || !available()) return;
        try { activity.unregisterReceiver(receiver); } catch (IllegalArgumentException ignored) {}
        receiver = null;
    }

    private void handleCompleted(MainActivity activity, long id) {
        if (!available()) return;
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        if (id < 0 || prefs.getLong(KEY_ID, -1) != id) return;
        try {
            JSONObject current = status(activity, new JSONObject());
            if ("ready".equals(current.optString("state"))) {
                activity.notifyAndroidUpdateReady(current.optString("version"), current.optString("sha256"));
            } else if ("failed".equals(current.optString("state"))) {
                activity.notifyAndroidUpdateFailed();
            }
        } catch (Exception error) {
            SystemLog.warn("update", "could not check Android update download", error);
        }
    }

    private static String sha256(Context context, Uri uri) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (InputStream input = context.getContentResolver().openInputStream(uri)) {
            if (input == null) throw new IllegalStateException("Downloaded APK is unavailable");
            byte[] buffer = new byte[32 * 1024];
            int count;
            while ((count = input.read(buffer)) >= 0) digest.update(buffer, 0, count);
        }
        StringBuilder hex = new StringBuilder(64);
        for (byte b : digest.digest()) hex.append(String.format(Locale.ROOT, "%02x", b & 0xff));
        return hex.toString();
    }
}
