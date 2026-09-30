#include <gtk/gtk.h>
#include <stdlib.h>
#include <string.h>
#include "_cgo_export.h"

#define WCOUNT 400
static GtkWidget *widgets[WCOUNT];
static GtkWidget *window;
static GtkWidget *stack;
static GtkWidget *progress;
static int loop_running;
static int allow_quit;

static void ep_clicked(GtkWidget *widget, gpointer data) {
    (void)widget;
    easyPackageGoAction(GPOINTER_TO_INT(data));
}

static void ep_changed(GtkWidget *widget, gpointer data) {
    (void)widget;
    easyPackageGoAction(GPOINTER_TO_INT(data));
}

static gboolean ep_tick(gpointer data) {
    (void)data;
    easyPackageGoRefresh();
    return G_SOURCE_CONTINUE;
}

static void ep_destroy(GtkWidget *widget, gpointer data) {
    (void)widget;
    (void)data;
    if (loop_running) gtk_main_quit();
}

static gboolean ep_delete(GtkWidget *widget, GdkEvent *event, gpointer data) {
    (void)widget;
    (void)event;
    (void)data;
    if (allow_quit) return FALSE;
    easyPackageGoAction(11);
    return TRUE;
}

static void remember_widget(int id, GtkWidget *widget) {
    if (id > 0 && id < WCOUNT) widgets[id] = widget;
}

static GtkWidget *make_button(GtkWidget *box, int id, const char *text) {
    GtkWidget *button = gtk_button_new_with_label(text);
    remember_widget(id, button);
    gtk_box_pack_start(GTK_BOX(box), button, FALSE, FALSE, 0);
    g_signal_connect(button, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(id));
    return button;
}

static GtkWidget *make_entry(int id, gboolean secret) {
    GtkWidget *entry = gtk_entry_new();
    remember_widget(id, entry);
    gtk_entry_set_visibility(GTK_ENTRY(entry), !secret);
    gtk_entry_set_activates_default(GTK_ENTRY(entry), FALSE);
    gtk_widget_set_hexpand(entry, TRUE);
    return entry;
}

static GtkWidget *make_combo(int id) {
    GtkWidget *combo = gtk_combo_box_text_new();
    remember_widget(id, combo);
    gtk_widget_set_hexpand(combo, TRUE);
    g_signal_connect(combo, "changed", G_CALLBACK(ep_changed), GINT_TO_POINTER(id));
    return combo;
}

static void set_margins(GtkWidget *widget, int horizontal, int vertical) {
    gtk_widget_set_margin_start(widget, horizontal);
    gtk_widget_set_margin_end(widget, horizontal);
    gtk_widget_set_margin_top(widget, vertical);
    gtk_widget_set_margin_bottom(widget, vertical);
}

static GtkWidget *form_grid(void) {
    GtkWidget *grid = gtk_grid_new();
    gtk_grid_set_row_spacing(GTK_GRID(grid), 14);
    gtk_grid_set_column_spacing(GTK_GRID(grid), 12);
    set_margins(grid, 30, 24);
    gtk_widget_set_hexpand(grid, TRUE);
    gtk_widget_set_valign(grid, GTK_ALIGN_START);
    return grid;
}

static GtkWidget *form_label(const char *text) {
    GtkWidget *label = gtk_label_new(text);
    gtk_widget_set_halign(label, GTK_ALIGN_START);
    gtk_widget_set_valign(label, GTK_ALIGN_CENTER);
    return label;
}

static void add_section(GtkGrid *grid, const char *text, int row) {
    GtkWidget *label = gtk_label_new(text);
    gtk_widget_set_name(label, "section-title");
    gtk_widget_set_halign(label, GTK_ALIGN_START);
    gtk_widget_set_margin_top(label, 8);
    gtk_grid_attach(grid, label, 0, row, 3, 1);
}

