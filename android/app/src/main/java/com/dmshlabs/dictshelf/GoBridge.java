// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.dmshlabs.dictshelf;

import android.util.Log;
import android.content.Context;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
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
                                                  String version);
    private static native int nativeRuntimeStop();
    private static native String nativeConfigSnapshot();
    private static native String nativePrefsSnapshot();
    private static native String nativeBrowsePage(String query);
    private static native String nativeBrowseFindPage(String query);
    private static native String nativeBrowseDiagnostic(String payload);
    private static native String nativeAbout(String origin, String dictionary);
    private static native String nativeOrphans(String action, String body);
    private static native String nativeRemoveLibrary(String query);
    private static native String nativeLooks(String argument);
    private static native String nativeGroups(String action, String argument);
    private static native String nativeLemmas(String action, String code);
    private static native String nativeSetup(String action, String argument);
    private static native String nativeHeadwordExport(String action, String argument);
    private static native String nativeIntake(String action, String argument);
    private static native boolean nativePower(String state);
    private static native String nativeReindex(String action);
    private static native String nativeIndexWork(String action, String id, String body);
    private static native String nativePresetsSnapshot();
    private static native String nativeSavePreset(String body);
    private static native String nativeSavePrefs(String body);
    private static native String nativeSaveLanguage(String body);
    private static native String nativeSystemLogSnapshot();
    private static native int nativeSystemLogOptions(boolean enabled, boolean baseEvents, boolean webEvents);
    private static native int nativeIndexWorkers(int workers);
    private static native String nativeWebResource(String rawURL, String headers);
    private static native String nativeMediaOpen(String rawURL, String headers);
    private static native int nativeMediaRead(long handle, byte[] dst, int offset, int length);
    private static native void nativeMediaClose(long handle);
    private static native String nativeStyleSnapshot();
    private static native String nativeSaveStyle(String argument);
    private static native String nativeUserFilesSnapshot();
    private static native String nativeUserFileOperation(String method, String argument);
    private static native String nativeUserStorage(String method, String argument);
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

    static String browseFindPage(String query) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeBrowseFindPage(query);
        if (result == null) throw new IllegalStateException("Browse results are unavailable");
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

    static String headwordExport(String action, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeHeadwordExport(action, argument);
        if (result == null) throw new IllegalStateException("Headword export is unavailable");
        return result;
    }

    static String intake(String action, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeIntake(action, argument);
        if (result == null) throw new IllegalStateException("Archive import is unavailable");
        return result;
    }

    static boolean setPower(String state) {
        return loaded && nativePower(state);
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

    static String saveLanguage(String body) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeSaveLanguage(body);
        if (result == null) throw new IllegalStateException("Could not save language");
        return result;
    }

    static String systemLogSnapshot() {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeSystemLogSnapshot();
        if (result == null) throw new IllegalStateException("Go runtime is unavailable");
        return result;
    }

    static void configureSystemLog(boolean enabled, boolean baseEvents, boolean webEvents) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        if (nativeSystemLogOptions(enabled, baseEvents, webEvents) == 0) {
            throw new IllegalStateException("Could not configure Go system log");
        }
    }

    static void configureIndexWorkers(int workers) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        if (nativeIndexWorkers(workers) == 0) {
            throw new IllegalStateException("Could not configure indexing concurrency");
        }
    }

    static boolean restoreIndexWorkersIfRunning() {
        return loaded && nativeIndexWorkers(0) != 0;
    }

    static String webResource(String rawURL, String headers) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeWebResource(rawURL, headers);
        if (result == null) throw new IllegalStateException("Web resource is unavailable");
        return result;
    }

    static String mediaOpen(String rawURL, String headers) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        return nativeMediaOpen(rawURL, headers);
    }

    static InputStream mediaStream(long handle, long contentLength) {
        return new InputStream() {
            private boolean closed;
            private boolean finished;
            private long remaining = Math.max(0, contentLength);

            @Override public int read() throws IOException {
                byte[] single = new byte[1];
                return read(single, 0, 1) < 0 ? -1 : single[0] & 0xff;
            }

            @Override public int read(byte[] dst, int offset, int length) throws IOException {
                if (closed) throw new IOException("Media stream is closed");
                if (dst == null) throw new NullPointerException();
                if (offset < 0 || length < 0 || offset > dst.length - length)
                    throw new IndexOutOfBoundsException();
                if (length == 0) return 0;
                if (finished) return -1;
                int n = nativeMediaRead(handle, dst, offset, length);
                if (n > 0) remaining = Math.max(0, remaining - n);
                if (n < 0) {
                    finished = true;
                    nativeMediaClose(handle);
                }
                if (n == -2) throw new IOException("Could not read dictionary media");
                return n;
            }

            @Override public int available() {
                return (int) Math.min(Integer.MAX_VALUE, remaining);
            }

            @Override public void close() {
                if (!closed) {
                    closed = true;
                    if (!finished) nativeMediaClose(handle);
                }
            }
        };
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

    static String userStorage(String method, String argument) {
        if (!loaded) throw new IllegalStateException("Go bridge is unavailable");
        String result = nativeUserStorage(method, argument);
        if (result == null) throw new IllegalStateException("Could not access saved settings");
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
            File path = new File(new File(home, ".dmshlabs/dictshelf"), "dictshelf.toml");
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

    static int startRuntime(Context context, File home, File dbDir) {
        if (!loaded) return -9;
        try {
            JSONObject flags = new JSONObject(ShellPrefs.env(context));
            flags.put("DB_DIR", dbDir.getAbsolutePath());
            flags.put("USE_CACHED", "1");
            flags.put("NO_BROWSER", "1");
            File path = new File(new File(home, ".dmshlabs/dictshelf"), "dictshelf.toml");
            return nativeRuntimeStart(path.getAbsolutePath(), flags.toString(),
                    BuildConfig.VERSION_NAME);
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
