// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.net.Uri;
import android.webkit.ConsoleMessage;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import java.util.concurrent.atomic.AtomicInteger;

/** Bounded, value-free breadcrumbs that survive a native WebView process crash. */
final class WebViewDiagnostics {
    private static final String PREFIX = "wudict-diag ";
    private static final AtomicInteger CONSOLE_ERRORS = new AtomicInteger();
    private static final String SCRIPT = "(function(){"
            + "window.__wudictDiagActive=true;"
            + "if(window.__wudictDiagInstalled)return;window.__wudictDiagInstalled=true;"
            + "const safe=s=>String(s||'').replace(/[^A-Za-z0-9_-]/g,'').slice(0,48);"
            + "function emit(kind,target){if(!window.__wudictDiagActive)return;"
            + "const node=target.closest?.('button,a,input,select,summary,[role=button]')||target;"
            + "const dialog=node.closest?.('dialog');"
            + "const panel=node.closest?.('#panel');"
            + "console.info('wudict-diag '+kind+' tag='+safe(node.tagName).toLowerCase()"
            + "+' id='+safe(node.id)+' class='+safe(node.classList?.[0])"
            + "+' dialog='+safe(dialog?.id)+' panel='+(panel?'settings':'none'));}"
            + "document.addEventListener('click',e=>emit('click',e.target),true);"
            + "document.addEventListener('focusin',e=>{if(e.target.matches?.('input,select,textarea'))emit('focus',e.target)},true);"
            + "document.addEventListener('toggle',e=>{if(e.target.tagName==='DIALOG')emit(e.target.open?'open':'close',e.target)},true);"
            + "})();";

    private WebViewDiagnostics() {}

    static void install(WebView view) {
        view.evaluateJavascript(SystemLog.detailedEnabled()
                ? SCRIPT : "window.__wudictDiagActive=false", null);
    }

    static void page(String window, String stage, String url) {
        SystemLog.record("web " + window + " page " + stage + " path=" + path(url));
    }

    static void error(String window, WebResourceRequest request, WebResourceError error) {
        if (request.isForMainFrame()) SystemLog.recordFailure("web " + window + " load error="
                + error.getErrorCode() + " path=" + path(request.getUrl().toString()));
    }

    static void httpError(String window, WebResourceRequest request, int status) {
        if (request.isForMainFrame()) SystemLog.recordFailure("web " + window + " http="
                + status + " path=" + path(request.getUrl().toString()));
    }

    static boolean console(ConsoleMessage message) {
        String value = message.message();
        if (value == null) return false;
        if (value.startsWith(PREFIX)) {
            String event = value.substring(PREFIX.length());
            if (event.length() <= 240 && event.matches("[A-Za-z0-9_ =-]+"))
                SystemLog.recordDetailed("web ui " + event);
            return true;
        }
        if (SystemLog.detailedEnabled() && message.messageLevel() == ConsoleMessage.MessageLevel.ERROR
                && CONSOLE_ERRORS.getAndIncrement() < 30) {
            String category = value.split(":", 2)[0].replaceAll("[^A-Za-z ]", "").trim();
            if (category.length() > 40) category = category.substring(0, 40);
            SystemLog.recordDetailed("web console error=" + category + " path=" + path(message.sourceId())
                    + " line=" + message.lineNumber());
        }
        return false;
    }

    private static String path(String url) {
        try {
            String value = Uri.parse(url).getPath();
            if (value == null || value.isEmpty() || "/".equals(value)) return "/";
            // Paths may contain article titles or local filenames; log only known routes.
            String first = value.substring(1).split("/", 2)[0];
            if (first.matches("api|assets|browse|setup|lemmas|favicon[.]ico")) return "/" + first;
        } catch (RuntimeException ignored) {}
        return "[other]";
    }
}
