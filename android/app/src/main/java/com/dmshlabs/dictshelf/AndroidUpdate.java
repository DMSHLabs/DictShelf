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

import org.json.JSONObject;

import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Locale;

/** GitHub/F-Droid APK updater. Kept in src/main but is inert in the Play flavour. */
final class AndroidUpdate {
    private static final String PREFS = "dictshelf_updates";
    private static final String KEY_ID = "download_id";
    private static final String KEY_SHA = "download_sha256";
    private static final String KEY_VERIFIED = "download_verified";
    private static final String KEY_LAUNCHED = "install_launched";
    private BroadcastReceiver receiver;

    AndroidUpdate() {}

    static boolean available() {
        return "foss".equals(BuildConfig.FLAVOR);
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
        DownloadManager.Request download = new DownloadManager.Request(uri)
                .setTitle(activity.getString(R.string.update_download_title))
                .setDescription(activity.getString(R.string.update_download_description))
                .setMimeType("application/vnd.android.package-archive")
                .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                .setAllowedOverMetered(true)
                .setAllowedOverRoaming(false)
                .setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS,
                        "DictShelf-" + version + ".apk");
        long id = manager.enqueue(download);
        activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putLong(KEY_ID, id).putString(KEY_SHA, sha)
                .putBoolean(KEY_VERIFIED, false).putBoolean(KEY_LAUNCHED, false).apply();
        return new JSONObject().put("ok", true).put("downloadId", id);
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

    private static void handleCompleted(Activity activity, long id) {
        if (!available()) return;
        android.content.SharedPreferences prefs = activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        if (id < 0 || prefs.getLong(KEY_ID, -1) != id) return;
        DownloadManager manager = (DownloadManager) activity.getSystemService(Context.DOWNLOAD_SERVICE);
        if (manager == null) return;
        DownloadManager.Query query = new DownloadManager.Query().setFilterById(id);
        try (android.database.Cursor cursor = manager.query(query)) {
            if (cursor == null || !cursor.moveToFirst()) return;
            int status = cursor.getInt(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS));
            if (status == DownloadManager.STATUS_FAILED) {
                prefs.edit().remove(KEY_ID).apply();
                return;
            }
            if (status != DownloadManager.STATUS_SUCCESSFUL) return;
            Uri apk = manager.getUriForDownloadedFile(id);
            if (apk == null) return;
            if (!prefs.getBoolean(KEY_VERIFIED, false)) {
                String actual = sha256(activity, apk);
                if (!actual.equalsIgnoreCase(prefs.getString(KEY_SHA, ""))) {
                    manager.remove(id);
                    prefs.edit().remove(KEY_ID).apply();
                    return;
                }
                prefs.edit().putBoolean(KEY_VERIFIED, true).apply();
            }
            if (Build.VERSION.SDK_INT >= 26
                    && !activity.getPackageManager().canRequestPackageInstalls()) return;
            if (prefs.getBoolean(KEY_LAUNCHED, false)) return;
            Intent install = new Intent(Intent.ACTION_VIEW)
                    .setDataAndType(apk, "application/vnd.android.package-archive")
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_GRANT_READ_URI_PERMISSION);
            activity.startActivity(install);
            prefs.edit().putBoolean(KEY_LAUNCHED, true).apply();
        } catch (Exception error) {
            SystemLog.warn("update", "could not prepare Android update", error);
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
