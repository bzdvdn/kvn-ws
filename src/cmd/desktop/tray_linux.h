#ifndef TRAY_LINUX_H
#define TRAY_LINUX_H

#include <gtk/gtk.h>

static void tray_activate_cb(GtkWidget *widget, gpointer data) {
    void (*fn)(void) = (void (*)(void))data;
    fn();
}

static void tray_popup_menu_cb(GtkStatusIcon *icon, guint button, guint32 activate_time, gpointer data) {
    void (*fn)(void) = (void (*)(void))data;
    fn();
}

static inline GCallback getMenuPopupCB(void) {
    return G_CALLBACK(tray_popup_menu_cb);
}

static inline GCallback getActivateCB(void) {
    return G_CALLBACK(tray_activate_cb);
}

static inline GtkStatusIcon* createStatusIcon(void) {
    return gtk_status_icon_new();
}

static inline void setIconFromPixbuf(GtkStatusIcon *si, GdkPixbuf *pb) {
    gtk_status_icon_set_from_pixbuf(si, pb);
}

static inline void setTooltipText(GtkStatusIcon *si, const char *text) {
    gtk_status_icon_set_tooltip_text(si, text);
}

static inline void setVisible(GtkStatusIcon *si, int visible) {
    gtk_status_icon_set_visible(si, visible);
}

static inline GtkWidget* createMenuItem(const char *label) {
    return gtk_menu_item_new_with_label(label);
}

static inline void menuAppend(GtkWidget *menu, GtkWidget *item) {
    gtk_menu_shell_append(GTK_MENU_SHELL(menu), item);
}

static inline GtkWidget* createMenu(void) {
    return gtk_menu_new();
}

static inline void showWidget(GtkWidget *w) {
    gtk_widget_show(w);
}

static inline GtkWidget* createSeparator(void) {
    return gtk_separator_menu_item_new();
}

static inline void connectSignal(GtkWidget *widget, const char *signal, GCallback cb, void *data) {
    g_signal_connect_data(widget, signal, cb, data, NULL, 0);
}

static inline void connectIconSignal(GtkStatusIcon *icon, const char *signal, GCallback cb, void *data) {
    g_signal_connect_data(icon, signal, cb, data, NULL, 0);
}

static inline GdkPixbuf* loadPixbufFromData(const void *data, gsize len, GError **err) {
    GInputStream *stream = g_memory_input_stream_new_from_data(data, len, NULL);
    GdkPixbuf *pb = gdk_pixbuf_new_from_stream(stream, NULL, err);
    g_object_unref(stream);
    return pb;
}

#endif /* TRAY_LINUX_H */
