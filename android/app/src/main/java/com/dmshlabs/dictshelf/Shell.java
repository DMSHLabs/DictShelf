// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// What every WebView in this app has in common: the one origin that belongs to
// us, how a URL is built for it, which settings the SPA needs, and where a
// navigation that is NOT ours goes instead.
//
// It lives here rather than in MainActivity because there are now two windows
// onto the same in-process Go runtime - the app and the selection-lookup popup (D67) - and a
// second copy of this policy is a second thing to keep in step. Nothing here
// knows about either activity's layout or lifecycle.
package com.dmshlabs.dictshelf;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Environment;
import android.os.Message;
import android.provider.DocumentsContract;
import android.util.Log;
import android.webkit.JsPromptResult;
import android.webkit.JsResult;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import org.json.JSONObject;

import java.io.File;
import java.io.UnsupportedEncodingException;
import java.net.URLEncoder;

final class Shell {

    private static final String TAG = "wudict";
    static final String STABLE_HOST = "dictshelf.localhost";
    private static final String STABLE_ORIGIN = "https://" + STABLE_HOST;
    private static volatile boolean originPrepared;
    private static volatile boolean legacyOriginRequired = true;

    /**
     * The stable secure origin used after settings have moved out of WebView
     * storage. Until settings.json marks that import complete, keep the old
     * origin so its localStorage can be read without starting a listener.
     */
    static String origin(Context c) {
        return originPrepared && !legacyOriginRequired ? STABLE_ORIGIN : legacyOrigin(c);
    }

    private static String legacyOrigin(Context c) {
        return "http://" + ShellPrefs.LEGACY_HOST + ":" + ShellPrefs.legacyOriginPort(c);
    }

    /** Selects the origin once the in-process Go runtime can inspect settings files. */
    static synchronized void prepareOrigin(Context c) {
        if (originPrepared) return;
        try {
            JSONObject response = new JSONObject(GoBridge.userStorage("storageState", ""));
            JSONObject state = response.optJSONObject("value");
            if (!response.optBoolean("ok") || state == null)
                throw new IllegalStateException("settings state is unavailable");
            legacyOriginRequired = !state.optBoolean("exists");
            originPrepared = true;
            SystemLog.record("WebView origin selected: " +
                    (legacyOriginRequired ? "legacy settings import" : "secure app origin"));
        } catch (Exception error) {
            // Preserving the old origin gives the embedded bootstrap one more
            // chance to import the existing WebView profile if file status is
            // temporarily unavailable.
            legacyOriginRequired = true;
            originPrepared = true;
            Log.w(TAG, "Could not inspect settings files; keeping legacy WebView origin", error);
        }
    }

    /**
     * The origin of the page now on screen, remembered as it is loaded.
     *
     * <p>It is usually {@link #origin}, and it used to BE that by assumption -
     * which broke the moment a row could change the stored port from inside the
     * page: the port is part of {@code origin()}, so Restore defaults (or the
     * port row) moved what the shell considers our own address while the open
     * page went on living at the old one, and every later bridge prompt failed
     * the check below and fell through to the WebView's own prompt dialog -
     * a modal "The page at … says: wudict:appearance" over the page, blocking
     * its JavaScript. Found on the emulator, 2026-09-28, the first time that
     * combination was ever possible.
     *
     * <p>Written on every page start, and read only by the prompt check: the
     * question the check answers is "did this prompt come from a page WE put in
     * this window", and the page in the window is the one that was loaded.
     */
    private static volatile String pageOrigin;

    /** Records where the page in this window came from. Called on page start. */
    static void notePageUrl(String url) {
        int scheme = url == null ? -1 : url.indexOf("://");
        if (scheme < 0) {
            pageOrigin = null; // about:blank and friends are not a page of ours
            return;
        }
        int cut = url.indexOf('/', scheme + 3);
        pageOrigin = cut < 0 ? url : url.substring(0, cut);
    }

    /**
     * Whether a prompt came from this app's own page. Two origins answer yes:
     * the address the shell would build NOW (which is what a page loaded at
     * this moment carries) and the one the page in front of the reader was
     * loaded from (see {@link #pageOrigin}). A third party's page in this
     * window - a dictionary's website that a link somehow reached - is refused
     * by both.
     */
    private static boolean ownPage(Context c, String url) {
        if (url == null) return false;
        if (url.startsWith(origin(c) + "/")) return true;
        String loaded = pageOrigin;
        return loaded != null && url.startsWith(loaded + "/");
    }

    static String pageUrl(Context c) {
        return origin(c) + "/?" + shellQuery(c);
    }

    private Shell() {
    }

    /**
     * A search the page will run on load. `?q=…&mode=…&dict=…` is the SPA's own
     * deep-link shape (web/index.html, applyURL) - the shell adds no API and the
     * page learns nothing about Android (D54).
     *
     * <p>Every component is encoded here, so a caller's text - a selection from
     * another app, a URI from an intent - can carry anything at all.
     */
    static String searchUrl(Context c, String q, String mode, String dict) {
        StringBuilder b = new StringBuilder(origin(c)).append("/?q=").append(enc(q));
        if (mode != null && !mode.isEmpty()) b.append("&mode=").append(enc(mode));
        if (dict != null && !dict.isEmpty()) b.append("&dict=").append(enc(dict));
        return b.append("&").append(shellQuery(c)).toString();
    }

