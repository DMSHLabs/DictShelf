// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package com.legbehindneck.wudict;

import android.app.Activity;
import android.app.AlertDialog;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.ColorFilter;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PixelFormat;
import android.graphics.Typeface;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.text.Editable;
import android.text.SpannableString;
import android.text.Spanned;
import android.text.TextWatcher;
import android.text.style.BackgroundColorSpan;
import android.text.style.ForegroundColorSpan;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.webkit.JsPromptResult;
import android.widget.ArrayAdapter;
import android.widget.Button;
import android.widget.EditText;
import android.widget.ImageButton;
import android.widget.TextView;
import android.widget.Spinner;
import android.widget.ListView;
import android.widget.LinearLayout;
import android.widget.AdapterView;
import android.webkit.WebView;
import java.util.ArrayList;
import java.util.List;
import java.util.WeakHashMap;
import org.json.JSONArray;
import org.json.JSONObject;
import org.json.JSONException;

/** A themed replacement for WebView's dictionary select popup. */
final class DictionaryPicker {
    private static final WeakHashMap<WebView, Live> live = new WeakHashMap<>();

    // The opening prompt is answered immediately. JavaScript may then search and
    // stream new rows while this native window remains open.
    static void showLive(Activity activity, WebView web, String payload, JsPromptResult result) {
        if (activity.isFinishing() || activity.isDestroyed()) { result.cancel(); return; }
        try {
            Live previous = live.remove(web);
            if (previous != null) previous.dialog.dismiss();
            Live picker = new Live(activity, web, payload);
            live.put(web, picker);
            picker.dialog.show();
            picker.dialog.getWindow().setSoftInputMode(
                    WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE |
                    WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
            picker.dialog.getWindow().setBackgroundDrawable(
                    WindowBackground.dialogDrawable(activity, ShellPrefs.pageBg(activity)));
            result.confirm("open");
        } catch (JSONException bad) { result.cancel(); }
    }

    static void updateLive(WebView web, String payload) {
        Live picker = live.get(web);
        if (picker == null) return;
        try { picker.update(new JSONObject(payload)); }
        catch (JSONException ignored) { }
    }

    private static final class Live {
        final WebView web;
        final AlertDialog dialog;
        final Spinner groups;
        final ListView list;
        final ArrayAdapter<String> groupAdapter, rowAdapter;
        final ImageButton searchMode;
        final EditText searchInput;
        final TextView searchCount;
        final Button previous, next;
        final JSONObject searchLabels;
        final Activity activity;
        final int backgroundColor, textColor, accent;
        final List<Integer> shownRows = new ArrayList<>(), matchingRows = new ArrayList<>();
        JSONArray rows = new JSONArray();
        JSONArray groupValues = new JSONArray();
        boolean updating;
        final boolean found;
        boolean filtering;
        int activeMatch = -1;
        String empty = "No dictionaries";
        String groupId = "all";

        Live(Activity activity, WebView web, String payload) throws JSONException {
            this.activity = activity;
            this.web = web;
            JSONObject initial = new JSONObject(payload);
            found = initial.optBoolean("found");
            searchLabels = initial.optJSONObject("listSearch") == null
                    ? new JSONObject() : initial.optJSONObject("listSearch");
            int color = ShellPrefs.pageBg(activity);
            backgroundColor = color;
            textColor = ShellPrefs.darkIcons(color) ? 0xDE000000 : 0xFFFFFFFF;
            int parsedAccent;
            try { parsedAccent = Color.parseColor(searchLabels.optString("accent", "#e08600")); }
            catch (IllegalArgumentException invalid) { parsedAccent = 0xFFE08600; }
            accent = parsedAccent;
            int pad = (int)(12 * activity.getResources().getDisplayMetrics().density);
            LinearLayout body = new LinearLayout(activity);
            body.setOrientation(LinearLayout.VERTICAL);
            body.setPadding(pad, pad, pad, pad);
            groups = new Spinner(activity);
            groups.setPopupBackgroundDrawable(WindowBackground.dialogDrawable(activity, color));
            groupAdapter = new ArrayAdapter<String>(activity, android.R.layout.simple_spinner_item) {
                @Override public View getView(int position, View recycled, ViewGroup parent) {
                    TextView text = (TextView) super.getView(position, recycled, parent);
                    text.setTextColor(textColor);
                    return text;
                }
                @Override public View getDropDownView(int position, View recycled, ViewGroup parent) {
                    TextView text = (TextView) super.getDropDownView(position, recycled, parent);
                    text.setTextColor(textColor);
                    return text;
                }
            };
            groupAdapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);
            groups.setAdapter(groupAdapter);
            LinearLayout heading = new LinearLayout(activity);
            heading.setOrientation(LinearLayout.HORIZONTAL);
            heading.setGravity(Gravity.CENTER_VERTICAL);
            heading.addView(groups, new LinearLayout.LayoutParams(0,
                    ViewGroup.LayoutParams.WRAP_CONTENT, 1));
            Button close = searchStep(activity, "✕", 20, color, textColor);
            close.setContentDescription(activity.getString(android.R.string.cancel));
            LinearLayout.LayoutParams closeLayout = new LinearLayout.LayoutParams(dp(activity, 40), dp(activity, 40));
            closeLayout.leftMargin = dp(activity, 8);
            heading.addView(close, closeLayout);
            body.addView(heading);
            View separator = new View(activity);
            separator.setBackgroundColor((textColor & 0x00FFFFFF) | 0x33000000);
            LinearLayout.LayoutParams separatorLayout = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    Math.max(1, (int) activity.getResources().getDisplayMetrics().density));
            separatorLayout.topMargin = pad;
            separatorLayout.bottomMargin = pad / 2;
            body.addView(separator, separatorLayout);
            list = new ListView(activity);
            rowAdapter = new ArrayAdapter<String>(activity, found
                    ? android.R.layout.simple_list_item_1
                    : android.R.layout.simple_list_item_single_choice) {
                @Override public View getView(int position, View recycled, ViewGroup parent) {
                    TextView text = (TextView) super.getView(position, recycled, parent);
                    text.setTextColor(textColor);
                    int source = position < shownRows.size() ? shownRows.get(position) : -1;
                    String label = getItem(position);
                    String query = searchInput.getText().toString().trim();
                    if (source >= 0 && !query.isEmpty() && label != null) {
                        SpannableString highlighted = new SpannableString(label);
                        for (int at = matchStart(label, query, 0); at >= 0;
                                at = matchStart(label, query, at + query.length())) {
                            highlighted.setSpan(new BackgroundColorSpan(accent), at,
                                    at + query.length(), Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                            highlighted.setSpan(new ForegroundColorSpan(
                                    ShellPrefs.darkIcons(accent) ? Color.BLACK : Color.WHITE),
                                    at, at + query.length(), Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                        }
                        text.setText(highlighted);
                    }
                    boolean match = !filtering && !query.isEmpty() && matchingRows.contains(source);
                    boolean current = match && activeMatch >= 0
                            && activeMatch < matchingRows.size()
                            && source == matchingRows.get(activeMatch);
                    if (match) {
                        GradientDrawable outline = new GradientDrawable();
                        outline.setColor(current ? mix(backgroundColor, accent, 14) : Color.TRANSPARENT);
                        outline.setStroke(dp(activity, 2), accent);
                        outline.setCornerRadius(dp(activity, 8));
                        text.setBackground(outline);
                    } else text.setBackgroundColor(Color.TRANSPARENT);
                    return text;
                }
            };
            list.setAdapter(rowAdapter);
            rowAdapter.setNotifyOnChange(false);
            if (!found) list.setChoiceMode(ListView.CHOICE_MODE_SINGLE);
            list.setMinimumHeight((int)(220 * activity.getResources().getDisplayMetrics().density));
            body.addView(list, new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, 0, 1));
            View searchDivider = new View(activity);
            searchDivider.setBackgroundColor(ShellPrefs.darkIcons(color) ? 0x44000000 : 0x66FFFFFF);
            LinearLayout.LayoutParams dividerLayout = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, Math.max(1, dp(activity, 1)));
            dividerLayout.topMargin = dp(activity, 6);
            body.addView(searchDivider, dividerLayout);
            LinearLayout dock = new LinearLayout(activity);
            dock.setOrientation(LinearLayout.HORIZONTAL);
            dock.setGravity(Gravity.CENTER_VERTICAL);
            dock.setContentDescription(searchLabels.optString("search", "Search dictionary list"));
            LinearLayout.LayoutParams dockLayout = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            dockLayout.topMargin = dp(activity, 6);
            body.addView(dock, dockLayout);
            int control = dp(activity, (float)searchLabels.optDouble("height", 43.2));
            float font = (float)searchLabels.optDouble("font", 16);
            int gap = dp(activity, 4);
            int iconPadding = Math.min(dp(activity, 9), control / 5);
            searchMode = new ImageButton(activity);
            searchMode.setBackground(controlBackground(activity, color, false, accent));
            searchMode.setImageDrawable(new SearchModeIcon(textColor,
                    Math.min(dp(activity, 22), Math.max(1, control - iconPadding * 2))));
            searchMode.setPadding(iconPadding, iconPadding, iconPadding, iconPadding);
            dock.addView(searchMode, new LinearLayout.LayoutParams(control, control));
            searchInput = new EditText(activity);
            searchInput.setSingleLine(true);
            searchInput.setImeOptions(EditorInfo.IME_ACTION_SEARCH);
            searchInput.setHint(searchLabels.optString("placeholder", "Find dictionaries…"));
            searchInput.setContentDescription(searchLabels.optString("search", "Search dictionary list"));
            searchInput.setTextColor(textColor);
            searchInput.setHintTextColor((textColor & 0x00FFFFFF) | 0x88000000);
            searchInput.setTextSize(TypedValue.COMPLEX_UNIT_DIP, font);
            searchInput.setPadding(dp(activity, 8), 0, dp(activity, 8), 0);
            searchInput.setBackground(controlBackground(activity, color, false, accent));
            LinearLayout.LayoutParams inputLayout = new LinearLayout.LayoutParams(0, control, 1);
            inputLayout.leftMargin = gap;
            dock.addView(searchInput, inputLayout);
            searchCount = new TextView(activity);
            searchCount.setTextColor(textColor);
            searchCount.setTextSize(TypedValue.COMPLEX_UNIT_DIP, font);
            searchCount.setGravity(Gravity.CENTER);
            LinearLayout.LayoutParams countLayout = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, control);
            countLayout.leftMargin = gap;
            countLayout.rightMargin = gap;
            dock.addView(searchCount, countLayout);
            previous = searchStep(activity, "▲", font, color, textColor);
            next = searchStep(activity, "▼", font, color, textColor);
            dock.addView(previous, new LinearLayout.LayoutParams(control, control));
            LinearLayout.LayoutParams nextLayout = new LinearLayout.LayoutParams(control, control);
            nextLayout.leftMargin = gap;
            dock.addView(next, nextLayout);
            previous.setContentDescription(searchLabels.optString("previous", "Previous matching dictionary"));
            next.setContentDescription(searchLabels.optString("next", "Next matching dictionary"));
            dialog = new BackgroundDialogBuilder(activity)
                    .setView(body)
                    .create();
            dialog.setCanceledOnTouchOutside(true);
            close.setOnClickListener(v -> dialog.dismiss());
            dialog.setOnDismissListener(d -> {
                if (live.get(web) == this) {
                    live.remove(web);
                    web.evaluateJavascript("window.wudictPickerClosed&&window.wudictPickerClosed()", null);
                }
            });
            body.setFocusableInTouchMode(true);
            body.requestFocus();
            searchInput.addTextChangedListener(new TextWatcher() {
                @Override public void beforeTextChanged(CharSequence s, int start, int count, int after) { }
                @Override public void onTextChanged(CharSequence s, int start, int before, int count) { }
                @Override public void afterTextChanged(Editable value) {
                    activeMatch = value.length() == 0 ? -1 : 0;
                    rebuildRows(false, "");
                }
            });
            searchInput.setOnEditorActionListener((field, action, event) -> {
                if (action != EditorInfo.IME_ACTION_SEARCH) return false;
                navigate(1);
                return true;
            });
            searchMode.setOnClickListener(v -> {
                filtering = !filtering;
                activeMatch = filtering ? -1 : matchingRows.isEmpty() ? -1 : 0;
                rebuildRows(false, "");
            });
            previous.setOnClickListener(v -> navigate(-1));
            next.setOnClickListener(v -> navigate(1));
            groups.setOnItemSelectedListener(new AdapterView.OnItemSelectedListener() {
                @Override public void onNothingSelected(AdapterView<?> parent) { }
                @Override public void onItemSelected(AdapterView<?> parent, View view, int position, long id) {
                    if (updating) return;
                    try {
                        if (position >= groupValues.length()) return;
                        String chosen = groupValues.getJSONObject(position).getString("id");
                        if (!chosen.equals(groupId)) {
                            groupId = chosen;
                            web.evaluateJavascript("window.wudictPickerGroupChanged("+
                                    JSONObject.quote(chosen)+")", null);
                        }
                    } catch (JSONException ignored) { }
                }
            });
            list.setOnItemClickListener((parent, view, position, id) -> {
                try {
                    if (position >= shownRows.size()) return;
                    int source = shownRows.get(position);
                    if (source < 0 || source >= rows.length()) return;
                    String value = rows.getJSONObject(source).optString("id");
                    if (value.isEmpty()) return;
                    web.evaluateJavascript("window.wudictPickerDictionarySelected("+
                            JSONObject.quote(value)+")", null);
                    dialog.dismiss();
                } catch (JSONException ignored) { }
            });
            update(initial);
        }

        void update(JSONObject data) throws JSONException {
            updating = true;
            JSONArray values = data.getJSONArray("groups");
            groupValues = values;
            groupId = data.optString("group", "all");
            groupAdapter.clear();
            int selected = 0;
            for (int i = 0; i < values.length(); i++) {
                JSONObject value = values.getJSONObject(i);
                groupAdapter.add(value.getString("name"));
                if (groupId.equals(value.getString("id"))) selected = i;
            }
            groupAdapter.notifyDataSetChanged();
            groups.setSelection(selected);
            rows = data.getJSONArray("rows");
            empty = data.optString("empty", "No dictionaries");
            rebuildRows(true, data.optString("selected"));
            updating = false;
        }

        void rebuildRows(boolean keepPosition, String selected) {
            int first = list.getFirstVisiblePosition();
            View top = list.getChildAt(0);
            int offset = top == null ? 0 : top.getTop();
            String query = searchInput.getText().toString().trim();
            matchingRows.clear();
            shownRows.clear();
            for (int i = 0; i < rows.length(); i++) {
                JSONObject row = rows.optJSONObject(i);
                if (row == null) continue;
                boolean match = !query.isEmpty() && matchStart(row.optString("label"), query, 0) >= 0;
                if (match) matchingRows.add(i);
                if (!filtering || query.isEmpty() || match) shownRows.add(i);
            }
            if (query.isEmpty() || matchingRows.isEmpty()) activeMatch = -1;
            else if (activeMatch < 0 && !filtering) activeMatch = 0;
            else if (activeMatch >= matchingRows.size()) activeMatch = matchingRows.size() - 1;
            rowAdapter.clear();
            int checked = -1;
            for (int i = 0; i < shownRows.size(); i++) {
                JSONObject row = rows.optJSONObject(shownRows.get(i));
                if (row == null) continue;
                rowAdapter.add(row.optString("label"));
                if (row.optString("id").equals(selected)) checked = i;
            }
            if (rows.length() == 0) {
                shownRows.add(-1);
                rowAdapter.add(empty);
            }
            rowAdapter.notifyDataSetChanged();
            if (!found) {
                list.clearChoices();
                if (checked >= 0) list.setItemChecked(checked, true);
            }
            if (!filtering && activeMatch >= 0) list.setSelectionFromTop(matchingRows.get(activeMatch), 0);
            else if (keepPosition && top != null && rowAdapter.getCount() > 0)
                list.setSelectionFromTop(Math.min(first, rowAdapter.getCount() - 1), offset);
            else if (rowAdapter.getCount() > 0) list.setSelectionFromTop(0, 0);
            updateSearchControls();
        }

        void navigate(int direction) {
            if (filtering || matchingRows.isEmpty()) return;
            activeMatch = (activeMatch + direction + matchingRows.size()) % matchingRows.size();
            rowAdapter.notifyDataSetChanged();
            list.setSelectionFromTop(matchingRows.get(activeMatch), 0);
            updateSearchControls();
        }

        void updateSearchControls() {
            String hint = searchLabels.optString(filtering ? "filterPlaceholder" : "placeholder",
                    filtering ? "Filter dictionaries…" : "Find dictionaries…");
            searchInput.setHint(hint);
            searchInput.setContentDescription(hint);
            int total = filtering ? shownRows.size() : matchingRows.size();
            if (rows.length() == 0) total = 0;
            int current = filtering ? total : activeMatch < 0 ? 0 : activeMatch + 1;
            searchCount.setText(current + "/" + total);
            searchCount.setContentDescription(searchLabels.optString("count", "{current} of {total}")
                    .replace("{current}", Integer.toString(current))
                    .replace("{total}", Integer.toString(total)));
            previous.setEnabled(!filtering && matchingRows.size() > 1);
            next.setEnabled(!filtering && matchingRows.size() > 1);
            searchMode.setBackground(controlBackground(activity,
                    filtering ? accent : backgroundColor, filtering, accent));
            searchMode.setImageDrawable(new SearchModeIcon(filtering ? backgroundColor : textColor,
                    searchMode.getDrawable().getIntrinsicWidth(), filtering));
            searchMode.setContentDescription(searchLabels.optString(filtering ? "search" : "filter",
                    filtering ? "Search dictionary list" : "Filter matching dictionaries"));
        }
    }

    private static int dp(Activity activity, float value) {
        return Math.max(1, Math.round(value * activity.getResources().getDisplayMetrics().density));
    }

    private static int mix(int base, int accent, int percent) {
        int other = 100 - percent;
        return Color.rgb((Color.red(base) * other + Color.red(accent) * percent) / 100,
                (Color.green(base) * other + Color.green(accent) * percent) / 100,
                (Color.blue(base) * other + Color.blue(accent) * percent) / 100);
    }

    private static GradientDrawable controlBackground(Activity activity, int color,
            boolean selected, int accent) {
        GradientDrawable background = new GradientDrawable();
        background.setColor(color);
        background.setCornerRadius(dp(activity, 6));
        background.setStroke(dp(activity, 1), selected ? accent
                : ShellPrefs.darkIcons(color) ? 0x33000000 : 0x66FFFFFF);
        return background;
    }

    private static Button searchStep(Activity activity, String symbol, float font,
            int color, int textColor) {
        Button button = new Button(activity);
        button.setAllCaps(false);
        button.setText(symbol);
        button.setTextSize(TypedValue.COMPLEX_UNIT_DIP, font);
        button.setTextColor(textColor);
        button.setGravity(Gravity.CENTER);
        button.setPadding(0, 0, 0, 0);
        button.setMinWidth(0);
        button.setMinimumWidth(0);
        button.setMinHeight(0);
        button.setMinimumHeight(0);
        button.setBackground(controlBackground(activity, color, false, textColor));
        return button;
    }

    private static int matchStart(String label, String query, int from) {
        if (query.isEmpty()) return -1;
        for (int i = Math.max(0, from); i <= label.length() - query.length(); i++)
            if (label.regionMatches(true, i, query, 0, query.length())) return i;
        return -1;
    }

    private static final class SearchModeIcon extends Drawable {
        private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final int color, size;
        private final boolean filtering;

        SearchModeIcon(int color, int size) { this(color, size, false); }
        SearchModeIcon(int color, int size, boolean filtering) {
            this.color = color;
            this.size = size;
            this.filtering = filtering;
            paint.setColor(color);
            paint.setStyle(Paint.Style.STROKE);
            paint.setStrokeWidth(2);
            paint.setStrokeCap(Paint.Cap.ROUND);
            paint.setStrokeJoin(Paint.Join.ROUND);
        }
        @Override public int getIntrinsicWidth() { return size; }
        @Override public int getIntrinsicHeight() { return size; }
        @Override public void draw(Canvas canvas) {
            android.graphics.Rect bounds = getBounds();
            int save = canvas.save();
            canvas.translate(bounds.exactCenterX(), bounds.exactCenterY());
            float scale = Math.min(bounds.width(), bounds.height()) / 24f;
            canvas.scale(scale, scale);
            if (filtering) {
                Path funnel = new Path();
                funnel.moveTo(-9, -8); funnel.lineTo(9, -8); funnel.lineTo(2, 0);
                funnel.lineTo(2, 7); funnel.lineTo(-3, 10); funnel.lineTo(-3, 0);
                funnel.close();
                canvas.drawPath(funnel, paint);
            } else {
                canvas.drawCircle(-2, -2, 6, paint);
                canvas.drawLine(2.5f, 2.5f, 9, 9, paint);
            }
            canvas.restoreToCount(save);
        }
        @Override public void setAlpha(int alpha) { paint.setAlpha(alpha); invalidateSelf(); }
        @Override public void setColorFilter(ColorFilter filter) {
            paint.setColorFilter(filter);
            invalidateSelf();
        }
        @Override public int getOpacity() { return PixelFormat.TRANSLUCENT; }
    }

    static void show(Activity activity, String payload, JsPromptResult result) {
        if (activity.isFinishing() || activity.isDestroyed()) { result.cancel(); return; }
        try {
            JSONObject data = new JSONObject(payload);
            boolean found = data.optBoolean("found");
            boolean examples = "stylerPreset".equals(data.optString("kind"));
            boolean plain = found || examples;
            JSONArray rows = data.getJSONArray("rows");
            String[] labels = new String[rows.length()];
            int[] indices = new int[rows.length()];
            boolean[] disabled = new boolean[rows.length()];
            int checked = -1;
            for (int i = 0; i < rows.length(); i++) {
                JSONObject row = rows.getJSONObject(i);
                labels[i] = row.getString("label");
                indices[i] = row.getInt("index");
                disabled[i] = row.optBoolean("disabled") || indices[i] < 0;
                if (indices[i] >= 0 && indices[i] == data.optInt("selected", -1)) checked = i;
            }
            int color = ShellPrefs.pageBg(activity);
            int textColor = ShellPrefs.darkIcons(color) ? 0xDE000000 : 0xFFFFFFFF;
            ArrayAdapter<String> adapter = new ArrayAdapter<String>(activity,
                    plain ? android.R.layout.simple_list_item_1
                            : android.R.layout.simple_list_item_single_choice, labels) {
                @Override public boolean areAllItemsEnabled() { return false; }
                @Override public boolean isEnabled(int position) { return !disabled[position]; }
                @Override public int getViewTypeCount() { return 2; }
                @Override public int getItemViewType(int position) { return indices[position] < 0 ? 1 : 0; }
                @Override public View getView(int position, View recycled, ViewGroup parent) {
                    TextView text;
                    if (indices[position] < 0) {
                        text = recycled instanceof TextView ? (TextView) recycled : new TextView(activity);
                        int pad = (int)(16 * activity.getResources().getDisplayMetrics().density);
                        text.setPadding(pad, pad, pad, pad / 2);
                        text.setTypeface(null, Typeface.BOLD);
                        if (examples) text.setTextSize(18);
                        text.setText(labels[position]);
                    } else {
                        text = (TextView) super.getView(position, recycled, parent);
                        if (examples) {
                            int indent = (int)(32 * activity.getResources().getDisplayMetrics().density);
                            text.setPaddingRelative(indent, text.getPaddingTop(),
                                    text.getPaddingEnd(), text.getPaddingBottom());
                        }
                    }
                    text.setTextColor(textColor);
                    text.setAlpha(disabled[position] && indices[position] >= 0 ? .5f : 1f);
                    return text;
                }
            };
            // Every dismissal must release the pending JavaScript prompt exactly once.
            boolean[] answered = {false};
            AlertDialog.Builder builder = new BackgroundDialogBuilder(activity);
            if (found) {
                TextView title = new TextView(activity);
                title.setText(R.string.found_dictionaries_title);
                title.setTextColor(textColor);
                title.setTextSize(20);
                int pad = (int)(20 * activity.getResources().getDisplayMetrics().density);
                title.setPadding(pad, pad, pad, pad / 2);
                builder.setCustomTitle(title);
            }
            android.content.DialogInterface.OnClickListener select = (d, which) -> {
                        if (disabled[which]) return;
                        answered[0] = true;
                        result.confirm(Integer.toString(indices[which]));
                        d.dismiss();
                    };
            if (rows.length() == 0) builder.setMessage(R.string.found_dictionaries_empty);
            else if (plain) builder.setAdapter(adapter, select);
            else builder.setSingleChoiceItems(adapter, checked, select);
            AlertDialog dialog = builder.setNegativeButton(android.R.string.cancel, (d, which) -> {})
                    .create();
            dialog.setOnDismissListener(d -> {
                if (!answered[0]) { answered[0] = true; result.cancel(); }
            });
            dialog.setCanceledOnTouchOutside(true);
            dialog.show();
            dialog.getWindow().setBackgroundDrawable(WindowBackground.dialogDrawable(activity, color));
            if (examples || "mode".equals(data.optString("kind"))) {
                android.util.DisplayMetrics metrics = activity.getResources().getDisplayMetrics();
                int width = Math.min((int)(260 * metrics.density), (int)(metrics.widthPixels * .9f));
                dialog.getWindow().setLayout(width, android.view.WindowManager.LayoutParams.WRAP_CONTENT);
            }
            if (dialog.getListView() != null) dialog.getListView().setBackgroundColor(android.graphics.Color.TRANSPARENT);
            TextView message = dialog.findViewById(android.R.id.message);
            if (message != null) message.setTextColor(textColor);
            dialog.getButton(AlertDialog.BUTTON_NEGATIVE).setTextColor(textColor);
        } catch (JSONException | IllegalArgumentException bad) {
            result.cancel();
        }
    }
}