static void add_entry_row(GtkGrid *grid, int id, const char *label_text, int button_id,
                          const char *button_text, int row, gboolean secret) {
    GtkWidget *label = form_label(label_text);
    GtkWidget *entry = make_entry(id, secret);
    gtk_grid_attach(grid, label, 0, row, 1, 1);
    gtk_grid_attach(grid, entry, 1, row, 1, 1);
    if (button_id > 0) {
        GtkWidget *button = gtk_button_new_with_label(button_text);
        remember_widget(button_id, button);
        gtk_grid_attach(grid, button, 2, row, 1, 1);
        g_signal_connect(button, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(button_id));
    }
}

static void add_combo_row(GtkGrid *grid, int id, const char *label_text, int row) {
    gtk_grid_attach(grid, form_label(label_text), 0, row, 1, 1);
    gtk_grid_attach(grid, make_combo(id), 1, row, 2, 1);
}

static GtkWidget *create_pack_page(void) {
    GtkWidget *scroller = gtk_scrolled_window_new(NULL, NULL);
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scroller), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    GtkWidget *grid = form_grid();
    add_section(GTK_GRID(grid), "01   选择要打包的项目", 0);
    add_entry_row(GTK_GRID(grid), 100, "项目文件夹", 101, "浏览…", 1, FALSE);
    add_combo_row(GTK_GRID(grid), 102, "常用 / 最近目录", 2);

    add_section(GTK_GRID(grid), "02   设置过滤规则", 3);
    GtkWidget *checks = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 16);
    GtkWidget *class_filter = gtk_check_button_new_with_label("自动过滤 .class 编译文件");
    GtkWidget *large_filter = gtk_check_button_new_with_label("过滤超过此大小的文件");
    remember_widget(103, class_filter);
    remember_widget(104, large_filter);
    gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(class_filter), TRUE);
    g_signal_connect(class_filter, "toggled", G_CALLBACK(ep_changed), GINT_TO_POINTER(103));
    g_signal_connect(large_filter, "toggled", G_CALLBACK(ep_changed), GINT_TO_POINTER(104));
    gtk_box_pack_start(GTK_BOX(checks), class_filter, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(checks), large_filter, FALSE, FALSE, 0);
    gtk_grid_attach(GTK_GRID(grid), checks, 0, 4, 3, 1);
    GtkWidget *limit_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    GtkWidget *limit = make_entry(105, FALSE);
    gtk_entry_set_text(GTK_ENTRY(limit), "100");
    gtk_entry_set_width_chars(GTK_ENTRY(limit), 10);
    GtkWidget *unit = make_combo(106);
    gtk_widget_set_size_request(unit, 100, -1);
    gtk_box_pack_start(GTK_BOX(limit_box), limit, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(limit_box), unit, FALSE, FALSE, 0);
    gtk_grid_attach(GTK_GRID(grid), form_label("文件大小阈值"), 0, 5, 1, 1);
    gtk_grid_attach(GTK_GRID(grid), limit_box, 1, 5, 2, 1);
    GtkWidget *excluded = gtk_label_new("默认排除 .git、.idea、.codegraph、.vscode、.vs、.svn、.hg、.fleet 及其内部文件");
    gtk_widget_set_halign(excluded, GTK_ALIGN_START);
    gtk_style_context_add_class(gtk_widget_get_style_context(excluded), "dim-label");
    gtk_grid_attach(GTK_GRID(grid), excluded, 0, 6, 3, 1);

    add_section(GTK_GRID(grid), "03   保存与安全", 7);
    add_entry_row(GTK_GRID(grid), 107, "输出 ZIP / JPG 文件", 108, "另存为…", 8, FALSE);
    GtkWidget *cover = gtk_check_button_new_with_label("附加 JPG 封面");
    remember_widget(109, cover);
    gtk_grid_attach(GTK_GRID(grid), cover, 0, 9, 3, 1);
    g_signal_connect(cover, "toggled", G_CALLBACK(ep_changed), GINT_TO_POINTER(109));
    add_entry_row(GTK_GRID(grid), 110, "封面图片", 111, "选择 JPG…", 10, FALSE);

    GtkWidget *password = gtk_check_button_new_with_label("启用 AES-256 压缩密码");
    remember_widget(112, password);
    gtk_grid_attach(GTK_GRID(grid), password, 0, 11, 3, 1);
    g_signal_connect(password, "toggled", G_CALLBACK(ep_changed), GINT_TO_POINTER(112));
    GtkWidget *password_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    GtkWidget *password_entry = make_entry(113, TRUE);
    GtkWidget *show = gtk_button_new_with_label("显示");
    GtkWidget *generate = gtk_button_new_with_label("生成密码");
    remember_widget(114, show);
    remember_widget(115, generate);
    gtk_box_pack_start(GTK_BOX(password_box), password_entry, TRUE, TRUE, 0);
    gtk_box_pack_start(GTK_BOX(password_box), show, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(password_box), generate, FALSE, FALSE, 0);
    gtk_grid_attach(GTK_GRID(grid), password_box, 0, 12, 3, 1);
    g_signal_connect(show, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(114));
    g_signal_connect(generate, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(115));

    GtkWidget *actions = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    GtkWidget *preview = gtk_button_new_with_label("预览文件");
    GtkWidget *start = gtk_button_new_with_label("开始打包");
    remember_widget(116, preview);
    remember_widget(117, start);
    gtk_box_pack_start(GTK_BOX(actions), preview, FALSE, FALSE, 0);
    gtk_box_pack_end(GTK_BOX(actions), start, FALSE, FALSE, 0);
    gtk_grid_attach(GTK_GRID(grid), actions, 0, 13, 3, 1);
    g_signal_connect(preview, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(116));
    g_signal_connect(start, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(117));
    gtk_container_add(GTK_CONTAINER(scroller), grid);
    return scroller;
}

