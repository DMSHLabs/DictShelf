// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// The shell's own settings (D100) - and what is LEFT of them on this side.
//
// Until 2026-09-28 this screen carried the whole set: the three lookup rows,
// the information messages, the access key and the Advanced overrides (D101).
// They live in the app page now - the Settings drawer's System window, which
// draws them over the wudict:system bridge and writes the same ShellPrefs -
// and the reader asked for the copies here to go, so that there is one place
// per setting instead of two views to keep in step.
//
// Three things are left, and each for its own reason:
//
//   * The listen PORT, which is a row in the page's System window too. It is
//     here because of the one case the page cannot cover: a port that will not
//     bind means the page never loads, and the page is where the row would
//     otherwise live - so the app would have no way back in, and Restore
//     defaults (all-or-nothing, and it clears every other tuned row with it)
//     would be the only instrument. Both copies write the same key through the
//     same validator, and this screen is also the one that can apply the change
//     on the spot (Restart server now below).
//   * Restore defaults, DUPLICATED on the reader's word. It is the last line of
//     defence: a config row the app cannot start with - a port held by another
//     app, say - is undone from here, with no page and no server in the way.
//   * The restart footer - the stale notice and "Restart server now" - which
//     CANNOT work from the page: stopping the server is safe only while no
//     window holds it, and the window that asks from inside the page IS one
//     (MainActivity retains the server for as long as it exists). So the page
//     always says "it will pick them up the next time the app is opened", and
//     the one place the button can ever appear is here - the screen a reader
//     reaches with no app window behind it.
//   * The sentence saying where the rest of the rows went. This screen is on
//     the launcher's long-press menu, so it is where a reader who remembers the
//     old screen arrives; an almost empty screen with no signpost would read as
//     something lost.
//
// What is left still passes the charter: the restart is the PROCESS, the
// restore is a shell preference, and the sentence is a signpost - never a
// dictionary fact, which belongs to the page and the server (D54).
//
// Nothing here writes wudict.toml. An override is stored in SharedPreferences
// and delivered on the child's exec line, which is a HIGHER config layer than
// the file (flag > env > file > default), so the rule that Java never touches
// the user's config file is kept by construction rather than by discipline.
// Empty means emit nothing, so a row can only ever add a value, never
// countermand one the user wrote themselves.
//
// Views are built in code, like every other view in this app - there is no
// androidx and no res/layout, by design. The theme is the popup's own
// (Theme.WuWeiDict.Lookup): a floating window is exempt from the forced
// edge-to-edge that MainActivity.applyWindowInsets exists for, and it is
// dialog-shaped anyway.
//
// PROPORTION. The project's ratio is φ (D58's motion ladder, the stylesheet's
// --sp-* scale), and D70's lesson is that φ coordinates land between pixels:
// on an integer grid you use the Fibonacci integers, whose successive ratios
// are within half a percent of φ and which are whole numbers by definition. dp
// is an integer grid, so spacing here is 3·5·8·13·21·34·55. Type steps by √φ
// (1.272) rather than φ, because a full φ step from 13 sp lands on 21 and
// leaves nothing usable in between. Where beauty and ergonomics disagree,
// ergonomics wins: every interactive row is at least 55 dp tall, which is the
// ladder's own rung above Android's 48 dp touch minimum - the two agree here,
// which is why 55 and not 48.
package com.legbehindneck.wudict;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.text.InputType;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.Map;

public class SettingsActivity extends Activity {
    @Override public android.content.res.Resources getResources() {
        return UiLanguage.resources(getBaseContext(), super.getResources());
    }


    // The Fibonacci dp ladder, and the two rungs that are ergonomic floors.
    private static final int SP_2 = 5, SP_3 = 8, SP_4 = 13, SP_5 = 21, SP_6 = 34;
    private static final int ROW_MIN = 55;   // ≥ the 48 dp touch minimum
    private static final int FIELD_W = 89;   // four digits, right-aligned column

    // 13 · 13√φ
    private static final float TEXT_HINT = 13f, TEXT_LABEL = 16.5f;
    private static final float LINE_PHI = 1.618f;

    private TextView staleText;
    private final java.util.List<Runnable> backgroundButtonUpdates = new java.util.ArrayList<>();
    private Button applyNow;
    private volatile boolean gone;

