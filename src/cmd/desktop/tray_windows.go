//go:build windows

package main

import (
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// @sk-task desktop-tray#T2.1: windows tray via Shell_NotifyIconW (AC-001, AC-002, AC-003)
type windowsTray struct {
	mu       sync.Mutex
	hwnd     windows.Handle
	icon     windows.Handle
	nid      NOTIFYICONDATAW
	actionCh chan TrayAction
	stopCh   chan struct{}
	doneCh   chan struct{}
	connected bool
}

type NOTIFYICONDATAW struct {
	cbSize           uint32
	hWnd             windows.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     windows.Handle
}

const (
	NIM_ADD        = 0
	NIM_MODIFY     = 1
	NIM_DELETE     = 2
	NIM_SETVERSION = 4
	NIF_MESSAGE    = 1
	NIF_ICON       = 2
	NIF_TIP        = 4
	NIF_INFO       = 0x10
	NIF_GUID       = 0x20
	WM_USER        = 0x0400
	WM_TRAYICON    = WM_USER + 100
	WM_COMMAND     = 0x0111
	ID_TRAY_SHOW   = 1001
	ID_TRAY_HIDE   = 1002
	ID_TRAY_QUIT   = 1003
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procFindWindowW         = user32.NewProc("FindWindowW")
)

const (
	SW_HIDE            = 0
	SW_SHOW            = 5
	SWP_NOSIZE         = 0x0001
	SWP_NOMOVE         = 0x0002
	SWP_SHOWWINDOW     = 0x0040
	HWND_TOP           = 0
	IMAGE_ICON         = 1
	LR_LOADFROMFILE    = 0x0010
	LR_DEFAULTSIZE     = 0x0040
	MF_STRING          = 0
	TPM_RIGHTBUTTON    = 2
	TPM_BOTTOMALIGN    = 0x0020
	WS_EX_TOOLWINDOW   = 0x00000080
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	COLOR_WINDOW       = 5
)

func newPlatformTray() TrayManager {
	return &windowsTray{
		actionCh: make(chan TrayAction, 4),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (t *windowsTray) ActionCh() <-chan TrayAction { return t.actionCh }

func (t *windowsTray) Show() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hwnd != 0 {
		procShowWindow.Call(uintptr(t.hwnd), SW_SHOW)
		procSetForegroundWindow.Call(uintptr(t.hwnd))
	}
}

func (t *windowsTray) Hide() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hwnd != 0 {
		procShowWindow.Call(uintptr(t.hwnd), SW_HIDE)
	}
}

func (t *windowsTray) SetStatus(connected bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = connected
}

func (t *windowsTray) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.nid.hWnd != 0 {
		shellNotifyIcon(NIM_DELETE, &t.nid)
	}
	if t.icon != 0 {
		windows.DestroyIcon(t.icon)
	}
	if t.hwnd != 0 {
		procDestroyWindow.Call(uintptr(t.hwnd))
	}
	close(t.stopCh)
}

func (t *windowsTray) Run() {
	const className = "KVN_Tray_Window"
	inst, err := windows.GetModuleHandle("")
	if err != nil {
		t.actionCh <- TrayQuit
		return
	}

	wc := windows.WndClassEx{
		WndProc:    windows.NewCallback(t.wndProc),
		Instance:   inst,
		ClassName:  windows.StringToUTF16Ptr(className),
		Background: COLOR_WINDOW,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))

	if _, err := registerClass(&wc); err != nil {
		t.actionCh <- TrayQuit
		return
	}

	hwnd, err := createWindow(className, "KVN Tray", WS_OVERLAPPEDWINDOW,
		0, 0, 0, 0, 0, 0, inst, nil)
	if err != nil {
		t.actionCh <- TrayQuit
		return
	}
	t.mu.Lock()
	t.hwnd = hwnd
	t.mu.Unlock()

	icoData, err := readIcon("kvn.ico")
	if err != nil {
		t.actionCh <- TrayQuit
		return
	}

	tmpDir := os.TempDir()
	tmpIco := filepath.Join(tmpDir, "kvn-tray.ico")
	if err := os.WriteFile(tmpIco, icoData, 0644); err != nil {
		t.actionCh <- TrayQuit
		return
	}
	defer os.Remove(tmpIco)

	icon, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(tmpIco))),
		IMAGE_ICON, 0, 0, LR_LOADFROMFILE|LR_DEFAULTSIZE,
	)
	if icon == 0 {
		t.actionCh <- TrayQuit
		return
	}
	t.icon = windows.Handle(icon)

	t.nid = NOTIFYICONDATAW{
		cbSize:           uint32(unsafe.Sizeof(t.nid)),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		uCallbackMessage: WM_TRAYICON,
		hIcon:            windows.Handle(icon),
	}
	copy(t.nid.szTip[:], windows.StringToUTF16("KVN Desktop"))

	if !shellNotifyIcon(NIM_ADD, &t.nid) {
		t.actionCh <- TrayQuit
		return
	}

	var msg windows.Msg
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (t *windowsTray) wndProc(hwnd windows.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAYICON:
		switch lParam {
		case 0x0203: // WM_LBUTTONDBLCLK
			t.actionCh <- TrayShow
		case 0x0205: // WM_RBUTTONUP
			t.showContextMenu(hwnd)
		}
	case WM_COMMAND:
		switch windows.LOWORD(uint32(wParam)) {
		case ID_TRAY_SHOW:
			t.actionCh <- TrayShow
		case ID_TRAY_HIDE:
			t.actionCh <- TrayHide
		case ID_TRAY_QUIT:
			t.actionCh <- TrayQuit
		}
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func (t *windowsTray) showContextMenu(hwnd windows.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	procAppendMenuW.Call(menu, MF_STRING, ID_TRAY_SHOW, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Show"))))
	procAppendMenuW.Call(menu, MF_STRING, ID_TRAY_HIDE, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Hide"))))
	procAppendMenuW.Call(menu, MF_STRING, ID_TRAY_QUIT, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Quit"))))

	var p windows.Point
	windows.GetCursorPos(&p)
	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(menu, TPM_RIGHTBUTTON|TPM_BOTTOMALIGN,
		uintptr(p.X), uintptr(p.Y), 0, uintptr(hwnd), 0)
}

func shellNotifyIcon(cmd int, nid *NOTIFYICONDATAW) bool {
	ret, _, _ := procShellNotifyIconW.Call(uintptr(cmd), uintptr(unsafe.Pointer(nid)))
	return ret != 0
}

func registerClass(wc *windows.WndClassEx) (uint16, error) {
	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wc)))
	if ret == 0 {
		return 0, err
	}
	return uint16(ret), nil
}

func createWindow(className, windowName string, style int, x, y, width, height int, parent, menu, instance windows.Handle, param unsafe.Pointer) (windows.Handle, error) {
	ret, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(className))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowName))),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		uintptr(parent), uintptr(menu), uintptr(instance),
		uintptr(param),
	)
	if ret == 0 {
		return 0, err
	}
	return windows.Handle(ret), nil
}