static GtkWidget *create_restore_page(void) {
    GtkWidget *scroller = gtk_scrolled_window_new(NULL, NULL);
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scroller), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    GtkWidget *grid = form_grid();
    add_section(GTK_GRID(grid), "01   选择要还原的 JPG 或 ZIP", 0);
    GtkWidget *hint = gtk_label_new("支持普通 ZIP、AES ZIP 及 EasyPackage JPG。");
    gtk_widget_set_halign(hint, GTK_ALIGN_START);
    gtk_grid_attach(GTK_GRID(grid), hint, 0, 1, 3, 1);
    add_entry_row(GTK_GRID(grid), 200, "压缩文件", 201, "选择文件…", 2, FALSE);
    add_section(GTK_GRID(grid), "02   选择解压位置", 3);
    add_entry_row(GTK_GRID(grid), 202, "目标父文件夹", 203, "浏览…", 4, FALSE);
    add_combo_row(GTK_GRID(grid), 210, "常用 / 最近目录", 5);
    gtk_grid_attach(GTK_GRID(grid), form_label("新文件夹名称"), 0, 6, 1, 1);
    gtk_grid_attach(GTK_GRID(grid), make_entry(204, FALSE), 1, 6, 2, 1);
    add_section(GTK_GRID(grid), "03   解压密码（未加密文件留空）", 7);
    gtk_grid_attach(GTK_GRID(grid), make_entry(205, TRUE), 0, 8, 2, 1);
    GtkWidget *show = gtk_button_new_with_label("显示");
    remember_widget(206, show);
    gtk_grid_attach(GTK_GRID(grid), show, 2, 8, 1, 1);
    g_signal_connect(show, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(206));
    GtkWidget *start = gtk_button_new_with_label("直接解压到新文件夹");
    remember_widget(207, start);
    gtk_widget_set_halign(start, GTK_ALIGN_END);
    gtk_grid_attach(GTK_GRID(grid), start, 1, 9, 2, 1);
    g_signal_connect(start, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(207));
    gtk_container_add(GTK_CONTAINER(scroller), grid);
    return scroller;
}