    // The port row - the one override this screen still edits (see the class
    // comment), and the only field left to keep in step.
    private EditText portField;
    private TextView portHint;

    // What the running server answered, kept so that a later re-test can be
    // made against it without asking again: the server's own values cannot
    // change while it runs - only what a spawn would pass can, and that is what
    // isStale compares.
    private final Map<String, String> serverValues = new LinkedHashMap<>();
    private final Map<String, String> serverOrigins = new HashMap<>();
    private boolean serverAnswered;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setTitle(R.string.settings_title);
        applySepiaWindow();

        LinearLayout col = new LinearLayout(this);
        col.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(SP_5);
        col.setPadding(pad, pad, pad, pad);

        // The signpost, first, because it answers the question the reader
        // arrived with: this screen used to hold the lookup rows, the messages,
        // the access key and the advanced rows, and they are in the app now.
        // A row on the launcher's long-press menu that opens an almost empty
        // window with no explanation reads as something lost rather than as
        // something moved.
        col.addView(caption(getString(R.string.settings_moved_hint), 0, SP_4));

        // The port, first among the controls because it is the one that can
        // keep the app from starting at all (see the class comment).
        col.addView(portRow());

        // Restore defaults, duplicated on the reader's word: it is the one bulk
        // action, and it is worth having when the app page is not reachable
        // (see the class comment). Both copies confirm and both clear the same
        // overrides.
        col.addView(restoreButton());

        // What the running server was started with, and the one way to change
        // it without reopening the app. The notice is built from /api/config's
        // own answer, so it can say the true thing rather than a remembered one.
        staleText = caption("", SP_6, SP_2);
        staleText.setVisibility(View.GONE);
        col.addView(staleText);

        applyNow = new Button(this);
        styleBackgroundButton(applyNow);
        applyNow.setText(R.string.settings_apply_now);
        applyNow.setVisibility(View.GONE);
        applyNow.setOnClickListener(v -> applyNow());
        col.addView(applyNow, wide(SP_2));

        // The way out. Kept although the screen is short, because this window
        // wears no ✕ in a corner: tapping outside and Back both close it, and
        // neither is visible on the screen the reader is looking at.
        Button close = new Button(this);
        styleBackgroundButton(close);
        close.setText(R.string.settings_close);
        close.setOnClickListener(v -> finish());
        col.addView(close, wide(SP_6));

