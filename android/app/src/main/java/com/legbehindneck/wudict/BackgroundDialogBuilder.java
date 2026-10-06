// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.app.AlertDialog;
import android.content.Context;
import android.view.Window;
import android.view.WindowManager;

/** Shared surface for app-owned dialogs, including those shown via create(). */
final class BackgroundDialogBuilder extends AlertDialog.Builder {
    private final Context context;

    BackgroundDialogBuilder(Context context) {
        super(UiLanguage.context(context), ShellPrefs.darkIcons(ShellPrefs.pageBg(context))
                ? android.R.style.Theme_Material_Light_Dialog_Alert
                : android.R.style.Theme_Material_Dialog_Alert);
        this.context = context;
    }

    @Override public AlertDialog create() {
        AlertDialog dialog = super.create();
        Window window = dialog.getWindow();
        window.setBackgroundDrawable(WindowBackground.dialogDrawable(context, ShellPrefs.pageBg(context)));
        window.addFlags(WindowManager.LayoutParams.FLAG_DIM_BEHIND);
        window.setDimAmount(.42f);
        return dialog;
    }
}