void ep_run(void) {
    gtk_init(NULL, NULL);
    GtkCssProvider *css = gtk_css_provider_new();
    gtk_css_provider_load_from_data(css,
        "window { background: #f4f6fa; }"
        "#header { background: #ffffff; border-bottom: 1px solid #e3e7ef; }"
        "#sidebar { background: #edf1f7; }"
        "#section-title { color: #243758; font-size: 15px; font-weight: 700; }"
        ".dim-label { color: #69768a; }"
        "button { padding: 7px 13px; }"
        "entry, combobox { min-height: 29px; }", -1, NULL);
    gtk_style_context_add_provider_for_screen(gdk_screen_get_default(), GTK_STYLE_PROVIDER(css), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

    window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "EasyPackage · 轻松打包");
    GdkMonitor *monitor = gdk_display_get_primary_monitor(gdk_display_get_default());
    GdkRectangle workarea = {0, 0, 1160, 860};
    if (monitor) gdk_monitor_get_workarea(monitor, &workarea);
    gtk_window_set_default_size(GTK_WINDOW(window), MIN(1160, MAX(820, workarea.width - 80)), MIN(860, MAX(600, workarea.height - 80)));
    gtk_window_set_position(GTK_WINDOW(window), GTK_WIN_POS_CENTER);
    gtk_window_set_resizable(GTK_WINDOW(window), TRUE);
    g_signal_connect(window, "destroy", G_CALLBACK(ep_destroy), NULL);
    g_signal_connect(window, "delete-event", G_CALLBACK(ep_delete), NULL);

    GtkWidget *outer = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_container_add(GTK_CONTAINER(window), outer);
    GtkWidget *header = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_set_name(header, "header");
    set_margins(header, 26, 18);
    GtkWidget *title = gtk_label_new(NULL);
    gtk_label_set_markup(GTK_LABEL(title), "<span size='x-large' weight='bold' foreground='#1d2c45'>EasyPackage</span>");
    gtk_widget_set_halign(title, GTK_ALIGN_START);
    GtkWidget *subtitle = gtk_label_new("本机项目打包与还原 · 文件仅在本机处理");
    gtk_widget_set_halign(subtitle, GTK_ALIGN_START);
    gtk_style_context_add_class(gtk_widget_get_style_context(subtitle), "dim-label");
    gtk_box_pack_start(GTK_BOX(header), title, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(header), subtitle, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(outer), header, FALSE, FALSE, 0);

    GtkWidget *body = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_box_pack_start(GTK_BOX(outer), body, TRUE, TRUE, 0);
    GtkWidget *sidebar = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
    gtk_widget_set_name(sidebar, "sidebar");
    gtk_widget_set_size_request(sidebar, 208, -1);
    set_margins(sidebar, 16, 22);
    GtkWidget *pack_nav = make_button(sidebar, 12, "◈   项目打包");
    GtkWidget *restore_nav = make_button(sidebar, 13, "↻   JPG / ZIP 还原");
    gtk_widget_set_halign(pack_nav, GTK_ALIGN_FILL);
    gtk_widget_set_halign(restore_nav, GTK_ALIGN_FILL);
    GtkWidget *spacer = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_box_pack_start(GTK_BOX(sidebar), spacer, TRUE, TRUE, 0);
    make_button(sidebar, 11, "退出");
    gtk_box_pack_start(GTK_BOX(body), sidebar, FALSE, FALSE, 0);

    stack = gtk_stack_new();
    gtk_stack_set_transition_type(GTK_STACK(stack), GTK_STACK_TRANSITION_TYPE_CROSSFADE);
    gtk_stack_add_named(GTK_STACK(stack), create_pack_page(), "pack");
    gtk_stack_add_named(GTK_STACK(stack), create_restore_page(), "restore");
    gtk_widget_set_hexpand(stack, TRUE);
    gtk_widget_set_vexpand(stack, TRUE);
    gtk_box_pack_start(GTK_BOX(body), stack, TRUE, TRUE, 0);

    GtkWidget *footer = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
    set_margins(footer, 24, 12);
    gtk_widget_set_name(footer, "header");
    GtkWidget *status = gtk_label_new("准备就绪 · 选择项目文件夹开始");
    remember_widget(300, status);
    gtk_widget_set_halign(status, GTK_ALIGN_START);
    gtk_widget_set_hexpand(status, TRUE);
    progress = gtk_progress_bar_new();
    remember_widget(301, progress);
    gtk_widget_set_size_request(progress, 180, -1);
    GtkWidget *cancel = gtk_button_new_with_label("取消");
    remember_widget(208, cancel);
    gtk_widget_set_sensitive(cancel, FALSE);
    g_signal_connect(cancel, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(208));
    GtkWidget *show_output = gtk_button_new_with_label("查看位置");
    remember_widget(209, show_output);
    gtk_widget_set_sensitive(show_output, FALSE);
    g_signal_connect(show_output, "clicked", G_CALLBACK(ep_clicked), GINT_TO_POINTER(209));
    gtk_box_pack_start(GTK_BOX(footer), status, TRUE, TRUE, 0);
    gtk_box_pack_start(GTK_BOX(footer), progress, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(footer), cancel, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(footer), show_output, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(outer), footer, FALSE, FALSE, 0);

    easyPackageGoReady();
    gtk_widget_show_all(window);
    gtk_widget_hide(widgets[208]);
    gtk_widget_set_visible(widgets[210], TRUE);
    loop_running = 1;
    g_timeout_add(180, ep_tick, NULL);
    gtk_main();
    loop_running = 0;
    g_object_unref(css);
}