    /**
     * The shell's own parameters for the page, every one of them written as an
     * explicit value: the window's background, and whether the page shows its
     * waiting messages and the "N of M ready" line (ShellPrefs.INFO_MESSAGES;
     * the page's morph note is deliberately outside that switch).
     *
     * <p>On the URL rather than installed afterwards for the reason the key is
     * (see {@link #key}): each of these has to be true at FIRST PAINT - the
     * page paints its background and its waiting art before anything the shell
     * could evaluate later - and an explicit value is what lets a page that
     * remembers one of them (both the background and this are kept in
     * sessionStorage, because a reload re-requests this same URL) hear a
     * withdrawal as clearly as a setting.
     */
    private static String shellQuery(Context c) {
        // shell_bg is the READER's colour and shell_paper the paper an enabled
        // PRESET paints, and the two are separate parameters because they mean
        // different things to the app page: it wears the first (the colour
        // pins --bg and gives its windows the paper tint) and only FORWARDS
        // the second, because it is already painting that paper itself - the
        // preset's layer is its --bg. What needs the paper told is the three
        // standalone pages, which have no layers, and the shell's own windows.
        return "shell_bg=" + (ShellPrefs.sepia(c) ? enc(ShellPrefs.sepiaColorText(c)) : "")
                + "&shell_paper=" + enc(ShellPrefs.paperFor(c, ShellPrefs.night(c)))
                + "&shell_image=" + (WindowBackground.active(c) ? "1" : "0")
                // One inversion, here and nowhere else: the settings screen
                // stores what the user chose to SEE, the page is told what to
                // SUPPRESS.
                + "&shell_quiet=" + (ShellPrefs.infoMessages(c) ? "0" : "1");
    }

    static void applyBackground(WebView web) {
        Context c = web.getContext();
        boolean image = WindowBackground.active(c);
        web.setBackgroundColor(image ? android.graphics.Color.TRANSPARENT : ShellPrefs.pageBg(c));
        String color = ShellPrefs.sepia(c) ? ShellPrefs.sepiaColorText(c) : "";
        String paper = ShellPrefs.paperFor(c, ShellPrefs.night(c));
        // Only a validated six-digit colour is interpolated into JavaScript.
        // The NAME comes from the same theme-aware reader as `active` above:
        // reading the raw key here sent the DAY wallpaper's name while the
        // image was the night one, which is how the Appearance sheet came to
        // wear the day's picture on a dark page.
        //
        // The fourth argument is the PRESET's paper, and it is not a second
        // colour for this document: the app page paints that paper with its
        // own layer and only forwards it, while the three standalone pages -
        // no layers, no --bg of their own - wear it as their ground. One call
        // shape, and each document's hook is what differs.
        web.evaluateJavascript("window.wudictShellBackground && window.wudictShellBackground('"
                + color + "'," + image + "," + org.json.JSONObject.quote(image
                ? ShellPrefs.backgroundImage(c) : "") + "," + org.json.JSONObject.quote(paper)
                + ")", null);
        // wudictNativeShell exposes the page's native controls; the Android
        // marker reserves the wudict:appearance prompt bridge for this host.
        // The flag first, then the two calls that need it: the page reports the
        // paper an enabled preset paints only where a shell answers, and the
        // report is what keeps the standalone pages and the native windows in
        // step with the app page. It is asked for here rather than left to the
        // page's own boot push because the flag arrives at onPageFinished,
        // which either side of that push can win.
        web.evaluateJavascript("window.wudictAndroidShell=1;window.wudictNativeShell=1;"
                + "if(typeof wudictPresetPaper==='function')wudictPresetPaper();"
                + "if(typeof appearanceRead==='function' && document.getElementById('styler')"
                + " && document.getElementById('styler').classList.contains('show')) appearanceRead();"
                + NATIVE_SELECT_PICKER_JS, null);
    }

    // What the settings screen sends after emptying the browser's cache: the
    // window that shows the page belongs to another Activity, so it is asked to
    // load the page again rather than reached into.
    static final String EXTRA_RELOAD = "wudict.reload";

    /**
     * Empties the WebView's resource cache - the copies of the page's own files
     * (HTML, CSS, JS, the favicon) it keeps on the device.
     *
     * Only that. The dictionaries and the prepared library are the server's
     * files on disk, and localStorage (theme, wide mode, which group the picker
     * is on) is the reader's state, not a cache: neither is touched, which is
     * why this needs no confirmation and changes no setting.
     *
     * clearCache() is per-APPLICATION despite being an instance method - the
     * WebView documentation says so outright - so a throwaway WebView is the
     * whole requirement here.
     */
    static void clearWebCache(Context c) {
        WebView probe = new WebView(c);
        probe.clearCache(true);
        probe.destroy();
    }

    /** Clear cached pages once for each installed APK version, before a page loads. */
    static synchronized void clearWebCacheAfterUpdate(Context c) {
        android.content.SharedPreferences prefs = c.getSharedPreferences("shell", Context.MODE_PRIVATE);
        int version = BuildConfig.VERSION_CODE;
        if (prefs.getInt("web_cache_version", -1) == version) return;
        clearWebCache(c);
        prefs.edit().putInt("web_cache_version", version).commit();
    }

