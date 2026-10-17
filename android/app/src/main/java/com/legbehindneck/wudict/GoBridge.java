// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.util.Log;
import android.content.Context;
import java.io.File;
import java.util.Map;
import org.json.JSONObject;

final class GoBridge {
    private static final String TAG = "wudict-GoBridge";
    private static final boolean loaded;
    private static volatile Context appContext;
    private static boolean observerInstalled;

    static {
        boolean ok;
        try {
            System.loadLibrary("wudictbridge");
            ok = true;
        } catch (UnsatisfiedLinkError error) {
            Log.w(TAG, "Go bridge is unavailable", error);
            ok = false;
        }
        loaded = ok;
    }

    private GoBridge() {}

    private static native int nativeProtocol();
    private static native String nativeConfigSummary(String path, String flagsJSON);
    private static native boolean nativeInstallBusyObserver();
    private static native int nativeRuntimeStart(String path, String flagsJSON,
                                                  String token, String version);
    private static native int nativeRuntimeStop();
    private static native String nativeConfigSnapshot();
    private static native String nativePrefsSnapshot();
    private static native String nativeBrowsePage(String query);
    private static native String nativeBrowseDiagnostic(String payload);
    private static native String nativeAbout(String origin, String dictionary);
    private static native String nativeOrphans(String action, String body);
    private static native String nativeRemoveLibrary(String query);
    private static native String nativeLooks(String argument);
    private static native String nativeGroups(String action, String argument);
    private static native String nativeLemmas(String action, String code);
    private static native String nativeSetup(String action, String argument);
    private static native String nativeIntake(String action, String argument);
    private static native String nativeReindex(String action);
    private static native String nativeIndexWork(String action, String id, String body);
    private static native String nativePresetsSnapshot();
    private static native String nativeSavePreset(String body);
    private static native String nativeSavePrefs(String body);
    private static native String nativeStyleSnapshot();
    private static native String nativeSaveStyle(String argument);
    private static native String nativeUserFilesSnapshot();
    private static native String nativeUserFileOperation(String method, String argument);
    private static native int nativeDictStreamStart();
    private static native int nativeRescanStreamStart();
    private static native int nativeRescanIndexesStart(String origin, String request);
    private static native int nativeIngestStart(String origin, String query);
    private static native int nativeSearchStreamStart(String origin, String request);
    private static native int nativeBrowseStreamStart(String method, String query);
    private static native String nativeDictStreamPoll(int id);
    private static native void nativeDictStreamCancel(int id);