void ep_set_text(int id, const char *text) {
    if (id <= 0 || id >= WCOUNT || !widgets[id]) return;
    GtkWidget *widget = widgets[id];
    if (GTK_IS_ENTRY(widget)) gtk_entry_set_text(GTK_ENTRY(widget), text ? text : "");
    else if (GTK_IS_LABEL(widget)) gtk_label_set_text(GTK_LABEL(widget), text ? text : "");
    else if (GTK_IS_BUTTON(widget)) gtk_button_set_label(GTK_BUTTON(widget), text ? text : "");
}

const char *ep_get_text(int id) {
    if (id <= 0 || id >= WCOUNT || !widgets[id] || !GTK_IS_ENTRY(widgets[id])) return "";
    return gtk_entry_get_text(GTK_ENTRY(widgets[id]));
}

int ep_checked(int id) {
    if (id <= 0 || id >= WCOUNT || !widgets[id] || !GTK_IS_TOGGLE_BUTTON(widgets[id])) return 0;
    return gtk_toggle_button_get_active(GTK_TOGGLE_BUTTON(widgets[id]));
}

void ep_set_secret(int id, int secret) {
    if (id > 0 && id < WCOUNT && widgets[id] && GTK_IS_ENTRY(widgets[id]))
        gtk_entry_set_visibility(GTK_ENTRY(widgets[id]), !secret);
}

void ep_set_enabled(int id, int enabled) {
    if (id > 0 && id < WCOUNT && widgets[id]) gtk_widget_set_sensitive(widgets[id], enabled != 0);
}

void ep_set_status(const char *text, int percent, int marquee) {
    if (widgets[300]) gtk_label_set_text(GTK_LABEL(widgets[300]), text ? text : "");
    if (marquee) gtk_progress_bar_pulse(GTK_PROGRESS_BAR(progress));
    else gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(progress), percent / 100.0);
}

void ep_set_busy(int busy) {
    if (widgets[208]) gtk_widget_set_visible(widgets[208], busy != 0);
}

void ep_set_page(int page) {
    if (stack) gtk_stack_set_visible_child_name(GTK_STACK(stack), page == 1 ? "restore" : "pack");
}

void ep_set_options(int id, char **items, int count, int active) {
    if (id <= 0 || id >= WCOUNT || !widgets[id] || !GTK_IS_COMBO_BOX_TEXT(widgets[id])) return;
    GtkComboBoxText *combo = GTK_COMBO_BOX_TEXT(widgets[id]);
    gtk_combo_box_text_remove_all(combo);
    for (int i = 0; i < count; ++i) gtk_combo_box_text_append_text(combo, items[i]);
    gtk_combo_box_set_active(GTK_COMBO_BOX(combo), active);
}

