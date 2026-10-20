// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Launcher shortcut for app-wide system settings and diagnostics. The app's
// settings page is the only place that edits runtime tuning; this screen keeps
// the recovery actions available before the Go runtime has started.
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
import android.util.TypedValue;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;


public class SettingsActivity extends Activity {
    @Override public android.content.res.Resources getResources() {
        return UiLanguage.resources(getBaseContext(), super.getResources());
    }


    // The Fibonacci dp ladder, and the two rungs that are ergonomic floors.
    private static final int SP_3 = 8, SP_4 = 13, SP_5 = 21, SP_6 = 34;
    private static final int ROW_MIN = 55;   // ≥ the 48 dp touch minimum

    // 13 · 13√φ
    private static final float TEXT_HINT = 13f;
    private static final float LINE_PHI = 1.618f;

    private final java.util.List<Runnable> backgroundButtonUpdates = new java.util.ArrayList<>();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setTitle(R.string.settings_title);
        applySepiaWindow();

        LinearLayout col = new LinearLayout(this);
        col.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(SP_5);
        col.setPadding(pad, pad, pad, pad);

        // This shortcut remains in the launcher's long-press menu.
        col.addView(caption(getString(R.string.settings_moved_hint), 0, SP_4));

        Button saveLog = new Button(this);
        styleBackgroundButton(saveLog);
        saveLog.setText(R.string.system_log_save);
        saveLog.setOnClickListener(v -> SystemLogExport.start(this));
        col.addView(saveLog, wide(SP_6));

        // Restore defaults, duplicated on the reader's word: it is the one bulk
        // action, and it is worth having when the app page is not reachable
        // (see the class comment). Both copies confirm and both clear the same
        // overrides.
        col.addView(restoreButton(), wide(SP_4));


        // The way out. Kept although the screen is short, because this window
        // wears no ✕ in a corner: tapping outside and Back both close it, and
        // neither is visible on the screen the reader is looking at.
        Button close = new Button(this);
        styleBackgroundButton(close);
        close.setText(R.string.settings_close);
        close.setOnClickListener(v -> finish());
        col.addView(close, wide(SP_4));

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

    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, android.content.Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        SystemLogExport.result(this, requestCode, resultCode, data);
    }

    // ── the window ───────────────────────────────────────────────────────

    private void applySepiaWindow() {
        getWindow().setBackgroundDrawable(WindowBackground.dialogDrawable(this, ShellPrefs.sepia(this)
                ? ShellPrefs.sepiaColor(this) : getColor(R.color.window_bg)));
        for (Runnable update : backgroundButtonUpdates) update.run();
    }

    private void styleBackgroundButton(Button button) {
        button.setAllCaps(false);
        button.setMinHeight(dp(ROW_MIN));
        button.setMinimumHeight(dp(ROW_MIN));
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
                    GoBridge.restoreIndexWorkersIfRunning();
                    toast(getString(R.string.settings_restore_done));
                })
                .show());
        return b;
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
