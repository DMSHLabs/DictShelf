// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// SHELL FACTS: the fourth category of state in this project, and the only one
// that belongs to Java (D100).
//
    // The other three are spoken for. Collection facts - which dictionaries are
    // searched, in what order - live in state.json. Install facts - folders and
    // memory limits - live in wudict.toml, which this shell seeds
// once and never reads back (GoRuntime.seedConfig). Browser-view facts -
// theme, wide mode - live in the page's localStorage.
//
// A shell fact is none of those: it is about the Android shell's own
// behaviour, and two properties force it here rather than into any of them.
//
    // It must be READABLE BY THE DECIDER AT DECISION TIME. LookupActivity chooses
    // a window before the Go runtime is ready, so there may be no WebView:
    // localStorage is unreachable by construction (it is
// keyed by origin and needs a live page), and a running server cannot be
// assumed.
//
// And it must be OWNED BY WHOEVER THE VALUE IS ABOUT. state.json is readable
// from Java - it is JSON, and Prefs.Replace writes it temp-file + rename, so a
// read cannot tear - but it is the SERVER's file: heal() rewrites it, Replace()
// is its only writer, and its `version` exists so a format change can be
// recognised rather than guessed at. Putting a value there that Go would never
// read turns every future change to that schema into a cross-language
// compatibility event, invisible from the Go side. A store whose owner never
// reads the value is the wrong store.
//
// SharedPreferences is the only store that passes both: same process as the
// decider, synchronous, no parse, no schema, no version negotiation, and a
// missing key is simply the default.
//
// ── PLATFORM OVERRIDES (D101) ────────────────────────────────────────────────
//
// The second half of this file stores a different kind of value: a Go config
// key whose right answer differs BECAUSE OF THIS DEVICE. It is still a shell
// fact - the device is the shell's subject - and it is still stored here, but
// it is DELIVERED into the config layering rather than read by Java.
//
// config.Load resolves every key as flag > env > file > default. The shell
// already writes the top two layers on every spawn, so an override rides a
// layer that OUTRANKS wudict.toml and never reads it: the rule that Java must
// not parse or rewrite the user's config file stands untouched, and nothing
// new had to be invented to keep it.
//
// Two states, never three. Set means "emit this value"; unset means "emit
// nothing", so the file and the built-in default decide. The shell never emits
// a NEGATING value, so a control on the settings screen can never silently
// countermand something the user wrote in wudict.toml themselves.
package com.dmshlabs.dictshelf;

import android.app.ActivityManager;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.res.Configuration;
import android.graphics.Color;

import java.io.BufferedReader;
import java.io.FileReader;
import java.util.LinkedHashMap;
import java.util.Map;

final class ShellPrefs {

    private static final String FILE = "shell";

    static final String SEPIA = "sepia";
    // Legacy dictionary-picker preferences are no longer read; the page's
    // shared picker owns group choice, result navigation and list filtering.
    private static final String SEPIA_COLOR = "sepia_color";
    private static final int DEFAULT_SEPIA_COLOR = 0xFFF4ECD8;
    private static final int DEFAULT_SEPIA_COLOR_NIGHT = 0xFF1C1A17;
    static final String BACKGROUND_IMAGE = "background_image";

    // ── the paper an enabled preset paints (2026-09-28) ─────────────────────
    //
    // The app page's own background, when a preset layer repaints it: the
    // Warm look is a layer whose --bg is #f4ecd8 and nothing else, so the page
    // is warm while every OTHER window of the app stayed the app's off-white.
    // Those windows cannot read a CSS file - three of them (Edit Folders,
    // Lemmatization, Browse) are documents the host tells a colour, and the
    // dictionary list, the search-mode list, the popup and the inset strip are
    // drawn by this side outright - so the page REPORTS the paper it is
    // painting (index.html: presetPaperPush, over the appearance bridge) and
    // this file remembers it per theme, exactly like the reader's own colour.
    //
    // Precedence, and it is the page's own: the reader's window colour wins
    // over the preset's paper (the colour pins --bg), the preset's paper wins
    // over the app's built-in background, and nothing at all leaves the
    // built-in to decide. Both are per theme, so a day-only preset says
    // nothing at night by construction.
    //
    // NOT the reader's colour: this key is never drawn in the Appearance
    // sheet, because the reader never set it - the layer they switched on did,
    // and switching that off withdraws it.
    private static final String PAPER = "preset_paper";

