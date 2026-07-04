//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>

void* createTray(const char *pngData, int pngLen);
void setTrayStatus(void *tray, int connected);
void destroyTray(void *tray);
extern void goTrayShow(void);
extern void goTrayHide(void);
extern void goTrayQuit(void);
*/
import "C"
import (
	"unsafe"
)

// @sk-task desktop-tray#T2.3: macOS tray via NSStatusBar (AC-001, AC-002, AC-003)
type darwinTray struct {
	ptr      unsafe.Pointer
	actionCh chan TrayAction
	stopCh   chan struct{}
}

var darwinTrayInstance *darwinTray

//export goTrayShow
func goTrayShow() {
	if darwinTrayInstance != nil {
		darwinTrayInstance.actionCh <- TrayShow
	}
}

//export goTrayHide
func goTrayHide() {
	if darwinTrayInstance != nil {
		darwinTrayInstance.actionCh <- TrayHide
	}
}

//export goTrayQuit
func goTrayQuit() {
	if darwinTrayInstance != nil {
		darwinTrayInstance.actionCh <- TrayQuit
	}
}

func newPlatformTray() TrayManager {
	return &darwinTray{
		actionCh: make(chan TrayAction, 4),
		stopCh:   make(chan struct{}),
	}
}

func (t *darwinTray) ActionCh() <-chan TrayAction { return t.actionCh }

func (t *darwinTray) Show() {
	t.actionCh <- TrayShow
}

func (t *darwinTray) Hide() {
	t.actionCh <- TrayHide
}

func (t *darwinTray) SetStatus(connected bool) {
	if t.ptr != nil {
		C.setTrayStatus(t.ptr, boolToCInt(connected))
	}
}

func (t *darwinTray) Stop() {
	close(t.stopCh)
}

func (t *darwinTray) Run() {
	darwinTrayInstance = t

	iconData, err := readIcon("connected.png")
	if err != nil {
		t.actionCh <- TrayQuit
		return
	}

	ptr := C.createTray(unsafe.Pointer(&iconData[0]), C.int(len(iconData)))
	if ptr == nil {
		t.actionCh <- TrayQuit
		return
	}
	t.ptr = ptr

	<-t.stopCh

	C.destroyTray(t.ptr)
	t.ptr = nil
}

func boolToCInt(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