    /**
     * The app page's System window: every row of the shell's own settings
     * screen, as the page draws them (index.html, #sysSettings).
     *
     * <p>The page asks through {@code wudict:system} and receives Android shell
     * preferences, diagnostics settings and app-tuning rows. Server controls
     * are hidden because Android has no HTTP listener. Row labels travel with
     * the values because they come from Android string resources.
     *
     * <p>Effective config values are read from the in-process Go runtime through
     * JNI; Android does not make an HTTP request.
     */
    static org.json.JSONObject systemState(Context c) throws org.json.JSONException {
        org.json.JSONObject reply = new org.json.JSONObject();
        reply.put("toolbar", ShellPrefs.opensApp(c, ShellPrefs.TOOLBAR));
        reply.put("share", ShellPrefs.opensApp(c, ShellPrefs.SHARE));
        reply.put("link", ShellPrefs.opensApp(c, ShellPrefs.LINK));
        reply.put("reader", ShellPrefs.opensApp(c, ShellPrefs.READER));
        reply.put("info", ShellPrefs.infoMessages(c));
        reply.put("serverAvailable", false);
        org.json.JSONObject prefs = new org.json.JSONObject(GoBridge.prefsSnapshot());
        org.json.JSONObject uiPrefs = prefs.optJSONObject("ui");
        reply.put("updateChannel", uiPrefs == null ? "release" : uiPrefs.optString("updateChannel", "release"));
        reply.put("updateNotifications", uiPrefs == null || !uiPrefs.optBoolean("updateNotificationsOff", false));
        reply.put("logEnabled", SystemLog.enabled());
        reply.put("logBaseEvents", SystemLog.baseEvents());
        reply.put("logWebEvents", SystemLog.detailedEvents());
        org.json.JSONArray rows = new org.json.JSONArray();
        for (ShellPrefs.Override o : ShellPrefs.OVERRIDES) {
            org.json.JSONObject row = new org.json.JSONObject();
            row.put("key", o.key);
            row.put("kind", o.kind == ShellPrefs.BOOL ? "bool"
                    : o.kind == ShellPrefs.MEGABYTES ? "mb" : "count");
            row.put("label", c.getString(o.label));
            row.put("hint", c.getString(o.hint));
            row.put("min", o.min);
            row.put("max", ShellPrefs.maxOf(c, o));
            row.put("onValue", o.onValue == null ? org.json.JSONObject.NULL : o.onValue);
            String stored = ShellPrefs.override(c, o);
            row.put("stored", stored == null ? org.json.JSONObject.NULL : stored);
            // The value this Android app contributes for the Go runtime.
            String emitted = ShellPrefs.emitted(c, o);
            row.put("emitted", emitted == null ? org.json.JSONObject.NULL : emitted);
            rows.put(row);
        }
        reply.put("rows", rows);
        return reply;
    }

    // Keep the real select as the source of truth, including streamed options,
    // groups, disabled entries and its existing change handler.
    static final String NATIVE_SELECT_PICKER_JS = """
            (() => {
              function installPickers() {
              for (const select of document.querySelectorAll('#mode, #articleFindMode, #stylerPreset, #groupSelect, #groupAvailableFilter, #sysAdvanced select, #bulkIndexes select')) {
              const id = select.id || 'bulkIndexAction';
              if (!select || select.dataset.shellPicker) continue;
              select.dataset.shellPicker = '1';
              function open() {
                if (select.disabled) return;
                const options = Array.from(select.options), rows = [];
                let group = null;
                for (let index = 0; index < options.length; index++) {
                  const option = options[index];
                  if (id === 'stylerPreset' && !option.value) continue;
                  const parent = option.parentElement;
                  if (parent.tagName === 'OPTGROUP' && parent !== group) {
                    rows.push({label: parent.label, index: -1, disabled: true});
                  }
                  group = parent;
                  rows.push({label: option.textContent, index,
                    disabled: option.disabled || (parent.tagName === 'OPTGROUP' && parent.disabled)});
                }
                const answer = window.prompt('wudict:native-select-picker',
                  JSON.stringify({rows, selected: select.selectedIndex,
                    kind: id === 'articleFindMode' ? 'mode' : id}));
                if (answer === null) return;
                const index = Number(answer);
                if (!Number.isInteger(index) || !options[index]) return;
                if (index !== select.selectedIndex) {
                  select.selectedIndex = index;
                  select.dispatchEvent(new Event('change', {bubbles:true}));
                }
              }
              let pointer = null, pointerClick = false, opening = false;
              function scheduleOpen() {
                if (opening) return;
                opening = true;
                // Let WebView finish the gesture before a native window takes focus.
                setTimeout(() => { try { open(); } finally { opening = false; } }, 0);
              }
              select.addEventListener('pointerdown', event => {
                if (event.button !== 0) return;
                event.preventDefault();
                pointer = {id:event.pointerId, x:event.clientX, y:event.clientY};
                pointerClick = true;
              });
              select.addEventListener('pointerup', event => {
                const start = pointer; pointer = null;
                if (!start || start.id !== event.pointerId) return;
                event.preventDefault();
                if (select.hasPointerCapture(event.pointerId)) select.releasePointerCapture(event.pointerId);
                if (Math.hypot(event.clientX-start.x, event.clientY-start.y) < 12) scheduleOpen();
              });
              select.addEventListener('pointercancel', () => {
                pointer = null;
              });
              // Accessibility activation can arrive as a click without a pointer.
              select.addEventListener('click', event => {
                event.preventDefault();
                if (pointerClick) { pointerClick = false; return; }
                if (event.detail === 0) scheduleOpen();
              });
              select.addEventListener('keydown', event => {
                if (['Enter',' ','ArrowDown','ArrowUp'].includes(event.key)) {
                  event.preventDefault();
                  pointerClick = false;
                  if (!event.repeat) scheduleOpen();
                }
              });
              }
              }
              installPickers();
              // Bulk action selects are recreated when dictionary settings refresh.
              const bulk = document.getElementById('bulkIndexes');
              if (bulk) new MutationObserver(installPickers).observe(bulk, {childList:true, subtree:true});
              const groupRows = document.getElementById('groupRows');
              if (groupRows) new MutationObserver(installPickers).observe(groupRows, {childList:true, subtree:true});
            })();
            """;