    // The background belongs to a THEME, and the theme is the page's to
    // resolve (wudict_theme, plus "auto" following the system). So each key
    // below has a night twin, and every reader here picks by what the page last
    // reported - which is what lets the window be painted correctly BEFORE the
    // page exists at all, rather than corrected a frame later.
    //
    // The unsuffixed keys ARE the day set: an install that predates this keeps
    // its background exactly where it left it, and starts with none at night -
    // which is the wanted default, not a migration gap.
    private static final String NIGHT_SUFFIX = "_night";

    private static String themed(String base, boolean night) {
        return night ? base + NIGHT_SUFFIX : base;
    }

    /** The theme the window is showing, as the page last reported it. */
    static boolean night(Context c) { return pageDark(c); }

    static boolean sepia(Context c) { return sepiaFor(c, night(c)); }

    static boolean sepiaFor(Context c, boolean night) {
        // Warm is the new-profile default look, and its background colour is
        // part of the look on every platform.
        return of(c).getBoolean(themed(SEPIA, night), true);
    }

    static int sepiaColor(Context c) { return sepiaColorFor(c, night(c)); }

    static int sepiaColorFor(Context c, boolean night) {
        int fallback = night ? DEFAULT_SEPIA_COLOR_NIGHT : DEFAULT_SEPIA_COLOR;
        return of(c).getInt(themed(SEPIA_COLOR, night), fallback) | 0xFF000000;
    }

    static String sepiaColorText(Context c) { return sepiaColorTextFor(c, night(c)); }

    static String sepiaColorTextFor(Context c, boolean night) {
        return String.format(java.util.Locale.ROOT, "#%06x", sepiaColorFor(c, night) & 0xFFFFFF);
    }

    /** The chosen background image for the theme, or "" when there is none. */
    static String backgroundImage(Context c) { return backgroundImageFor(c, night(c)); }

    static String backgroundImageFor(Context c, boolean night) {
        return of(c).getString(themed(BACKGROUND_IMAGE, night), "");
    }

    /** The paper the enabled preset paints for the theme, or "" for none. */
    static String paperFor(Context c, boolean night) {
        return of(c).getString(themed(PAPER, night), "");
    }

    /**
     * Records the paper the page reported for one theme. An empty value
     * WITHDRAWS it - that is how switching the preset off is heard - and
     * anything that is not a six-digit colour is refused outright, because a
     * value this side cannot parse would be a window painted in whatever
     * {@code Color.parseColor} makes of it.
     *
     * <p>Answers whether the value CHANGED, which is what keeps the bridge
     * from ping-ponging: the page pushes this on every layer load, and a write
     * that repainted unconditionally would come back as another push.
     */
    static boolean setPaper(Context c, boolean night, String value) {
        String v = value == null ? "" : value.trim();
        if (!v.isEmpty() && !v.matches("#[0-9a-fA-F]{6}")) {
            throw new IllegalArgumentException(v);
        }
        v = v.toLowerCase(java.util.Locale.ROOT);
        SharedPreferences p = of(c);
        String key = themed(PAPER, night);
        if (v.equals(p.getString(key, ""))) return false;
        SharedPreferences.Editor e = p.edit();
        if (v.isEmpty()) e.remove(key);
        else e.putString(key, v);
        e.apply();
        return true;
    }

    static void setSepia(Context c, boolean night, boolean on) {
        of(c).edit().putBoolean(themed(SEPIA, night), on).apply();
    }

    static void setBackgroundImage(Context c, boolean night, String name) {
        of(c).edit().putString(themed(BACKGROUND_IMAGE, night), name).apply();
    }

    static void setSepiaColor(Context c, boolean night, String value) {
        value = value.trim();
        if (!value.matches("#?[0-9a-fA-F]{6}")) throw new IllegalArgumentException(value);
        if (!value.startsWith("#")) value = "#" + value;
        of(c).edit().putInt(themed(SEPIA_COLOR, night), Color.parseColor(value)).apply();
    }

