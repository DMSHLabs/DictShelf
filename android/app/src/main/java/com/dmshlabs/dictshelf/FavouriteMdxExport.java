// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.dmshlabs.dictshelf;

import android.app.Activity;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.os.Environment;
import android.provider.DocumentsContract;
import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.util.HashSet;
import java.util.Set;

/** Copies an MDX/MDD pair from app-owned temporary storage into a SAF folder. */
final class FavouriteMdxExport {
    static final int REQUEST = 0x574e;
    private static final Object LOCK = new Object();
    private static android.webkit.JsPromptResult pending;
    private static volatile Status status = new Status();

    private static final class Status {
        volatile boolean running, canceled;
        volatile String error = "";
    }

    static void pick(Activity activity, android.webkit.JsPromptResult result) {
        synchronized (LOCK) {
            if (pending != null) pending.cancel();
            pending = result;
        }
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION
                        | Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                        | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
        try {
            activity.startActivityForResult(intent, REQUEST);
        } catch (RuntimeException e) {
            settle(null);
        }
    }

    static void onActivityResult(Activity activity, int code, int resultCode, Intent data) {
        if (code != REQUEST) return;
        Uri tree = resultCode == Activity.RESULT_OK && data != null ? data.getData() : null;
        if (tree == null) { settle(null); return; }
        try {
            int flags = data.getFlags() & (Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
            activity.getContentResolver().takePersistableUriPermission(tree, flags);
            settle(tree.toString());
        } catch (Exception e) {
            settle(null);
        }
    }

    private static void settle(String value) {
        android.webkit.JsPromptResult callback;
        synchronized (LOCK) { callback = pending; pending = null; }
        if (callback != null) { if (value == null) callback.cancel(); else callback.confirm(value); }
    }

    static String tempDirectory(Activity activity) throws IOException {
        File base = activity.getExternalFilesDir(Environment.DIRECTORY_DOCUMENTS);
        if (base == null) base = activity.getFilesDir();
        File dir = new File(base, "favourites-export");
        if (!dir.isDirectory() && !dir.mkdirs()) throw new IOException("Cannot prepare export folder");
        File[] stale = dir.listFiles((parent, name) -> name.endsWith(".mdx") || name.endsWith(".mdd"));
        if (stale != null) for (File file : stale) if (!file.delete()) throw new IOException("Cannot clear previous export files");
        return dir.getAbsolutePath();
    }

    static String startCopy(Activity activity, String treeText, String filesText) {
        final Uri tree;
        final JSONArray files;
        try { tree = Uri.parse(treeText); files = new JSONArray(filesText); }
        catch (Exception e) { return "invalid export destination"; }
        if (!"content".equals(tree.getScheme()) || files.length() < 1 || files.length() > 2) return "invalid export destination";
        synchronized (LOCK) {
            if (status.running) return "an export is already running";
            status = new Status(); status.running = true;
        }
        final JSONArray names = files;
        new Thread(() -> copy(activity, tree, names), "wudict-favourites-export").start();
        return "";
    }

    static String copyStatus(String action) {
        Status current = status;
        if ("stop".equals(action) && current.running) current.canceled = true;
        JSONObject result = new JSONObject();
        try {
            result.put("running", current.running);
            result.put("canceled", current.canceled);
            result.put("error", current.error);
        } catch (Exception ignored) { }
        return result.toString();
    }

    private static void copy(Activity activity, Uri tree, JSONArray files) {
        Status current = status;
        java.util.ArrayList<Uri> created = new java.util.ArrayList<>();
        java.util.ArrayList<File> sources = new java.util.ArrayList<>();
        try {
            for(int i=0;i<files.length();i++) sources.add(new File(files.getString(i)));
            String treeId = DocumentsContract.getTreeDocumentId(tree);
            Uri parent = DocumentsContract.buildDocumentUriUsingTree(tree, treeId);
            Set<String> existing = childNames(activity, tree, treeId);
            String base = new File(files.getString(0)).getName().replaceFirst("(?i)\\.mdx$", "");
            for (int n = 1; n <= 10000; n++) {
                String candidate = n == 1 ? base : base + " (" + n + ")";
                boolean free = true;
                for (int i = 0; i < files.length(); i++) {
                    String ext = files.getString(i).substring(files.getString(i).lastIndexOf('.'));
                    if (existing.contains(candidate + ext)) { free = false; break; }
                }
                if (!free) continue;
                for (int i = 0; i < files.length(); i++) {
                    if (current.canceled) throw new IOException("Export canceled");
                    String source = files.getString(i);
                    File sourceFile = new File(source);
                    String ext = source.substring(source.lastIndexOf('.'));
                    Uri target = DocumentsContract.createDocument(activity.getContentResolver(), parent, "application/octet-stream", candidate + ext);
                    if (target == null) throw new IOException("Could not create " + candidate + ext);
                    created.add(target);
                    try (FileInputStream in = new FileInputStream(sourceFile);
                         OutputStream out = activity.getContentResolver().openOutputStream(target, "w")) {
                        if (out == null) throw new IOException("Cannot open export destination");
                        byte[] buffer = new byte[1024 * 1024];
                        int count;
                        while ((count = in.read(buffer)) >= 0) {
                            if (current.canceled) throw new IOException("Export canceled");
                            out.write(buffer, 0, count);
                        }
                    }
                }
                return;
            }
            throw new IOException("No available file name");
        } catch (Exception e) {
            for (Uri uri : created) try { DocumentsContract.deleteDocument(activity.getContentResolver(), uri); } catch (Exception ignored) { }
            current.error = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        } finally {
            for (File file : sources) try { file.delete(); } catch (Exception ignored) { }
            current.running = false;
        }
    }

    private static Set<String> childNames(Activity activity, Uri tree, String treeId) throws IOException {
        Set<String> names = new HashSet<>();
        Uri children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, treeId);
        try (Cursor cursor = activity.getContentResolver().query(children,
                new String[]{DocumentsContract.Document.COLUMN_DISPLAY_NAME}, null, null, null)) {
            if (cursor == null) throw new IOException("Cannot read selected folder");
            while (cursor.moveToNext()) names.add(cursor.getString(0));
        }
        return names;
    }

    private FavouriteMdxExport() { }
}