int ep_selected(int id) {
    if (id <= 0 || id >= WCOUNT || !widgets[id] || !GTK_IS_COMBO_BOX(widgets[id])) return -1;
    return gtk_combo_box_get_active(GTK_COMBO_BOX(widgets[id]));
}

char *ep_get_selected_text(int id) {
    if (id <= 0 || id >= WCOUNT || !widgets[id] || !GTK_IS_COMBO_BOX_TEXT(widgets[id])) return NULL;
    gchar *value = gtk_combo_box_text_get_active_text(GTK_COMBO_BOX_TEXT(widgets[id]));
    if (!value) return NULL;
    char *copy = strdup(value);
    g_free(value);
    return copy;
}

int ep_choose(int kind, const char *current, char **selected) {
    GtkFileChooserAction action = GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER;
    const char *title = "选择文件夹";
    const char *accept = "选择";
    if (kind == 2) { action = GTK_FILE_CHOOSER_ACTION_OPEN; title = "选择 JPG 图片"; accept = "选择"; }
    if (kind == 3 || kind == 5) { action = GTK_FILE_CHOOSER_ACTION_SAVE; title = "保存 EasyPackage 压缩包"; accept = "保存"; }
    if (kind == 4) { action = GTK_FILE_CHOOSER_ACTION_OPEN; title = "选择 JPG / ZIP 文件"; accept = "打开"; }
    GtkFileChooserNative *native = gtk_file_chooser_native_new(title, GTK_WINDOW(window), action, accept, "取消");
    GtkFileChooser *chooser = GTK_FILE_CHOOSER(native);
    if (current && *current) {
        if (action == GTK_FILE_CHOOSER_ACTION_SAVE) {
            gtk_file_chooser_set_filename(chooser, current);
        } else if (action == GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER) {
            gtk_file_chooser_set_current_folder(chooser, current);
        } else {
            gtk_file_chooser_set_filename(chooser, current);
        }
    } else if (action == GTK_FILE_CHOOSER_ACTION_SAVE) {
        gtk_file_chooser_set_current_name(chooser, kind == 3 ? "package.zip" : "package.jpg");
    }
    if (kind == 2 || kind == 4) {
        GtkFileFilter *filter = gtk_file_filter_new();
        gtk_file_filter_set_name(filter, kind == 2 ? "JPEG 图片 (*.jpg, *.jpeg)" : "EasyPackage / ZIP (*.jpg, *.jpeg, *.zip)");
        if (kind == 2) {
            gtk_file_filter_add_mime_type(filter, "image/jpeg");
        } else {
            gtk_file_filter_add_pattern(filter, "*.jpg");
            gtk_file_filter_add_pattern(filter, "*.jpeg");
            gtk_file_filter_add_pattern(filter, "*.zip");
        }
        gtk_file_chooser_add_filter(chooser, filter);
    }
    int response = gtk_native_dialog_run(GTK_NATIVE_DIALOG(native));
    int ok = response == GTK_RESPONSE_ACCEPT;
    if (ok) {
        gchar *filename = gtk_file_chooser_get_filename(chooser);
        *selected = filename ? strdup(filename) : NULL;
        g_free(filename);
    }
    g_object_unref(native);
    return ok;
}

int ep_message(const char *title, const char *message, int confirm) {
    GtkWidget *dialog = gtk_message_dialog_new(GTK_WINDOW(window), GTK_DIALOG_MODAL | GTK_DIALOG_DESTROY_WITH_PARENT,
        confirm ? GTK_MESSAGE_QUESTION : GTK_MESSAGE_INFO,
        confirm ? GTK_BUTTONS_YES_NO : GTK_BUTTONS_CLOSE, "%s", message ? message : "");
    gtk_window_set_title(GTK_WINDOW(dialog), title ? title : "EasyPackage");
    int response = gtk_dialog_run(GTK_DIALOG(dialog));
    gtk_widget_destroy(dialog);
    return confirm ? response == GTK_RESPONSE_YES : 1;
}

void ep_quit(void) {
    if (!loop_running) return;
    allow_quit = 1;
    gtk_widget_destroy(window);
}