    static int startDictStream() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeDictStreamStart();
    }

    static int startRescanStream() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeRescanStreamStart();
    }

    static int startRescanIndexes(String origin, String request) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeRescanIndexesStart(origin, request);
    }

    static int startIngest(String origin, String query) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeIngestStart(origin, query);
    }

    static int startSearchStream(String origin, String request) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeSearchStreamStart(origin, request);
    }

    static int startBrowseStream(String method, String query) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeBrowseStreamStart(method, query);
    }

    static String pollDictStream(int id) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeDictStreamPoll(id);
        if (result == null) throw new IllegalStateException("Dictionary stream is unavailable");
        return result;
    }

    static void cancelDictStream(int id) {
        if (loaded) nativeDictStreamCancel(id);
    }

    static String configSnapshot() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeConfigSnapshot();
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String prefsSnapshot() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativePrefsSnapshot();
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String browsePage(String query) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeBrowsePage(query);
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static void browseDiagnostic(String payload) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeBrowseDiagnostic(payload);
        if (result == null) throw new IllegalStateException("Could not record Browse timing");
    }

    static String about(String origin, String dictionary) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeAbout(origin, dictionary);
        if (result == null) throw new IllegalStateException("Dictionary description is unavailable");
        return result;
    }

    static String orphans(String action, String body) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeOrphans(action, body);
        if (result == null) throw new IllegalStateException("Could not load missing dictionaries");
        return result;
    }

    static String removeLibrary(String query) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeRemoveLibrary(query);
        if (result == null) throw new IllegalStateException("Could not remove dictionary");
        return result;
    }

    static String looks(String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeLooks(argument);
        if (result == null) throw new IllegalStateException("Saved appearances are unavailable");
        return result;
    }

    static String groups(String action, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeGroups(action, argument);
        if (result == null) throw new IllegalStateException("Could not edit filters");
        return result;
    }

    static String lemmas(String action, String code) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeLemmas(action, code);
        if (result == null) throw new IllegalStateException("Lemmatization is unavailable");
        return result;
    }

    static String setup(String action, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeSetup(action, argument);
        if (result == null) throw new IllegalStateException("Folder setup is unavailable");
        return result;
    }

    static String intake(String action, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeIntake(action, argument);
        if (result == null) throw new IllegalStateException("Archive import is unavailable");
        return result;
    }

    static String reindex(String action) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeReindex(action);
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String indexWork(String action, String id, String body) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeIndexWork(action, id, body);
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String presetsSnapshot() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativePresetsSnapshot();
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String savePreset(String body) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeSavePreset(body);
        if (result == null) throw new IllegalStateException("Could not save preset");
        return result;
    }

    static String savePrefs(String body) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeSavePrefs(body);
        if (result == null) throw new IllegalStateException("Could not save preferences");
        return result;
    }

    static String styleSnapshot() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeStyleSnapshot();
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String saveStyle(String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeSaveStyle(argument);
        if (result == null) throw new IllegalStateException("Could not save style");
        return result;
    }

    static String userFilesSnapshot() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeUserFilesSnapshot();
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static String userFileOperation(String method, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeUserFileOperation(method, argument);
        if (result == null) throw new IllegalStateException("Could not update user file");
        return result;
    }

    static synchronized void initialize(Context context) {
        if (!loaded) return;
        appContext = context.getApplicationContext();
        if (observerInstalled) return;
        try {
            observerInstalled = nativeInstallBusyObserver();
            if (!observerInstalled) Log.w(TAG, "Go busy observer unavailable");
        } catch (UnsatisfiedLinkError error) {
            Log.w(TAG, "Go busy observer registration failed", error);
        }
    }

    private static void onNativeBusy(boolean busy) {
        Context app = appContext;
        if (app != null) IndexService.busy(app, busy);
    }

    static int protocol() {
        if (!loaded) return 0;
        try {
            return nativeProtocol();
        } catch (UnsatisfiedLinkError error) {
            Log.e(TAG, "Go bridge protocol call failed", error);
            return 0;
        }
    }

    static void checkConfig(File home, File dbDir, Map<String, String> overrides) {
        if (!loaded) return;
        JSONObject flags = new JSONObject(overrides);
        try {
            flags.put("DB_DIR", dbDir.getAbsolutePath());
            flags.put("USE_CACHED", "1");
            File path = new File(new File(home, ".wudict"), "wudict.toml");
            String result = nativeConfigSummary(path.getAbsolutePath(), flags.toString());
            JSONObject summary = new JSONObject(result);
            if (summary.optBoolean("error")) {
                Log.w(TAG, "Go config read failed");
                return;
            }
            String message = "Go config loaded: dictDirs=" + summary.optInt("dictDirs")
                    + " dbMatches=" + summary.optBoolean("dbMatches")
                    + " sourceFound=" + summary.optBoolean("sourceFound");
            Log.i(TAG, message);
            SystemLog.record(message);
        } catch (Exception | UnsatisfiedLinkError error) {
            Log.w(TAG, "Go config probe failed", error);
        }
    }

    static int startRuntime(Context context, File home, File dbDir, int port) {
        if (!loaded) return -9;
        try {
            JSONObject flags = new JSONObject(ShellPrefs.env(context));
            flags.put("SERVER_IP", ShellPrefs.bindIp(context));
            flags.put("SERVER_PORT", String.valueOf(port));
            flags.put("DB_DIR", dbDir.getAbsolutePath());
            flags.put("USE_CACHED", "1");
            flags.put("NO_BROWSER", "1");
            File path = new File(new File(home, ".wudict"), "wudict.toml");
            return nativeRuntimeStart(path.getAbsolutePath(), flags.toString(),
                    ShellPrefs.token(context), BuildConfig.VERSION_NAME);
        } catch (Exception | UnsatisfiedLinkError error) {
            Log.e(TAG, "Go runtime start failed", error);
            return -9;
        }
    }

    static void stopRuntime() {
        if (!loaded) return;
        try {
            nativeRuntimeStop();
        } catch (UnsatisfiedLinkError error) {
            Log.e(TAG, "Go runtime stop failed", error);
        }
    }
}
