// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.net.Uri;
import android.util.Base64;
import android.util.Log;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;

import org.json.JSONObject;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.LinkedHashMap;
import java.util.Map;

/** Serves app pages and dictionary resources from the in-process Go runtime. */
final class GoAppResource {
    private GoAppResource() {}

    static WebResourceResponse intercept(WebResourceRequest request) {
        Uri source = request.getUrl();
        String host = source.getHost();
        String scheme = source.getScheme();
        String path = source.getPath();
        String method = request.getMethod();
        boolean legacy = "127.0.0.1".equalsIgnoreCase(host) && "http".equalsIgnoreCase(scheme);
        boolean current = Shell.STABLE_HOST.equalsIgnoreCase(host) && "https".equalsIgnoreCase(scheme);
        boolean appPage = "/".equals(path) || "/browse".equals(path) || "/setup".equals(path) ||
                "/lemmas".equals(path) || "/groups".equals(path) ||
                "/double-tap-probe".equals(path);
        boolean appResource = path != null && (path.startsWith("/assets/") ||
                path.equals("/favicon.ico") || path.startsWith("/style/") ||
                path.startsWith("/files/") || path.startsWith("/res/"));
        if ((!legacy && !current) || path == null || !"GET".equalsIgnoreCase(method) ||
                !(appPage || appResource)) return null;

        boolean streamResource = path.startsWith("/files/") || path.startsWith("/res/");
        Uri.Builder direct = new Uri.Builder().scheme("https").authority(Shell.STABLE_HOST)
                .encodedPath(source.getEncodedPath());
        if (source.getEncodedQuery() != null) direct.encodedQuery(source.getEncodedQuery());
        try {
            String headers = resourceHeaders(request);
            String rawResult = streamResource
                    ? GoBridge.mediaOpen(direct.build().toString(), headers)
                    : GoBridge.webResource(direct.build().toString(), headers);
            JSONObject result = new JSONObject(rawResult);
            if (!result.optBoolean("handled", false)) return null;
            String mime = "application/octet-stream", encoding = "UTF-8";
            Map<String, String> responseHeaders = new LinkedHashMap<>();
            long contentLength = 0;
            String rawHeaders = result.optString("headers", "");
            for (String line : rawHeaders.split("\\r?\\n")) {
                int colon = line.indexOf(':');
                if (colon <= 0) continue;
                String name = line.substring(0, colon).trim();
                String value = line.substring(colon + 1).trim();
                if ("Content-Type".equalsIgnoreCase(name)) {
                    String[] parts = value.split(";", 2);
                    mime = parts[0].trim();
                    if (parts.length == 2 && parts[1].toLowerCase().contains("charset="))
                        encoding = parts[1].substring(parts[1].toLowerCase().indexOf("charset=") + 8).trim();
                } else if ("Content-Length".equalsIgnoreCase(name)) {
                    try { contentLength = Long.parseLong(value); }
                    catch (NumberFormatException ignored) { contentLength = 0; }
                } else {
                    responseHeaders.put(name, value);
                }
            }
            int status = result.optInt("status", 200);
            String reason = status == 200 ? "OK" : status == 206 ? "Partial Content" :
                    status == 304 ? "Not Modified" : status == 404 ? "Not Found" :
                    status == 416 ? "Range Not Satisfiable" : "Embedded asset response";
            InputStream body = streamResource
                    ? GoBridge.mediaStream(result.getLong("stream"), contentLength)
                    : new ByteArrayInputStream(Base64.decode(result.optString("body", ""), Base64.DEFAULT));
            if (streamResource) {
                long rangePrefix = status == 206
                        ? contentRangeStart(responseHeaders.get("Content-Range")) : 0;
                if (rangePrefix > 0 && isSingleByteRange(request.getRequestHeaders().get("Range")))
                    body = new RangePrefixInputStream(body, rangePrefix);
                Log.i("wuDict2", "Media bridge " + path + " Range=" + request.getRequestHeaders().get("Range") +
                        " -> " + status + " Content-Length=" + contentLength +
                        " Content-Range=" + responseHeaders.get("Content-Range") +
                        " WebViewRangePrefix=" + rangePrefix);
            }
            boolean textResource = mime.startsWith("text/") || mime.contains("javascript") ||
                    mime.contains("json") || mime.contains("xml");
            return new WebResourceResponse(mime, streamResource && !textResource ? null : encoding,
                    status, reason, responseHeaders, body);
        } catch (Exception error) {
            Log.w("wuDict2", "Could not load app resource " + path, error);
            return null;
        }
    }

    private static String resourceHeaders(WebResourceRequest request) {
        JSONObject selected = new JSONObject();
        Map<String, String> headers = request.getRequestHeaders();
        if (headers != null) {
            String[] allowed = {"Range", "If-Range", "If-Modified-Since", "If-None-Match",
                    "If-Unmodified-Since", "If-Match", "Accept-Language"};
            for (String canonical : allowed) {
                for (Map.Entry<String, String> entry : headers.entrySet()) {
                    if (canonical.equalsIgnoreCase(entry.getKey())) {
                        try { selected.put(canonical, entry.getValue()); }
                        catch (org.json.JSONException ignored) {}
                        break;
                    }
                }
            }
        }
        return selected.toString();
    }

    private static boolean isSingleByteRange(String range) {
        return range != null && range.matches("(?i)^bytes=\\d+-\\d*$");
    }

    private static long contentRangeStart(String contentRange) {
        if (contentRange == null) return 0;
        java.util.regex.Matcher match = java.util.regex.Pattern
                .compile("(?i)^bytes\\s+(\\d+)-\\d+/(?:\\d+|\\*)$")
                .matcher(contentRange.trim());
        if (!match.matches()) return 0;
        try { return Long.parseLong(match.group(1)); }
        catch (NumberFormatException ignored) { return 0; }
    }

    // WebView can apply the Range offset a second time to an intercepted 206
    // InputStream. Pretend the skipped prefix exists while reads stay at the
    // range body supplied by Go. See Chromium issue 571186732.
    private static final class RangePrefixInputStream extends InputStream {
        private final InputStream body;
        private long prefixRemaining;

        RangePrefixInputStream(InputStream body, long prefixLength) {
            this.body = body;
            this.prefixRemaining = prefixLength;
        }

        @Override public int read() throws IOException { return body.read(); }
        @Override public int read(byte[] dst, int offset, int length) throws IOException {
            return body.read(dst, offset, length);
        }
        @Override public long skip(long count) throws IOException {
            if (count <= 0) return 0;
            if (prefixRemaining > 0) {
                long skipped = Math.min(count, Math.min(prefixRemaining, Integer.MAX_VALUE));
                prefixRemaining -= skipped;
                return skipped;
            }
            return body.skip(count);
        }
        @Override public int available() throws IOException {
            long total = prefixRemaining + body.available();
            return total > Integer.MAX_VALUE ? 0 : (int) total;
        }
        @Override public void close() throws IOException { body.close(); }
    }
}
