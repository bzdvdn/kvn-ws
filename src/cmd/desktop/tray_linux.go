//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include "tray_linux.h"

extern void goTrayShowCB(void);
extern void goTrayHideCB(void);
extern void goTrayQuitCB(void);
*/
import "C"

import (
	"errors"
	"time"
	"unsafe"
)

// @sk-task desktop-tray#T2.2: linux tray via GtkStatusIcon (AC-001, AC-002, AC-003)
type linuxTray struct {
	icon     *C.GtkStatusIcon
	menu     *C.GtkWidget
	actionCh chan TrayAction
	stopCh   chan struct{}
	initialized bool
}

var linuxTrayInst *linuxTray

//export goTrayShowCB
func goTrayShowCB() {
	if linuxTrayInst != nil {
		linuxTrayInst.actionCh <- TrayShow
	}
}

//export goTrayHideCB
func goTrayHideCB() {
	if linuxTrayInst != nil {
		linuxTrayInst.actionCh <- TrayHide
	}
}

//export goTrayQuitCB
func goTrayQuitCB() {
	if linuxTrayInst != nil {
		linuxTrayInst.actionCh <- TrayQuit
	}
}

func newPlatformTray() TrayManager {
	return &linuxTray{
		actionCh: make(chan TrayAction, 4),
		stopCh:   make(chan struct{}),
	}
}

func (t *linuxTray) ActionCh() <-chan TrayAction { return t.actionCh }

func (t *linuxTray) SetStatus(connected bool) {}

func (t *linuxTray) Show() {
	t.actionCh <- TrayShow
}

func (t *linuxTray) Hide() {
	t.actionCh <- TrayHide
}

func (t *linuxTray) Stop() {
	close(t.stopCh)
}

// initTray creates GTK widgets for the tray — MUST be called on the GTK thread (main goroutine).
// @sk-task desktop-tray#T2.2: linux tray GTK widget creation (AC-001, AC-002, AC-003)
func (t *linuxTray) initTray() error {
	linuxTrayInst = t

	iconData, err := readIcon("connected.png")
	if err != nil {
		return err
	}

	cData := C.CBytes(iconData)
	pb := C.loadPixbufFromData(cData, C.gsize(len(iconData)), nil)
	C.free(cData)
	if pb == nil {
		return errors.New("loadPixbufFromData failed")
	}

	t.icon = C.createStatusIcon()
	C.setIconFromPixbuf(t.icon, pb)
	C.g_object_unref(C.gpointer(pb))
	C.setTooltipText(t.icon, C.CString("KVN Desktop"))
	C.setVisible(t.icon, 1)

	menu := C.createMenu()

	showItem := C.createMenuItem(C.CString("Show"))
	C.connectSignal(showItem, C.CString("activate"), C.getActivateCB(), unsafe.Pointer(C.goTrayShowCB))
	C.menuAppend(menu, showItem)
	C.showWidget(showItem)

	hideItem := C.createMenuItem(C.CString("Hide"))
	C.connectSignal(hideItem, C.CString("activate"), C.getActivateCB(), unsafe.Pointer(C.goTrayHideCB))
	C.menuAppend(menu, hideItem)
	C.showWidget(hideItem)

	sep := C.createSeparator()
	C.menuAppend(menu, sep)
	C.showWidget(sep)

	quitItem := C.createMenuItem(C.CString("Quit"))
	C.connectSignal(quitItem, C.CString("activate"), C.getActivateCB(), unsafe.Pointer(C.goTrayQuitCB))
	C.menuAppend(menu, quitItem)
	C.showWidget(quitItem)

	C.connectIconSignal(t.icon, C.CString("popup-menu"), C.getMenuPopupCB(), unsafe.Pointer(C.goTrayShowCB))

	C.showWidget(menu)
	t.menu = menu
	t.initialized = true
	return nil
}

// destroyTray cleans up GTK widgets — MUST be called on the GTK thread.
func (t *linuxTray) destroyTray() {
	if !t.initialized {
		return
	}
	if t.icon != nil {
		C.setVisible(t.icon, 0)
		C.g_object_unref(C.gpointer(t.icon))
		t.icon = nil
	}
	t.initialized = false
	linuxTrayInst = nil
}

// pollGTK processes pending GTK events — MUST be called on the GTK thread.
// Returns false if Quit was signaled via stopCh.
func (t *linuxTray) pollGTK() bool {
	for C.gtk_events_pending() != 0 {
		C.gtk_main_iteration_do(0)
	}
	select {
	case <-t.stopCh:
		return false
	default:
		return true
	}
}

// Run blocks until Stop() is called. No GTK calls — safe for background goroutine.
func (t *linuxTray) Run() {
	<-t.stopCh
}

// WaitForAction blocks until a tray action arrives, processing GTK events in the meantime.
// Must be called on the GTK thread (main goroutine).
func (t *linuxTray) WaitForAction() TrayAction {
	for {
		if !t.pollGTK() {
			return TrayQuit
		}
		select {
		case action := <-t.actionCh:
			return action
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