    // URLEncoder writes ' ' as '+', which URLSearchParams reads back as ' ' -
    // the same pair the page already uses for its own links.
    private static String enc(String s) {
        try {
            return URLEncoder.encode(s, "UTF-8");
        } catch (UnsupportedEncodingException e) {
            return ""; // UTF-8 is guaranteed by the platform; this branch is unreachable
        }
    }

    /** The settings the SPA needs. Identical in every window, by construction. */
    static void configure(WebView web) {
        web.addJavascriptInterface(new GoPageBridge(), "wudictAndroidGo");
        web.getSettings().setJavaScriptEnabled(true);                 // the UI is one SPA
        web.getSettings().setDomStorageEnabled(true);                 // prefs live in localStorage
        web.getSettings().setMediaPlaybackRequiresUserGesture(false); // dictionary audio
        // D51 hands an article's external links to the page, which opens them
        // with window.open(). A WebView has no tabs, so that call is inert
        // unless multiple windows are supported and onCreateWindow answers it.
        web.getSettings().setSupportMultipleWindows(true);
    }

    private static final class GoPageBridge {
        @JavascriptInterface public boolean isDebugBuild() {
            return BuildConfig.DEBUG;
        }

        @JavascriptInterface public int startDictStream(boolean progress) {
            return GoBridge.startDictStream(progress);
        }

        @JavascriptInterface public int startRescanStream() {
            return GoBridge.startRescanStream();
        }

        @JavascriptInterface public int startRescanIndexes(String origin, String request) {
            return GoBridge.startRescanIndexes(origin, request);
        }

        @JavascriptInterface public int startIngest(String origin, String query) {
            return GoBridge.startIngest(origin, query);
        }

        @JavascriptInterface public int startSearchStream(String origin, String request) {
            return GoBridge.startSearchStream(origin, request);
        }

        @JavascriptInterface public int startBrowseStream(String method, String query) {
            return GoBridge.startBrowseStream(method, query);
        }

        @JavascriptInterface public String pollDictStream(int id) {
            return GoBridge.pollDictStream(id);
        }

        @JavascriptInterface public void cancelDictStream(int id) {
            GoBridge.cancelDictStream(id);
        }

        @JavascriptInterface public String config() {
            return GoBridge.configSnapshot();
        }

        @JavascriptInterface public String prefs() {
            return GoBridge.prefsSnapshot();
        }

        @JavascriptInterface public String browsePage(String query) {
            return GoBridge.browsePage(query);
        }

        @JavascriptInterface public String browseFindPage(String query) {
            return GoBridge.browseFindPage(query);
        }

        @JavascriptInterface public String browseDiagnostic(String payload) {
            return GoBridge.browseDiagnostic(payload);
        }

        @JavascriptInterface public String about(String origin, String dictionary) {
            return GoBridge.about(origin, dictionary);
        }

        @JavascriptInterface public String orphans(String action, String body) {
            return GoBridge.orphans(action, body);
        }

        @JavascriptInterface public String removeLibrary(String query) {
            return GoBridge.removeLibrary(query);
        }

        @JavascriptInterface public String looks(String argument) {
            return GoBridge.looks(argument);
        }

        @JavascriptInterface public String groups(String action, String argument) {
            return GoBridge.groups(action, argument);
        }

        @JavascriptInterface public String lemmas(String action, String code) {
            return GoBridge.lemmas(action, code);
        }

        @JavascriptInterface public String setup(String action, String argument) {
            return GoBridge.setup(action, argument);
        }
        @JavascriptInterface public String headwordExport(String action, String argument) {
            return GoBridge.headwordExport(action, argument);
        }

        @JavascriptInterface public String intake(String action, String argument) {
            return GoBridge.intake(action, argument);
        }

        @JavascriptInterface public String reindex(String action) {
            return GoBridge.reindex(action);
        }

        @JavascriptInterface public String indexWork(String action, String id, String body) {
            return GoBridge.indexWork(action, id, body);
        }

        @JavascriptInterface public String presets() {
            return GoBridge.presetsSnapshot();
        }

        @JavascriptInterface public String savePreset(String body) {
            return GoBridge.savePreset(body);
        }

        @JavascriptInterface public String savePrefs(String body) {
            return GoBridge.savePrefs(body);
        }

        @JavascriptInterface public String saveLanguage(String body) {
            return GoBridge.saveLanguage(body);
        }

        @JavascriptInterface public String style() {
            return GoBridge.styleSnapshot();
        }

