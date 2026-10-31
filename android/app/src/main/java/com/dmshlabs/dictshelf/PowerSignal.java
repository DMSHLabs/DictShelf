// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package com.dmshlabs.dictshelf;

import android.util.Log;

/**
 * Tells the Go runtime how much of the device it may use (D64).
 *
 * Go cannot see a lifecycle callback, a thermal event or a battery state -
 * every one of those is an Android API, and the shell is the only part of this
 * app that has one. So the shell watches and sends a single verdict through
 * JNI. Nothing is added to the Go program's platform knowledge: it is told a
 * state, not a platform.
 *
 * Delivery is deliberately lossy in one direction and stubborn in the other.
 * A dropped "active" costs a moment of conservatism; a dropped "background"
 * costs battery for as long as the app stays away, so a failed send is retried
 * a few times. States coalesce: only the latest one matters, and sending an
 * obsolete one would be wrong as well as wasteful.
 */
final class PowerSignal {

    private static final String TAG = "wudict";

    static final String ACTIVE = "active";
    static final String BACKGROUND = "background";
    static final String RESTRICTED = "restricted";
    static final String SAVING = "saving";
    static final String THERMAL_MODERATE = "thermal-moderate";
    static final String THERMAL_SEVERE = "thermal-severe";

    private static final int ATTEMPTS = 3;
    private static final long RETRY_MS = 800;
    // How long the sender waits for more work before ending. A parked thread is
    // a stack this app has no use for while nothing is changing.
    private static final long IDLE_MS = 30_000;

    private static final Object LOCK = new Object();
    private static final Object SEND_LOCK = new Object();
    private static String pending;   // newest state not yet delivered
    private static String delivered; // newest state Go acknowledged
    private static String requested; // latest state, replayed when Go starts
    private static boolean running;  // a sender thread is alive

    private PowerSignal() {}

    /** Requests that the runtime move to {@code state}; returns immediately. */
    static void set(String state) {
        synchronized (LOCK) {
            if (state.equals(pending) || (pending == null && state.equals(delivered))) {
                return; // already there, or already on its way there
            }
            requested = state;
            pending = state;
            LOCK.notifyAll();
            if (running) return;
            running = true;
        }
        Thread t = new Thread(PowerSignal::pump, "wudict-power");
        t.setDaemon(true);
        t.start();
    }

    /** Applies a freshly sampled state before the Go side starts user-requested work. */
    static void setNow(String state) {
        boolean startPump = false;
        synchronized (LOCK) {
            requested = state;
            pending = state;
            LOCK.notifyAll();
            if (!running) {
                running = true;
                startPump = true;
            }
        }
        boolean current;
        boolean ok = false;
        synchronized (SEND_LOCK) {
            synchronized (LOCK) {
                current = state.equals(pending);
            }
            if (current) ok = post(state);
        }
        boolean needPump = false;
        synchronized (LOCK) {
            if (state.equals(pending) && ok) {
                pending = null;
                delivered = state;
            }
            if (startPump) {
                if (pending == null) running = false;
                else needPump = true;
            }
        }
        if (needPump) {
            Thread t = new Thread(PowerSignal::pump, "wudict-power");
            t.setDaemon(true);
            t.start();
        }
    }

    /** Re-sends the last device state after the in-process Go runtime starts. */
    static void runtimeReady() {
        String state;
        synchronized (LOCK) {
            state = requested;
            delivered = null;
        }
        if (state != null) set(state);
    }

    private static void pump() {
        for (;;) {
            String state;
            synchronized (LOCK) {
                long deadline = System.currentTimeMillis() + IDLE_MS;
                while (pending == null) {
                    long left = deadline - System.currentTimeMillis();
                    if (left <= 0) {
                        running = false;
                        return;
                    }
                    try {
                        LOCK.wait(left);
                    } catch (InterruptedException e) {
                        running = false;
                        return;
                    }
                }
                state = pending;
            }
            boolean ok = false;
            for (int i = 0; i < ATTEMPTS && !ok; i++) {
                if (i > 0) {
                    synchronized (LOCK) {
                        if (!state.equals(pending)) break; // superseded: abandon this one
                        try {
                            LOCK.wait(RETRY_MS);
                        } catch (InterruptedException e) {
                            running = false;
                            return;
                        }
                        if (!state.equals(pending)) break;
                    }
                }
                ok = postCurrent(state);
            }
            synchronized (LOCK) {
                if (state.equals(pending)) {
                    pending = null;
                    if (ok) delivered = state;
                }
            }
        }
    }

    private static boolean post(String state) {
        try {
            boolean ok = GoBridge.setPower(state);
            if (!ok) Log.d(TAG, "power " + state + ": Go runtime unavailable");
            return ok;
        } catch (RuntimeException e) {
            Log.d(TAG, "power " + state + ": " + e);
            return false;
        }
    }

    private static boolean postCurrent(String state) {
        synchronized (SEND_LOCK) {
            synchronized (LOCK) {
                if (!state.equals(pending)) return true;
            }
            return post(state);
        }
    }
}