    // One key per way in, because the four carry different intent: a
    // selection-toolbar tap is a glance, a shared passage is deliberate, a
    // wudict:// call is programmatic, and a reader's dictionary button is the
    // reader's habit, configured once in the reader. All default to false -
    // float - so an upgrade changes nothing for anyone.
    static final String TOOLBAR = "lookup_toolbar_in_app";
    static final String SHARE = "lookup_share_in_app";
    static final String LINK = "lookup_link_in_app";
    static final String READER = "lookup_reader_in_app";

    // ── the information messages (2026-09-27) ────────────────────────────────
    //
    // The transient text this app draws while it is starting and while it is
    // loading dictionaries: "Starting DictShelf…", the page's waiting art, its
    // "N of M ready" counter, the lookup window's "Looking up …". They are
    // honest and they are also unreadable - the first ones are on screen for a
    // few hundred milliseconds - so the row exists to turn them off. What it
    // does NOT reach is anything the reader has to act on: a failure, or the
    // page's note that the answers below are for a different form of the word
    // that was typed (the reader kept that one, deliberately, after living with
    // the switch for a day).
    //
    // A shell fact by the CHARTER's own test above, and this is the reason the
    // row is here rather than in the page's Appearance sheet, which is where
    // the Screen rows went: one of the messages is painted by MainActivity
    // BEFORE a page or a server exists, so the decider has nobody to ask, and
    // a preference stored behind the page could not be read in time to silence
    // that first frame. The page's half rides the URL beside shell_bg
    // (Shell.shellQuery) - the same way a look is delivered, and for the same
    // reason: it has to be true at first paint.
    //
    // ON by default. The row exists to turn the messages off, and an install
    // that never touches it must behave exactly as it did before it existed.
    static final String INFO_MESSAGES = "show_info_messages";

    // Whether the app's own window hides the system bars while it is read.
    // A shell fact by the same two tests as the four above: the WINDOW is the
    // subject, and the decider - MainActivity, painting its first frame - has
    // no page and no server to ask. The popup is exempt by construction: a
    // floating window does not own the bars, so this key is read in one place.
    //
    // SUPERSEDED by BARS, which says WHICH bars rather than merely whether.
    // Kept because migrate() reads it: an upgrade must not silently turn a
    // user's full-screen reading off.
    private static final String IMMERSIVE = "immersive";

    // ── the display edge (D141) ──────────────────────────────────────────────
    //
    // On a cutout device the strip behind the camera is painted by THIS APP and
    // by nothing else. At targetSdk 36 the platform has taken every other lever
    // away: setStatusBarColor and setNavigationBarColor are no-ops from API 35,
    // layoutInDisplayCutoutMode is forced to ALWAYS for non-floating windows, and
    // windowOptOutEdgeToEdgeEnforcement was disabled for API 36. What is left is
    // MainActivity.applyWindowInsets padding the root and that padding showing a
    // colour - so the entire design space is which colour, and whether to pad.
    //
    // Which is why this is a shell fact and not a page preference: the decider
    // paints the first frame before any page or server exists, and the thing
    // being decided is a window, not a document.
    static final String EDGE_MODE = "edge_mode";

    /** The strip follows the OS day/night setting - @color/window_bg. */
    static final int EDGE_SYSTEM = 0;
    /** The strip follows the PAGE's own theme, whatever it last reported. */
    static final int EDGE_PAGE = 1;
    /** The strip is black, whatever anything else says. */
    static final int EDGE_BLACK = 2;
    /** The strip is {@link #EDGE_COLOR}. */
    static final int EDGE_CUSTOM = 3;
    /** There is no strip: the page is given the insets and paints them itself. */
    static final int EDGE_NONE = 4;

    private static final String EDGE_COLOR = "edge_color";

    // The page's resolved theme, as the page itself last reported it through
    // the wudict://theme channel. Cached here for ONE reason: the first frame
    // is painted before a WebView has loaded anything, so without a remembered
    // answer EDGE_PAGE would have to guess on every cold start. It is written
    // by MainActivity and read by nobody else.
    private static final String PAGE_DARK = "page_dark";