        ScrollView scroll = new ScrollView(this);
        scroll.addView(col, new ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT));
        setContentView(scroll);

        // 92 % of the screen - the same proportion the web UI's .container
        // uses. Height stays WRAP_CONTENT: the platform caps a dialog at the
        // space available and the ScrollView takes the overflow, so a small
        // screen scrolls rather than clipping.
        getWindow().setLayout((int) (getResources().getDisplayMetrics().widthPixels * 0.92f),
                WindowManager.LayoutParams.WRAP_CONTENT);

        askServer();
    }

    @Override
    protected void onDestroy() {
        gone = true;
        super.onDestroy();
    }

    // ── the window ───────────────────────────────────────────────────────

    private void applySepiaWindow() {
        getWindow().setBackgroundDrawable(WindowBackground.dialogDrawable(this, ShellPrefs.sepia(this)
                ? ShellPrefs.sepiaColor(this) : getColor(R.color.window_bg)));
        for (Runnable update : backgroundButtonUpdates) update.run();
    }

    private void styleBackgroundButton(Button button) {
        android.graphics.drawable.Drawable original = button.getBackground();
        android.content.res.ColorStateList originalText = button.getTextColors();
        android.animation.StateListAnimator originalAnimator = button.getStateListAnimator();
        float originalElevation = button.getElevation();
        Runnable update = () -> {
            boolean custom = ShellPrefs.sepia(this) || WindowBackground.active(this);
            if (!custom) {
                button.setBackground(original);
                button.setTextColor(originalText);
                button.setStateListAnimator(originalAnimator);
                button.setElevation(originalElevation);
                return;
            }
            int background = ShellPrefs.sepia(this)
                    ? ShellPrefs.sepiaColor(this) : getColor(R.color.window_bg);
            int rgb = ShellPrefs.darkIcons(background) ? 0x000000 : 0xFFFFFF;
            android.graphics.drawable.GradientDrawable outline = new android.graphics.drawable.GradientDrawable();
            outline.setColor(Color.TRANSPARENT);
            outline.setCornerRadius(dp(SP_3));
            outline.setStroke(dp(1), 0x55000000 | rgb);
            button.setBackground(new android.graphics.drawable.RippleDrawable(
                    android.content.res.ColorStateList.valueOf(0x22000000 | rgb), outline, null));
            button.setTextColor(0xDE000000 | rgb);
            button.setStateListAnimator(null);
            button.setElevation(0);
            button.setTranslationZ(0);
        };
        backgroundButtonUpdates.add(update);
        update.run();
    }

    /**
     * The listen port: the row the page has too, and the only one that is here
     * as well (the class comment argues why). Everything about it is what the
     * removed number rows did — the server's own answer as the placeholder, the
     * range and the value in effect appended to the hint, commit on focus loss,
     * on Enter and on leaving the screen, and a refusal that puts the field
     * back and says why.
     */
    private View portRow() {
        ShellPrefs.Override o = ShellPrefs.byKey("SERVER_PORT");
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setPadding(0, dp(SP_2), 0, dp(SP_2));

        LinearLayout line = new LinearLayout(this);
        line.setOrientation(LinearLayout.HORIZONTAL);
        line.setGravity(Gravity.CENTER_VERTICAL);
        line.setMinimumHeight(dp(ROW_MIN));

        TextView label = new TextView(this);
        label.setText(o.label);
        label.setTextSize(TypedValue.COMPLEX_UNIT_SP, TEXT_LABEL);
        line.addView(label, new LinearLayout.LayoutParams(0,
                ViewGroup.LayoutParams.WRAP_CONTENT, 1f));

        EditText f = new EditText(this);
        f.setInputType(InputType.TYPE_CLASS_NUMBER);
        f.setTextSize(TypedValue.COMPLEX_UNIT_SP, TEXT_LABEL);
        f.setGravity(Gravity.END);
        f.setSingleLine(true);
        f.setMinHeight(dp(ROW_MIN));
        f.setText(portValue());
        // Committing on focus loss rather than on every keystroke: "1" on the
        // way to "1024" is not an invalid value, it is an unfinished one.
        f.setOnFocusChangeListener((v, focused) -> {
            if (!focused) commitPort();
        });
        f.setOnEditorActionListener((v, id, ev) -> {
            commitPort();
            return false;
        });
        portField = f;
        line.addView(f, new LinearLayout.LayoutParams(dp(FIELD_W),
                ViewGroup.LayoutParams.WRAP_CONTENT));
        box.addView(line);

        portHint = caption("", 0, SP_4);
        box.addView(portHint);
        paintPortHint();
        return box;
    }

    /**
     * What the field shows: the reader's own port, or - when they have not
     * chosen one - the port a spawn would use ({@link ShellPrefs#emitted},
     * which for this key is never nothing: the shell always passes a port).
     *
     * <p>A blank box was the reader's report (2026-09-28): an empty field beside
     * a hint that names a port reads as a missing value, not as "the file
     * decides", and the port is the one row here whose value is worth seeing at
     * a glance. The seeded number is NOT a choice, and {@link #commitPort}
     * knows the difference - leaving it alone stores nothing.
     */
    private String portValue() {
        ShellPrefs.Override o = ShellPrefs.byKey("SERVER_PORT");
        String stored = ShellPrefs.override(this, o);
        return stored != null ? stored : String.valueOf(ShellPrefs.emitted(this, o));
    }

    /**
     * The port row's hint: what it costs, the range it accepts, and what the
     * RUNNING server was started with. That last part is the one thing the
     * field cannot say - the field holds what a spawn would pass, and after a
     * change the two differ until the next start.
     */
    private void paintPortHint() {
        if (portHint == null || portField == null) return;
        ShellPrefs.Override o = ShellPrefs.byKey("SERVER_PORT");
        int max = ShellPrefs.maxOf(this, o);
        String seen = serverAnswered ? serverValues.get(o.key) : null;
        portHint.setText(getString(o.hint)
                + " " + getString(R.string.settings_range, o.min, max)
                + (seen == null ? "" : " " + getString(R.string.settings_in_effect, seen)));
    }

    /**
     * Stores the port, or puts the field back with a sentence about why not.
     * The validator is {@link ShellPrefs#setOverrideChecked} - the very call the
     * page's System window writes through - so a value this screen accepts can
     * never be one the child cannot use, and the two copies cannot drift.
     */
    private void commitPort() {
        if (portField == null || gone) return;
        ShellPrefs.Override o = ShellPrefs.byKey("SERVER_PORT");
        String stored = ShellPrefs.override(this, o);
        String typed = portField.getText().toString().trim();
        if (typed.isEmpty()) {
            // An empty field is "follow the settings file" - and the number
            // that resolves to is put straight back, so the reader is never
            // left looking at a box that is blank and still means a port.
            if (stored != null) {
                ShellPrefs.setOverrideChecked(this, o, null);
                recheck();
            }
            portField.setText(portValue());
            paintPortHint();
            return;
        }
        if (stored == null && typed.equals(String.valueOf(ShellPrefs.emitted(this, o)))) {
            // The number on screen is the one already in force, not a choice:
            // a seeded field must not become an override by being left alone -
            // which is what a focus loss or a Back would otherwise do.
            portField.setText(portValue());
            return;
        }
        try {
            ShellPrefs.setOverrideChecked(this, o, typed);
        } catch (IllegalArgumentException refused) {
            toast(getString(R.string.settings_out_of_range, typed, o.min,
                    ShellPrefs.maxOf(this, o)));
            portField.setText(portValue());
            return;
        }
        portField.setText(portValue()); // canonical: "0128" is stored "128"
        // The footer is re-tested here because a port is the one row whose new
        // value can make the running server wrong to keep: it will be the next
        // SPAWN's port, and the child that is up was exec'd with another.
        recheck();
        paintPortHint();
    }

    private View restoreButton() {
        Button b = new Button(this);
        styleBackgroundButton(b);
        b.setText(R.string.settings_restore);
        // Confirmed, because one tap clears several tuned fields at once.
        // Clearing restores inheritance - it writes nothing anywhere.
        b.setOnClickListener(v -> new BackgroundDialogBuilder(this)
                .setMessage(R.string.settings_restore_confirm)
                .setNegativeButton(R.string.settings_cancel, null)
                .setPositiveButton(R.string.settings_restore, (d, w) -> {
                    ShellPrefs.clearOverrides(this);
                    // The port row shows an override again, so it is repainted
                    // with the rest - and what it shows now is the port a spawn
                    // would use, because there is no longer one of the reader's.
                    if (portField != null) portField.setText(portValue());
                    paintPortHint();
                    toast(getString(R.string.settings_restore_done));
                    // The running server may now hold values nobody would pass,
                    // so the footer is re-tested against the answer in hand.
                    recheck();
                })
                .show());
        return wrap(b, SP_6);
    }

    @Override
    protected void onPause() {
        super.onPause();
        // Leaving with a half-typed port must not lose it, and must not store a
        // value the server would refuse either: commitPort stores a valid
        // number or puts the field back, and both are what the reader meant.
        commitPort();
    }

    /**
     * Reads what config.FormatSize writes: a number with an optional B/KB/MB/GB
     * suffix. Returns -1 for anything else, which is treated as "unknown"
     * rather than guessed at.
     */
    private static long parseSize(String s) {
        String t = s.trim().toUpperCase();
        int shift = 0;
        if (t.endsWith("GB")) {
            shift = 30;
            t = t.substring(0, t.length() - 2);
        } else if (t.endsWith("MB")) {
            shift = 20;
            t = t.substring(0, t.length() - 2);
        } else if (t.endsWith("KB")) {
            shift = 10;
            t = t.substring(0, t.length() - 2);
        } else if (t.endsWith("B")) {
            t = t.substring(0, t.length() - 1);
        }
        try {
            return Long.parseLong(t.trim()) << shift;
        } catch (NumberFormatException e) {
            return -1;
        }
    }

    // ── what the running server is actually running with ─────────────────
    //
    // Several of these defaults are computed by Go from the device itself
    // (internal/config/tuning.go), so Java must not recompute them - it asks.
    // /api/config also answers the only question this screen still has: whether
    // a server that is already running was started with different values. The
    // page's System window asks the same endpoint for the same reason; neither
    // side keeps a copy of what it said, because the subject of the sentence is
    // what the server is RUNNING, and a remembered answer cannot say that.

    private void askServer() {
        // ServerProcess.port is "the server we can reach": the live child's
        // port while one is up, the configured one when none is. A probe of the
        // configured port instead would find nothing whenever a port row has
        // changed under a running app - and that is one of the states this
        // footer exists to report (found on the emulator, 2026-09-28).
        final int port = ServerProcess.port(this);
        Thread t = new Thread(() -> {
            Map<String, String> values = new LinkedHashMap<>();
            Map<String, String> origins = new HashMap<>();
            boolean answered = ServerProcess.fetchEffective(port, values, origins);
            if (gone) return;
            runOnUiThread(() -> {
                if (gone) return;
                serverAnswered = answered;
                serverValues.clear();
                serverOrigins.clear();
                serverValues.putAll(values);
                serverOrigins.putAll(origins);
                // The port row's placeholder and "in effect" line arrive with
                // this answer, so it is repainted before the footer is judged.
                paintPortHint();
                recheck();
            });
        }, "wudict-settings");
        t.setDaemon(true);
        t.start();
    }

    /**
     * Re-tests the running server against what a spawn would pass now: on the
     * answer arriving, and after Restore defaults, which is the one change this
     * screen can still make. Cheap: no request, only the maps the one request
     * already brought back.
     */
    private void recheck() {
        showStale(serverAnswered && isStale(serverValues, serverOrigins));
    }

    /**
     * Whether the running server would be started differently now. Two ways
     * that happens: a value we WOULD pass differs from the one it has, or we
     * would pass nothing for a key it received from a previous exec line -
     * origin "env" or "flag" is the shell's own fingerprint, since nothing
     * else on this device sets the child's environment.
     */
    private boolean isStale(Map<String, String> values, Map<String, String> origins) {
        for (ShellPrefs.Override o : ShellPrefs.OVERRIDES) {
            String has = values.get(o.key);
            if (has == null) continue;
            String want = ShellPrefs.emitted(this, o);
            if (want == null) {
                String from = origins.get(o.key);
                if ("env".equals(from) || "flag".equals(from)) return true;
                continue;
            }
            if (!same(o, want, has)) return true;
        }
        return false;
    }

    /** Value equality in the key's own terms: sizes by bytes, the rest literally. */
    private static boolean same(ShellPrefs.Override o, String a, String b) {
        if (o.kind == ShellPrefs.MEGABYTES) {
            long x = parseSize(a), y = parseSize(b);
            return x >= 0 && x == y;
        }
        return a.equals(b);
    }

    private void showStale(boolean stale) {
        if (!stale) {
            staleText.setVisibility(View.GONE);
            applyNow.setVisibility(View.GONE);
            return;
        }
        // The button appears only when stopping the server cannot take
        // anything away from anyone: no window is holding it and no ingest is
        // in flight. Otherwise the same notice appears without it, saying the
        // true thing rather than promising a restart that will not happen.
        boolean safe = ServerProcess.holders() == 0 && !IndexService.isBusy();
        staleText.setText(getString(R.string.settings_stale)
                + (safe ? "" : " " + getString(R.string.settings_stale_later)));
        staleText.setVisibility(View.VISIBLE);
        applyNow.setVisibility(safe ? View.VISIBLE : View.GONE);
    }

    private void applyNow() {
        applyNow.setEnabled(false);
        Thread t = new Thread(() -> {
            ServerProcess.stopAny(this);
            if (gone) return;
            runOnUiThread(() -> {
                if (gone) return;
                applyNow.setEnabled(true);
                showStale(false);
                toast(getString(R.string.settings_applied));
            });
        }, "wudict-apply");
        t.setDaemon(true);
        t.start();
    }

    // ── the ladder ───────────────────────────────────────────────────────

    private TextView caption(String text, int topDp, int bottomDp) {
        TextView t = new TextView(this);
        t.setText(text);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, TEXT_HINT);
        t.setLineSpacing(0f, LINE_PHI);
        t.setAlpha(0.7f);
        t.setPadding(0, dp(topDp), 0, dp(bottomDp));
        return t;
    }

    private View wrap(View v, int topDp) {
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setPadding(0, dp(topDp), 0, 0);
        box.addView(v, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return box;
    }

    private LinearLayout.LayoutParams wide(int topDp) {
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        p.topMargin = dp(topDp);
        return p;
    }

    private void toast(String message) {
        Toast.makeText(this, message, Toast.LENGTH_LONG).show();
    }

    private int dp(float v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v,
                getResources().getDisplayMetrics());
    }
}
