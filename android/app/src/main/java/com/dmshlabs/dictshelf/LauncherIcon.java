// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package com.dmshlabs.dictshelf;

import android.content.ComponentName;
import android.content.Context;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.os.Build;

import java.util.ArrayList;
import java.util.List;

final class LauncherIcon {
    private static final String PREFS = "launcher_icon";
    private static final String CHOICE = "choice";
    private static final String LEGACY_FAMILY = "family";
    private static final String AUTO = "auto";
    private static final String[] ALIASES = {
        ".LauncherIcon2", ".LauncherIcon2Transparent", ".LauncherIcon2TransparentLight"
    };

    private LauncherIcon() {}

    static void set(Context context, String family, boolean automatic) {
        String choice = "icon2_transparent_light".equals(family) ? family : "icon2_transparent";
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putString(CHOICE, choice).putBoolean(AUTO, automatic).apply();
        apply(context);
    }

    static void apply(Context context) {
        android.content.SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        String choice = prefs.getString(CHOICE, null);
        if (choice == null) {
            String legacy = prefs.getString(LEGACY_FAMILY, "icon2");
            choice = "light".equals(legacy) ? "icon2_transparent_light" : "icon2_transparent";
            prefs.edit().putString(CHOICE, choice).apply();
        }
        if (!"icon2_transparent_light".equals(choice)) {
            choice = "icon2_transparent";
            prefs.edit().putString(CHOICE, choice).apply();
        }
        boolean automatic = prefs.getBoolean(AUTO, true);
        boolean night = (context.getResources().getConfiguration().uiMode
            & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
        String selected = "icon2_transparent_light".equals(choice) ? ".LauncherIcon2TransparentLight"
            : automatic && night ? ".LauncherIcon2TransparentLight" : ".LauncherIcon2Transparent";
        if (selected.equals(prefs.getString("applied", ""))) return;

        PackageManager pm = context.getPackageManager();
        List<PackageManager.ComponentEnabledSetting> changes = new ArrayList<>();
        for (String alias : ALIASES) {
            ComponentName component = new ComponentName(context, LauncherIcon.class.getPackage().getName() + alias);
            boolean enabled = selected.equals(alias);
            if (Build.VERSION.SDK_INT >= 33) {
                changes.add(new PackageManager.ComponentEnabledSetting(component,
                    enabled ? PackageManager.COMPONENT_ENABLED_STATE_ENABLED : PackageManager.COMPONENT_ENABLED_STATE_DISABLED,
                    PackageManager.DONT_KILL_APP));
            } else {
                if (enabled) pm.setComponentEnabledSetting(component,
                    PackageManager.COMPONENT_ENABLED_STATE_ENABLED, PackageManager.DONT_KILL_APP);
            }
        }
        if (Build.VERSION.SDK_INT >= 33) pm.setComponentEnabledSettings(changes);
        else for (String alias : ALIASES) if (!selected.equals(alias)) {
            pm.setComponentEnabledSetting(new ComponentName(context, LauncherIcon.class.getPackage().getName() + alias),
                PackageManager.COMPONENT_ENABLED_STATE_DISABLED, PackageManager.DONT_KILL_APP);
        }
        prefs.edit().putString("applied", selected).apply();
    }
}