    // ── which system bars hide (D141) ────────────────────────────────────────
    //
    // A BITMASK, not an enum, because the four states are exactly the subsets of
    // a two-element set and the mask is what WindowInsetsController wants anyway.
    static final String BARS = "hide_bars";

    static final int BARS_OFF = 0;
    static final int BARS_STATUS = 1;
    static final int BARS_NAV = 2;
    static final int BARS_BOTH = BARS_STATUS | BARS_NAV;

    private ShellPrefs() {
    }

    private static boolean migrated;

    // What an older build cached /api/config's resolved values under (D101).
    // Nothing writes it any more - see migrate(), which is also what removes
    // what an existing install is still carrying.
    private static final String CACHE_PREFIX = "effective_";

    static SharedPreferences of(Context c) {
        SharedPreferences p = c.getSharedPreferences(FILE, Context.MODE_PRIVATE);
        migrate(p);
        return p;
    }

    /**
     * Brings a preference file written by an older version up to date, once per
     * process.
     *
     * <p>Hung off {@link #of} rather than called from an activity because it has
     * to run BEFORE the file is written, and three activities plus a service can
     * each be the first thing this process starts. Every read and every write in
     * this class goes through {@code of()}, so no caller can beat it - which is
     * what makes the "is this file empty?" test below mean what it says.
     *
     * <p>Each step is guarded by {@code contains()}, so running it twice is the
     * same as running it once and a user's later choice is never reverted.
     */
    private static synchronized void migrate(SharedPreferences p) {
        if (migrated) return;
        migrated = true;
        SharedPreferences.Editor e = null;

        if (!p.contains(EDGE_MODE)) {
            // An EXISTING install keeps what it has always looked like; only a
            // genuinely new one gets the better default. An upgrade is
            // recognised by the file having any existing shell preferences.
            boolean fresh = p.getAll().isEmpty();
            e = p.edit().putInt(EDGE_MODE, fresh ? EDGE_PAGE : EDGE_SYSTEM);
        }

        if (!p.contains(BARS)) {
            int v = p.getBoolean(IMMERSIVE, false) ? BARS_BOTH : BARS_OFF;
            if (e == null) e = p.edit();
            e.putInt(BARS, v);
        }

        // The effective-values cache is GONE (2026-09-28). Its only reader was
        // the shell screen's inherited-value display, and the rows are drawn by
        // the app page now, which asks the server for those values itself. An
        // older file keeps them forever otherwise, and a preference nobody
        // reads is a fact nobody can check - so this is where they go.
        for (String key : new java.util.ArrayList<>(p.getAll().keySet())) {
            if (!key.startsWith(CACHE_PREFIX)) continue;
            if (e == null) e = p.edit();
            e.remove(key);
        }

        if (e != null) e.apply();
    }

    /** Whether lookups arriving this way skip the popup and open the app. */
    static boolean opensApp(Context c, String key) {
        return of(c).getBoolean(key, false);
    }

    /**
     * Whether the app's transient messages are shown ({@link #INFO_MESSAGES}).
     * Read by MainActivity for its first frame and by Shell for the page's URL;
     * the only key here whose default is ON, which is why it has its own reader
     * rather than going through {@link #opensApp}.
     */
    static boolean infoMessages(Context c) {
        return of(c).getBoolean(INFO_MESSAGES, true);
    }

    static void set(Context c, String key, boolean on) {
        of(c).edit().putBoolean(key, on).apply();
    }

    /** Which system bars the app window hides. A {@code BARS_*} mask. */
    static int bars(Context c) {
        return of(c).getInt(BARS, BARS_OFF);
    }

    static void setBars(Context c, int mask) {
        of(c).edit().putInt(BARS, mask & BARS_BOTH).apply();
    }

    /** How the window's edges are painted. One of the {@code EDGE_*} constants. */
    static int edgeMode(Context c) {
        int v = of(c).getInt(EDGE_MODE, EDGE_SYSTEM);
        return v < EDGE_SYSTEM || v > EDGE_NONE ? EDGE_SYSTEM : v;
    }

