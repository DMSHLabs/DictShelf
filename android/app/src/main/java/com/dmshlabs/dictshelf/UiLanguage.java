// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.dmshlabs.dictshelf;

import android.content.Context;
import android.content.res.Configuration;
import android.content.res.Resources;
import android.os.LocaleList;
import android.view.ContextThemeWrapper;
import org.json.JSONObject;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;

/** Only app resources follow Language. Never change Locale.setDefault: speech
 * and dictionary processing have their own languages. Go owns/writes the file. */
final class UiLanguage {
    private static long modified = Long.MIN_VALUE, length = -1;
    private static String code = "en";
    private static Resources source, translated;
    private static Configuration configuration;
    private static String resourceCode;

    private UiLanguage() {}

    static synchronized String selected(Context context) {
        File state = new File(AppDirs.home(context), ".dmshlabs/dictshelf/state.json");
        long stamp = state.lastModified(), size = state.length();
        if (stamp != modified || size != length) {
            String choice = "system";
            try {
                JSONObject json = new JSONObject(new String(Files.readAllBytes(state.toPath()), StandardCharsets.UTF_8));
                String saved = json.optString("language", "system");
                if ("en".equals(saved) || "ru".equals(saved)) choice = saved;
            } catch (Exception ignored) { /* first run or unreadable state: System */ }
            modified = stamp;
            length = size;
            code = "system".equals(choice) ? systemLanguage(context) : choice;
        }
        return code;
    }

    private static String systemLanguage(Context context) {
        LocaleList locales = context.getResources().getConfiguration().getLocales();
        for (int i = 0; i < locales.size(); i++) {
            String language = locales.get(i).getLanguage();
            if ("ru".equals(language)) return "ru";
            if ("en".equals(language)) return "en";
        }
        return "en";
    }

    static Context context(Context base) {
        Configuration config = new Configuration();
        config.setLocales(LocaleList.forLanguageTags(selected(base)));
        // Override resources while retaining the Activity's window services.
        // A bare createConfigurationContext loses its window token: dialogs
        // then fail in show() with BadTokenException (token null).
        ContextThemeWrapper localized = new ContextThemeWrapper(base, 0);
        localized.applyOverrideConfiguration(config);
        return localized;
    }

    static synchronized Resources resources(Context base, Resources original) {
        if (base == null) return original;
        String language = selected(base);
        Configuration current = original.getConfiguration();
        if (source != original || !language.equals(resourceCode) || !current.equals(configuration)) {
            configuration = new Configuration(current);
            Configuration localized = new Configuration(current);
            localized.setLocales(LocaleList.forLanguageTags(language));
            translated = base.createConfigurationContext(localized).getResources();
            source = original;
            resourceCode = language;
        }
        return translated;
    }
}
