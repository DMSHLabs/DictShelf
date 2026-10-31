// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Everything the server is allowed to know about the device's mood is decided
// here, in one function, from fields that are the only inputs (D64). One place,
// because these inputs contradict each other - the app can be visible while the
// device is hot, backgrounded while charging - and a set of independent
// callbacks each pushing its own state would make the last event win rather
// than the strictest condition.
//
// A class of its own because of D67: the lookup popup is a SECOND window that
// can be visible, so visibility is a count, not MainActivity's boolean.
//
// The battery and power-save inputs are SAMPLED here rather than watched with
// broadcast receivers: a receiver is a wakeup the app would not otherwise take,
// which is the exact cost this whole mechanism exists to avoid, and every
// transition that matters already calls apply().
package com.dmshlabs.dictshelf;

import android.content.Context;
import android.os.BatteryManager;
import android.os.PowerManager;

final class Power {

    // Both mutated from the main thread only (activity lifecycle, and the
    // thermal listener posted to the main executor), so no locking; volatile
    // for the reader in a delayed post.
    private static volatile int windows; // activities between onStart and onStop
    private static volatile int thermal; // PowerManager.THERMAL_STATUS_*, 0 when unknown

    private Power() {
    }

    /** A window became visible (onStart). */
    static void enter(Context c) {
        windows++;
        apply(c);
    }

    /** A window went away (onStop). */
    static void exit(Context c) {
        if (windows > 0) windows--;
        apply(c);
    }

    static void thermal(Context c, int status) {
        thermal = status;
        apply(c);
    }

    static void apply(Context c) {
        PowerSignal.set(state(c));
    }

    /** Re-samples power-save and charging state synchronously before a new job. */
    static void applyForWork(Context c) {
        PowerSignal.setNow(state(c));
    }

    private static String state(Context c) {
        PowerManager pm = c.getSystemService(PowerManager.class);
        boolean saving = pm != null && pm.isPowerSaveMode();

        if (thermal >= PowerManager.THERMAL_STATUS_SEVERE) {
            return PowerSignal.THERMAL_SEVERE;
        } else if (thermal >= PowerManager.THERMAL_STATUS_MODERATE) {
            return PowerSignal.THERMAL_MODERATE;
        } else if (saving) {
            return PowerSignal.SAVING;
        } else if (windows == 0) {
            // Charging is the one case where staying active off-screen is
            // defensible: preparing a dictionary is the only real work this
            // app has, it is what the user is waiting for, and a plugged-in
            // device is not spending the user's battery on it.
            return charging(c) ? PowerSignal.ACTIVE : PowerSignal.BACKGROUND;
        } else {
            return PowerSignal.ACTIVE;
        }
    }

    private static boolean charging(Context c) {
        BatteryManager bm = c.getSystemService(BatteryManager.class);
        return bm != null && bm.isCharging();
    }
}