        @JavascriptInterface public String saveStyle(String argument) {
            return GoBridge.saveStyle(argument);
        }

        @JavascriptInterface public String userFiles() {
            return GoBridge.userFilesSnapshot();
        }

        @JavascriptInterface public String userFileOperation(String method, String argument) {
            return GoBridge.userFileOperation(method, argument);
        }

        @JavascriptInterface public String userStorage(String method, String argument) {
            return GoBridge.userStorage(method, argument);
        }
    }

    /**
     * Reports whether it took the navigation off the WebView's hands. Our own
     * origin stays; a dictionary's link to a real website goes to whatever the
     * user browses with, because a WebView with no address bar, no tabs and no
     * back affordance is a trap to land a website in.
     */
    static boolean openExternal(Activity a, Uri uri) {
        if (uri == null) return false;
        String scheme = uri.getScheme();
        if (scheme == null) return false;
        scheme = scheme.toLowerCase();
        // The article machinery's own URLs: srcdoc frames, data: media, the
        // blob: URLs audio playback builds. None of those leave the app.
        if (scheme.equals("data") || scheme.equals("blob")
                || scheme.equals("about") || scheme.equals("javascript")) {
            return false;
        }
        String url = uri.toString();
        // Our own pages stay: the origin the shell builds now, or the one the
        // open page was loaded from when a stored port change moved the first
        // (see pageOrigin) - without the second, the app's own links would be
        // handed to the system browser for as long as that mismatch lasts.
        if (ownPage(a, url)) return false;
        // Shell-private URLs (wudict://…) are a channel from the page to the
        // Java side that costs no JavascriptInterface and no server API: this
        // method already inspects every navigation, so the branch is free.
        // Intake first, then the flavour's Storage - Intake owns the one
        // host that means the same thing in both flavours and delegates the
        // rest unchanged.
        if (Intake.handleShellUri(a, uri)) return true;
        try {
            a.startActivity(new Intent(Intent.ACTION_VIEW, uri)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        } catch (ActivityNotFoundException | SecurityException e) {
            SystemLog.warn(TAG, "no app can open " + uri, e);
        }
        return true;
    }

    // The file picker <input type="file"> needs (D123: the styler's "Add…"
    // button). A WebView answers a file input with nothing whatsoever unless a
    // WebChromeClient implements onShowFileChooser - the shell's job, not the
    // page's.
    //
    // Distinct from Storage.REQ_TREE: both flavours' onActivityResult and this
    // one are called for every result the activity receives.
    private static final int REQ_FILES = 0x5AF1;

    // The callback the page is blocked on. Static because the picker is another
    // activity and ours may be stopped - or, for the lookup popup (D67),
    // recreated - before the answer lands; one file input can be open at a
    // time, so a single slot is the entire state.
    private static ValueCallback<Uri[]> pendingFiles;

    /**
     * The setup page's folder button (wudict:folder-picker): the system's own
     * folder dialog, answering with a REAL PATH the exec'd server can read -
     * the one thing the web's showDirectoryPicker cannot name, and it does
     * not exist in a WebView at all. Distinct from Storage.REQ_TREE,
     * REQ_FILES and Intake.REQ_PICK: every result reaches all four handlers.
     */
    private static final int REQ_DIR = 0x5AF3;

    // The prompt the page is blocked on, the same single-slot bargain as
    // pendingFiles: one folder picker open at a time.
    private static JsPromptResult pendingDir;

    /**
     * Answers the folder prompt, exactly once: the picked path, or a cancel
     * when the user backed out or named something this shell cannot turn into
     * a path. What is NOT legal is silence - an unanswered prompt leaves the
     * page's JavaScript blocked forever.
     */
    private static void settleDir(String path) {
        JsPromptResult cb = pendingDir;
        pendingDir = null;
        if (cb == null) return;
        if (path != null) cb.confirm(path);
        else cb.cancel();
    }

    /**
     * Answers the page's file input, exactly once, with whatever we have.
     *
     * <p>Null is a legitimate answer and means "cancelled". What is NOT legal is
     * silence: an unanswered callback leaves that input permanently dead, so
     * every exit from the chooser path runs through here.
     */
    private static void settleFiles(Uri[] uris) {
        ValueCallback<Uri[]> cb = pendingFiles;
        pendingFiles = null;
        if (cb != null) cb.onReceiveValue(uris);
    }

    /**
     * The pickers' answers, forwarded by whichever activity hosts the WebView.
     * parseResult handles single, multiple (clipData) and cancel alike.
     */
    static void onActivityResult(Activity a, int requestCode, int resultCode, Intent data) {
        SystemLogExport.result(a, requestCode, resultCode, data);
        if (requestCode == REQ_FILES) {
            settleFiles(WebChromeClient.FileChooserParams.parseResult(resultCode, data));
            return;
        }
        if (requestCode != REQ_DIR) return;
        Uri tree = resultCode == Activity.RESULT_OK && data != null ? data.getData() : null;
        settleDir(tree == null ? null : treePath(a, tree));
    }

    /**
     * The folder a tree pick names, as a path on the filesystem, or null when
     * there is no such thing - the ordinary answer for a provider that is not
     * a plain volume (a cloud document, say). Only the externalstorage
     * provider is translated, by the same rule Intake.realPath applies to
     * files: the primary volume is Environment's directory, and any other
     * volume id IS its mount name under /storage. Tested as a DIRECTORY and
     * for readability, because a path the exec'd server cannot open would
     * only move the failure somewhere the user cannot see it answered.
     */
    private static String treePath(Context c, Uri tree) {
        if (!"content".equalsIgnoreCase(tree.getScheme())
                || !"com.android.externalstorage.documents".equals(tree.getAuthority())) {
            return null;
        }
        try {
            String id = DocumentsContract.getTreeDocumentId(tree); // "primary:Download"
            int cut = id.indexOf(':');
            if (cut <= 0) return null;
            String volume = id.substring(0, cut), rel = id.substring(cut + 1);
            if ("primary".equalsIgnoreCase(volume)) {
                String p = readableDir(new File(
                        Environment.getExternalStorageDirectory(), rel).getPath());
                if (p != null) return p;
            }
            // A microSD card, whose volume id IS its mount name.
            return readableDir("/storage/" + volume + "/" + rel);
        } catch (Exception e) {
            SystemLog.warn(TAG, "cannot resolve " + tree, e);
            return null;
        }
    }

    private static String readableDir(String p) {
        if (p == null) return null;
        File f = new File(p);
        return f.isDirectory() && f.canRead() ? f.getAbsolutePath() : null;
    }

    /** Answers window.open(): reads the URL the new window wants, then sends it out. */
    static WebChromeClient windows(Activity a) {
        return new WebChromeClient() {
            @Override
            public boolean onConsoleMessage(android.webkit.ConsoleMessage message) {
                return WebViewDiagnostics.console(message);
            }

            @Override
            public boolean onJsPrompt(WebView view, String url, String message,
                                      String defaultValue, android.webkit.JsPromptResult result) {
                if (!ownPage(a, url)) return false;
                if ("wudict:system-log".equals(message)) {
                    try {
                        SystemLogExport.start(a);
                        result.confirm("ok");
                    } catch (RuntimeException e) {
                        result.confirm(e.getMessage());
                    }
                    return true;
                }
                if ("wudict:exit".equals(message)) {
                    result.confirm("ok");
                    a.runOnUiThread(() -> AppExit.request(a));
                    return true;
                }
                if ("wudict:appearance".equals(message)) {
                    try {
                        org.json.JSONObject request = new org.json.JSONObject(defaultValue);
                        String action = request.optString("action", "get");
                        // Which THEME's background this round trip is about. The
                        // page says so rather than letting the shell infer it
                        // from pageDark: the theme watcher travels as a
                        // subresource request, so inferring would race a switch
                        // - the reader can flip the theme and set a colour in
                        // the same breath. Absent means "the theme the window is
                        // already showing", which is what a page that predates
                        // this sends and what it means.
                        boolean night = request.optBoolean("night", ShellPrefs.night(a));
                        if ("set".equals(action)) {
                            String field = request.getString("field");
                            // The window's own two rows, moved here from the
                            // settings screen: they change the SHELL's window,
                            // not the page, so they are stored here and applied
                            // through the activity that owns one. In the
                            // floating lookup window there is nothing to apply
                            // them to - a popup does not own the system bars -
                            // and the value simply awaits the app window.
                            boolean window = false;
                            // Whether the write left the screen different.
                            // Only the paper says so; every other field here
                            // repaints, because a write that changes nothing
                            // is what the page's own controls never send.
                            boolean changed = true;
                            if ("colorEnabled".equals(field)) {
                                ShellPrefs.setSepia(a, night, request.getBoolean("value"));
                            } else if ("color".equals(field)) {
                                ShellPrefs.setSepiaColor(a, night, request.getString("value"));
                            } else if ("image".equals(field)) {
                                String name = request.getString("value");
                                if (!name.isEmpty() && !WindowBackground.images(a).contains(name))
                                    throw new IllegalArgumentException("Image unavailable");
                                ShellPrefs.setBackgroundImage(a, night, name);
                                // A selected image always uses its sampled
                                // paper color. Match the shared settings UI:
                                // selecting an image turns Color on, and
                                // clearing it leaves Color enabled and editable.
                                if (!name.isEmpty()) ShellPrefs.setSepia(a, night, true);
                            } else if ("edgeMode".equals(field)) {
                                int mode = request.optInt("value", -1);
                                if (mode < ShellPrefs.EDGE_SYSTEM || mode > ShellPrefs.EDGE_NONE)
                                    throw new IllegalArgumentException("Unknown edge mode");
                                ShellPrefs.setEdgeMode(a, mode);
                                window = true;
                            } else if ("edgeColor".equals(field)) {
                                // parseColor takes #RGB, #RRGGBB and #AARRGGBB
                                // alike; the alpha is forced opaque on the way
                                // in, and a string it cannot read throws.
                                ShellPrefs.setEdgeColorValue(a,
                                        android.graphics.Color.parseColor(request.getString("value")));
                                window = true;
                            } else if ("bars".equals(field)) {
                                int mask = request.optInt("value", -1);
                                if (mask < 0 || mask > ShellPrefs.BARS_BOTH)
                                    throw new IllegalArgumentException("Unknown bar mask");
                                ShellPrefs.setBars(a, mask);
                                window = true;
                            } else if ("paper".equals(field)) {
                                // The paper an enabled PRESET paints, reported
                                // by the page (index.html: presetPaperPush).
                                // Not a row anywhere: the reader switched the
                                // layer on, and that is what has to be undone
                                // to change it. A value that did not change
                                // repaints nothing - this arrives on every
                                // layer load, and the page's own push would
                                // otherwise be answered by another one.
                                changed = ShellPrefs.setPaper(a, night,
                                        request.getString("value"));
                            } else throw new IllegalArgumentException("Unknown field");
                            if (window) {
                                if (a instanceof MainActivity) ((MainActivity) a).refreshScreen();
                            } else if (changed) {
                                if (a instanceof MainActivity) {
                                    ((MainActivity) a).refreshAppearance();
                                } else if (a instanceof LookupActivity) {
                                    ((LookupActivity) a).refreshAppearance();
                                }
                            }
                        } else if (!"get".equals(action)) throw new IllegalArgumentException("Unknown action");
                        org.json.JSONObject reply = new org.json.JSONObject();
                        // The THEME's own values: the page asks about the theme
                        // it is showing, so a sheet opened at night never shows
                        // the day colour while the window wears the night one.
                        reply.put("colorEnabled", ShellPrefs.sepiaFor(a, night));
                        reply.put("color", ShellPrefs.sepiaColorTextFor(a, night));
                        reply.put("image", ShellPrefs.backgroundImageFor(a, night));
                        // What the enabled preset paints, so a page can tell
                        // whether its own report landed. Not a control: the
                        // sheet draws the reader's colour, not this.
                        reply.put("paper", ShellPrefs.paperFor(a, night));
                        org.json.JSONArray images = new org.json.JSONArray();
                        for (String name : WindowBackground.images(a)) images.put(name);
                        reply.put("images", images);
                        reply.put("edgeMode", ShellPrefs.edgeMode(a));
                        reply.put("edgeColor",
                                String.format("#%06X", 0xFFFFFF & ShellPrefs.edgeColorValue(a)));
                        reply.put("bars", ShellPrefs.bars(a));
                        result.confirm(reply.toString());
                    } catch (org.json.JSONException | IllegalArgumentException bad) {
                        result.cancel();
                    }
                    return true;
                }
                // The app page's System window: the shell's own settings,
                // drawn by the page. One prompt carries the whole round trip -
                // a read, a write, Restore defaults and Clear browser cache -
                // because all four answer with the same thing (the state after
                // the change), which is what keeps the page from holding a
                // copy of a value the shell owns.
                //
                // Clear browser cache is the one action whose second half is
                // not state at all: the window that shows the page has to load
                // it again, and that window is an Activity - asked, not reached
                // into, exactly as the settings screen asks it through
                // EXTRA_RELOAD. Here the reader is IN the window, so it is the
                // hosting one that reloads, whether that is the app or the
                // popup.
                if ("wudict:system".equals(message)) {
                    try {
                        org.json.JSONObject request = new org.json.JSONObject(defaultValue);
                        String action = request.optString("action", "get");
                        if ("set".equals(action)) {
                            String field = request.getString("field");
                            if ("log".equals(field)) {
                                SystemLog.configure(a, request.getBoolean("enabled"),
                                        request.getBoolean("baseEvents"), request.getBoolean("webEvents"));
                                GoBridge.configureSystemLog(request.getBoolean("enabled"),
                                        request.getBoolean("baseEvents"), request.getBoolean("webEvents"));
                                if (request.getBoolean("enabled")) ProcessExitDiagnostics.recordRecent(a);
                            } else if ("toolbar".equals(field)) {
                                ShellPrefs.set(a, ShellPrefs.TOOLBAR, request.getBoolean("value"));
                            } else if ("share".equals(field)) {
                                ShellPrefs.set(a, ShellPrefs.SHARE, request.getBoolean("value"));
                            } else if ("link".equals(field)) {
                                ShellPrefs.set(a, ShellPrefs.LINK, request.getBoolean("value"));
                            } else if ("reader".equals(field)) {
                                ShellPrefs.set(a, ShellPrefs.READER, request.getBoolean("value"));
                            } else if ("info".equals(field)) {
                                ShellPrefs.set(a, ShellPrefs.INFO_MESSAGES, request.getBoolean("value"));
                            } else if ("updateChannel".equals(field)) {
                                String channel = request.getString("value");
                                if (!java.util.Arrays.asList("release", "prerelease", "beta").contains(channel))
                                    throw new IllegalArgumentException("Unknown update channel");
                                org.json.JSONObject patch = new org.json.JSONObject().put("ui", new org.json.JSONObject().put("updateChannel", channel));
                                if (!new org.json.JSONObject(GoBridge.savePrefs(patch.toString())).optBoolean("ok"))
                                    throw new IllegalStateException("Could not save update channel");
                            } else if ("updateNotifications".equals(field)) {
                                boolean enabled = request.getBoolean("value");
                                org.json.JSONObject patch = new org.json.JSONObject().put("ui",
                                        new org.json.JSONObject().put("updateNotificationsOff", !enabled));
                                if (!new org.json.JSONObject(GoBridge.savePrefs(patch.toString())).optBoolean("ok"))
                                    throw new IllegalStateException("Could not save update notification setting");
                            } else if ("override".equals(field)) {
                                ShellPrefs.Override o = ShellPrefs.byKey(request.getString("key"));
                                // An absent or null value is "follow the
                                // settings file", which is a write like any
                                // other and not an error.
                                String value = request.isNull("value") ? null
                                        : request.optString("value", "");
                                ShellPrefs.setOverrideChecked(a, o, value);
                            } else throw new IllegalArgumentException("Unknown field");
                        } else if ("restore".equals(action)) {
                            ShellPrefs.clearOverrides(a);
                        } else if ("clearCache".equals(action)) {
                            clearWebCache(a);
                            if (a instanceof MainActivity) ((MainActivity) a).reloadPage();
                            else if (a instanceof LookupActivity) ((LookupActivity) a).reloadPage();
                        } else if (!"get".equals(action)) {
                            throw new IllegalArgumentException("Unknown action");
                        }
                        result.confirm(systemState(a).toString());
                    } catch (org.json.JSONException | RuntimeException bad) {
                        // Nothing is said here: the page holds the sentence a
                        // refused value deserves (its own field, its own
                        // words), and a cancel is how it learns to say it.
                        result.cancel();
                    }
                    return true;
                }
                // The setup page's 📁. The page asks through the prompt it is
                // then blocked on; startActivityForResult answers it from
                // onActivityResult, so returning true here is "the answer
                // comes later", the same bargain NativeSelectPicker makes.
                if ("wudict:folder-picker".equals(message)) {
                    // A second request supersedes the first; the abandoned
                    // prompt still gets its answer.
                    settleDir(null);
                    pendingDir = result;
                    Intent i = new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
                            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                    try {
                        a.startActivityForResult(i, REQ_DIR);
                    } catch (ActivityNotFoundException | SecurityException e) {
                        SystemLog.warn(TAG, "no folder picker on this device", e);
                        pendingDir = null;
                        result.cancel();
                    }
                    return true;
                }
                if (!"wudict:native-select-picker".equals(message)) return false;
                NativeSelectPicker.show(a, defaultValue, result);
                return true;
            }

            @Override
            public boolean onCreateWindow(WebView view, boolean isDialog,
                                          boolean isUserGesture, Message resultMsg) {
                // window.open() has no URL in this callback - the only way to
                // learn it is to hand the transport a throwaway WebView and read
                // the navigation it is about to make.
                WebView sink = new WebView(view.getContext());
                sink.setWebViewClient(new WebViewClient() {
                    @Override
                    public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest req) {
                        openExternal(a, req.getUrl());
                        v.post(v::destroy); // never destroy a WebView inside its own callback
                        return true;
                    }
                });
                ((WebView.WebViewTransport) resultMsg.obj).setWebView(sink);
                resultMsg.sendToTarget();
                return true;
            }

            @Override
            @SuppressWarnings("deprecation") // startActivityForResult: no androidx here, by design
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback,
                                             FileChooserParams params) {
                // A second request supersedes the first; the abandoned one still
                // has to be answered or its input stays dead.
                settleFiles(null);
                pendingFiles = callback;
                try {
                    // createIntent() carries the accept= types and the multiple
                    // flag the page asked for.
                    Intent picker = params.createIntent();
                    // Android's picker treats WebView's extension list as MIME
                    // types. It turns the archive/dictionary input's first
                    // extension into application/zip, which makes visible
                    // .dsl/.mdx files unselectable. These inputs are validated
                    // by Go after selection, so let the document provider show
                    // all openable files when the page supplied extensions.
                    boolean hasAcceptFilter = false;
                    for (String accept : params.getAcceptTypes()) {
                        if (accept != null && !accept.trim().isEmpty()
                                && !"*/*".equals(accept.trim())) {
                            hasAcceptFilter = true;
                            break;
                        }
                    }
                    if (hasAcceptFilter) {
                        picker.setType("*/*");
                        picker.removeExtra(Intent.EXTRA_MIME_TYPES);
                    }
                    a.startActivityForResult(picker, REQ_FILES);
                } catch (ActivityNotFoundException | SecurityException e) {
                    SystemLog.warn(TAG, "no file picker on this device", e);
                    settleFiles(null);
                }
                // True either way: the callback is ours now, and it has already
                // been answered on the failure path.
                return true;
            }

            // The page's confirm() and alert() get the shared themed surface,
            // not the platform default: the Files list's "Delete X?" must sit
            // on the same wallpaper and palette as the pickers it appears
            // beside, or it reads as having left the app. An unanswered
            // JsResult holds the page's JavaScript, so every exit - buttons,
            // back, a window that could not be shown - answers it exactly once.
            @Override
            public boolean onJsConfirm(WebView view, String url, String message,
                                       JsResult result) {
                try {
                    new BackgroundDialogBuilder(a)
                            .setMessage(message)
                            .setPositiveButton(android.R.string.ok,
                                    (d, w) -> result.confirm())
                            .setNegativeButton(android.R.string.cancel,
                                    (d, w) -> result.cancel())
                            .setOnCancelListener(d -> result.cancel())
                            .show();
                } catch (RuntimeException badWindow) {
                    result.cancel();
                }
                return true;
            }

            @Override
            public boolean onJsAlert(WebView view, String url, String message,
                                     JsResult result) {
                try {
                    new BackgroundDialogBuilder(a)
                            .setMessage(message)
                            .setPositiveButton(android.R.string.ok,
                                    (d, w) -> result.confirm())
                            .setOnCancelListener(d -> result.cancel())
                            .show();
                } catch (RuntimeException badWindow) {
                    result.cancel();
                }
                return true;
            }
        };
    }
}
