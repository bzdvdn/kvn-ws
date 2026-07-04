//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>

extern void goTrayShow(void);
extern void goTrayHide(void);
extern void goTrayQuit(void);

@interface KVNStatusItemController : NSObject {
    NSStatusItem *statusItem;
    BOOL isConnected;
}
- (instancetype)initWithImageData:(NSData *)data;
- (void)setConnected:(BOOL)connected;
- (void)cleanup;
@end

@implementation KVNStatusItemController

- (instancetype)initWithImageData:(NSData *)data {
    self = [super init];
    if (self) {
        isConnected = YES;
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        NSImage *icon = [[NSImage alloc] initWithData:data];
        [icon setTemplate:YES];
        statusItem.button.image = icon;
        [icon release];
        NSMenu *menu = [[NSMenu alloc] init];
        NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"Show" action:@selector(onShow:) keyEquivalent:@""];
        showItem.target = self; [menu addItem:showItem]; [showItem release];
        NSMenuItem *hideItem = [[NSMenuItem alloc] initWithTitle:@"Hide" action:@selector(onHide:) keyEquivalent:@""];
        hideItem.target = self; [menu addItem:hideItem]; [hideItem release];
        [menu addItem:[NSMenuItem separatorItem]];
        NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Quit" action:@selector(onQuit:) keyEquivalent:@"q"];
        quitItem.target = self; [menu addItem:quitItem]; [quitItem release];
        statusItem.menu = menu;
        [menu release];
    }
    return self;
}

- (void)onShow:(id)sender { goTrayShow(); }
- (void)onHide:(id)sender { goTrayHide(); }
- (void)onQuit:(id)sender { goTrayQuit(); }
- (void)setConnected:(BOOL)connected { isConnected = connected; }
- (void)cleanup { [[NSStatusBar systemStatusBar] removeStatusItem:statusItem]; statusItem = nil; }
- (void)dealloc { [self cleanup]; [super dealloc]; }

@end

void* createTray(const void *pngData, int pngLen) {
    NSData *data = [NSData dataWithBytes:pngData length:pngLen];
    KVNStatusItemController *ctrl = [[KVNStatusItemController alloc] initWithImageData:data];
    return (void*)ctrl;
}

void setTrayStatus(void *tray, int connected) {
    if (!tray) return;
    KVNStatusItemController *ctrl = (KVNStatusItemController*)tray;
    [ctrl setConnected:(BOOL)connected];
}

void destroyTray(void *tray) {
    if (!tray) return;
    KVNStatusItemController *ctrl = (KVNStatusItemController*)tray;
    [ctrl cleanup];
    [ctrl release];
}
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