    static void setEdgeMode(Context c, int mode) {
        of(c).edit().putInt(EDGE_MODE, mode).apply();
    }

    /** The stored custom strip colour. Opaque black until the user picks one. */
    static int edgeColorValue(Context c) {
        return of(c).getInt(EDGE_COLOR, 0xFF000000);
    }

    static void setEdgeColorValue(Context c, int argb) {
        of(c).edit().putInt(EDGE_COLOR, 0xFF000000 | argb).apply();
    }

    /** What the page last reported its own resolved theme to be. */
    static boolean pageDark(Context c) {
        // Falls back to the OS, which is what the page's own default - "auto" -
        // resolves to anyway, so a first-ever launch guesses the way the page
        // is about to decide rather than at random.
        return of(c).getBoolean(PAGE_DARK, systemDark(c));
    }

    /** Records the page's resolved theme. True when the value changed. */
    static boolean setPageDark(Context c, boolean dark) {
        SharedPreferences p = of(c);
        if (p.contains(PAGE_DARK) && p.getBoolean(PAGE_DARK, false) == dark) return false;
        p.edit().putBoolean(PAGE_DARK, dark).apply();
        return true;
    }

    static boolean systemDark(Context c) {
        return (c.getResources().getConfiguration().uiMode
                & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
    }

    /**
     * The page's own background colour, per whatever theme it is showing -
     * the colour every window this side paints wears: the native dialogs, the
     * lookup popup, the WebView behind its first frame, and the strip the
     * system bars sit on under EDGE_PAGE.
     *
     * <p>Three answers in the order the app page itself resolves them: the
     * reader's window colour (which pins the page's --bg), the paper an
     * enabled preset paints (the layer's --bg), and the app's built-in
     * background - the same value the page's own --bg token carries, so
     * nothing here is a colour of this side's own invention.
     */
    static int pageBg(Context c) {
        boolean night = night(c);
        if (sepiaFor(c, night)) return sepiaColorFor(c, night);
        String paper = paperFor(c, night);
        if (!paper.isEmpty()) return Color.parseColor(paper) | 0xFF000000;
        return c.getColor(pageDark(c) ? R.color.page_bg_dark : R.color.page_bg_light);
    }

    /**
     * The colour the shell paints into the inset padding - the strip behind the
     * camera, and the band under the gesture bar.
     *
     * <p>{@link #EDGE_NONE} has no strip, but the window still needs a colour to
     * paint before the first frame and behind a side cutout, and the page's own
     * background is the only answer that cannot flash.
     */
    static int edgeColor(Context c) {
        switch (edgeMode(c)) {
            case EDGE_BLACK:
                return 0xFF000000;
            case EDGE_CUSTOM:
                return edgeColorValue(c);
            case EDGE_PAGE:
            case EDGE_NONE:
                return pageBg(c);
            default:
                return c.getColor(R.color.window_bg);
        }
    }

    /**
     * Whether the system bars should draw their icons DARK - which they must
     * whenever the colour behind them is light.
     *
     * <p>Decided by contrast rather than by the day/night setting, because a
     * custom colour has no day/night setting to consult and a strip that follows
     * the page can disagree with the OS outright. The two candidate contrast
     * ratios are computed and the better one wins, so there is no threshold here
     * to be wrong about: it is arithmetic, not a preference, which is why it is
     * not a row on the settings screen.
     */
    static boolean darkIcons(int argb) {
        double l = luminance(argb);
        double onDark = (l + 0.05) / 0.05;          // black icons over this colour
        double onLight = 1.05 / (l + 0.05);         // white icons over this colour
        return onDark >= onLight;
    }

    /** WCAG 2.x relative luminance. */
    private static double luminance(int argb) {
        return 0.2126 * channel(Color.red(argb))
                + 0.7152 * channel(Color.green(argb))
                + 0.0722 * channel(Color.blue(argb));
    }

    private static double channel(int v) {
        double s = v / 255.0;
        return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
    }

    /**
     * Which key governs this launch. An action we do not recognise - or none at
     * all, which a hand-written intent can easily arrive with - falls to
     * TOOLBAR, whose default is the floating window: an unrecognised caller
     * gets the least intrusive outcome rather than a hijacked task.
     */
    static String sourceKey(Intent i) {
        // Before VIEW: Dictan's dispatcher is a VIEW told apart by its category.
        if (LookupActivity.fromReader(i)) return READER;
        String action = i == null ? null : i.getAction();
        if (Intent.ACTION_SEND.equals(action)) return SHARE;
        if (Intent.ACTION_VIEW.equals(action)) return LINK;
        return TOOLBAR;
    }

    // ── platform overrides ───────────────────────────────────────────────────

    /** A switch: stored on/off, emitted as {@link Override#onValue} when on. */
    static final int BOOL = 0;
    /** A count: stored and emitted verbatim, validated against [min, max]. */
    static final int COUNT = 1;
    /** Megabytes: stored as a bare number, emitted with the "MB" suffix. */
    static final int MEGABYTES = 2;

    // Bounds that only the device can answer. Resolved by maxOf().
    private static final int MAX_RAM_MB = -1;
    private static final int MAX_CORES = -2;

    /**
     * One overridable config key. Everything the settings screen draws and
     * the runtime's tuning values come from this table, so a second
     * override is one entry and two strings - which is the point of D101;
     * System-level Go runtime tuning.
     */
    static final class Override {
        final String key;      // the wudict config key
        final String flag;     // argv flag, or null to deliver through the environment
        final int kind;
        final String onValue;  // BOOL: what "on" emits
        final String offValue; // BOOL: what "off" means, for display only - never emitted
        final int min, max;    // COUNT/MEGABYTES, inclusive; negative = ask the device
        final int label, hint;

    // Package-private so the system settings bridge can serialize each row.
        Override(String key, String flag, int kind, String onValue, String offValue,
                         int min, int max, int label, int hint) {
            this.key = key;
            this.flag = flag;
            this.kind = kind;
            this.onValue = onValue;
            this.offValue = offValue;
            this.min = min;
            this.max = max;
            this.label = label;
            this.hint = hint;
        }

        String pref() {
            return "override_" + key;
        }
    }

    /** Retained only to read the previous WebView profile during first-run import. */
    static final String LEGACY_HOST = "127.0.0.1";
    /** Previous WebView localStorage was keyed by this origin and port. */
    static final int LEGACY_DEFAULT_PORT = 6889;

    // This table contains application tuning only. Android has no listener;
    // SERVER_PORT is read separately only for the one-time old-origin import.
    static final Override[] OVERRIDES = {};

    /** The stored override, or null when the key is left to the config. */
    static String override(Context c, Override o) {
        String v = of(c).getString(o.pref(), null);
        return v == null || v.isEmpty() ? null : v;
    }

    /** Stores an override; null or empty withdraws it. */
    static void setOverride(Context c, Override o, String value) {
        SharedPreferences.Editor e = of(c).edit();
        if (value == null || value.isEmpty()) {
            e.remove(o.pref());
        } else {
            e.putString(o.pref(), value);
        }
        e.apply();
    }

    /**
     * Stores an override after checking it against the row's own rules: a
     * whole number inside [min, maxOf], or a BOOL's ON marker and nothing
     * else. Empty withdraws the override, which is what "follow the settings
     * file" is.
     *
     * <p>This is the check the shell settings screen's commit() has always
     * made; it lives here now that a second control writes the same keys (the
     * app page's System window, over the wudict:system bridge), because the
     * value it guards is emitted on a child's exec line: a number the child
     * cannot use, or a BOOL's off-state stored as a value, would be this side
     * inventing its own config. The screen keeps the user-facing half - it
     * says WHY a value was refused, in the field's own terms.
     *
     * @throws IllegalArgumentException when the value cannot be stored as given
     */
    static void setOverrideChecked(Context c, Override o, String value) {
        String v = value == null ? "" : value.trim();
        if (v.isEmpty()) {
            setOverride(c, o, null);
            return;
        }
        if (o.kind == BOOL) {
            // Two states, never three: "off" is the absence of the key.
            if (!o.onValue.equals(v)) throw new IllegalArgumentException(v);
            setOverride(c, o, o.onValue);
            return;
        }
        long n;
        try {
            n = Long.parseLong(v);
        } catch (NumberFormatException e) {
            throw new IllegalArgumentException(v);
        }
        int max = maxOf(c, o);
        if (n < o.min || n > max) throw new IllegalArgumentException(v);
        setOverride(c, o, String.valueOf(n)); // canonical: "0128" is stored as "128"
    }

    static void clearOverrides(Context c) {
        SharedPreferences.Editor e = of(c).edit();
        for (Override o : OVERRIDES) e.remove(o.pref());
        e.remove("override_INDEX_WORKERS"); // obsolete setting from earlier versions
        e.apply();
    }

    static boolean anyOverride(Context c) {
        for (Override o : OVERRIDES) {
            if (override(c, o) != null) return true;
        }
        return false;
    }

    /** The value this application tuning key contributes to Go configuration. */
    static String emitted(Context c, Override o) {
        String v = override(c, o);
        if (v == null) return null;
        return o.kind == MEGABYTES ? v + "MB" : v;
    }

    /** The environment additions for a spawn: the env-delivered overrides. */
    static Map<String, String> env(Context c) {
        Map<String, String> out = new LinkedHashMap<>();
        for (Override o : OVERRIDES) {
            if (o.flag != null) continue;
            String v = emitted(c, o);
            if (v != null) out.put(o.key, v);
        }
        return out;
    }

    /** The table entry for a key. Never null for a key spelled as in OVERRIDES. */
    static Override byKey(String key) {
        for (Override o : OVERRIDES) {
            if (o.key.equals(key)) return o;
        }
        throw new IllegalArgumentException(key);
    }

    /** Prior install's port, used only to read localStorage from its old WebView origin. */
    static int legacyOriginPort(Context c) {
        String v = of(c).getString("override_SERVER_PORT", null);
        if (v == null || v.isEmpty()) return LEGACY_DEFAULT_PORT;
        try {
            int n = Integer.parseInt(v);
            return n >= 1024 && n <= 65535 ? n : LEGACY_DEFAULT_PORT;
        } catch (NumberFormatException e) {
            return LEGACY_DEFAULT_PORT; // a value we cannot use is a value we do not use
        }
    }

    /**
     * The upper bound for a numeric row, resolved against the device rather
     * than a constant: RAM for the memory caps, cores for the workers. A phone
     * with more of either simply gets more room, with nothing to revisit.
     */
    static int maxOf(Context c, Override o) {
        if (o.max == MAX_RAM_MB) return (int) Math.max(64, deviceRamMb(c));
        if (o.max == MAX_CORES) return availableCpuCores();
        return o.max;
    }

    /** Android's process quota can make the JVM report fewer CPUs than the device has online. */
    static int availableCpuCores() {
        try (BufferedReader reader = new BufferedReader(new FileReader("/sys/devices/system/cpu/online"))) {
            String value = reader.readLine();
            if (value != null) {
                int count = 0;
                for (String range : value.trim().split(",")) {
                    String[] ends = range.split("-", -1);
                    int first = Integer.parseInt(ends[0]);
                    int last = ends.length == 1 ? first : Integer.parseInt(ends[1]);
                    if (ends.length > 2 || first < 0 || last < first || last > 4095) return runtimeCpuCores();
                    count += last - first + 1;
                }
                if (count > 0) return count;
            }
        } catch (Exception ignored) {
            // Some Android builds restrict sysfs reads; the JVM count is the fallback.
        }
        return runtimeCpuCores();
    }

    private static int runtimeCpuCores() {
        return Math.max(1, Runtime.getRuntime().availableProcessors());
    }

    static long deviceRamMb(Context c) {
        ActivityManager am = (ActivityManager) c.getSystemService(Context.ACTIVITY_SERVICE);
        if (am == null) return 4096; // unreachable in practice; a sane phone-sized answer
        ActivityManager.MemoryInfo mi = new ActivityManager.MemoryInfo();
        am.getMemoryInfo(mi);
        return mi.totalMem >> 20;
    }
}
