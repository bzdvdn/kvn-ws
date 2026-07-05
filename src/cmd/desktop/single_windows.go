//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutexW    = modkernel32.NewProc("CreateMutexW")
	procCloseHandle     = modkernel32.NewProc("CloseHandle")
	procFindWindowW     = modkernel32.NewProc("FindWindowW")
	procSetForegroundWindow = modkernel32.NewProc("SetForegroundWindow")
)

var guardMutexHandle windows.Handle

// @sk-task desktop-tray#T4.2: windows single-instance mutex guard (AC-006)
func guardSingleInstance() bool {
	name, err := windows.UTF16PtrFromString("Global\\KVN-Desktop-2311")
	if err != nil {
		return true
	}

	r1, _, e1 := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	handle := windows.Handle(r1)

	if handle == 0 {
		return true
	}

	if e1 == windows.ERROR_ALREADY_EXISTS {
		procCloseHandle.Call(uintptr(handle))
		focusExistingWindow()
		return false
	}

	guardMutexHandle = handle
	return true
}

func releaseGuard() {
	if guardMutexHandle != 0 {
		procCloseHandle.Call(uintptr(guardMutexHandle))
		guardMutexHandle = 0
	}
}

func focusExistingWindow() {
	className, _ := windows.UTF16PtrFromString("")
	windowTitle, _ := windows.UTF16PtrFromString("KVN Desktop")
	r1, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowTitle)))
	if r1 != 0 {
		procSetForegroundWindow.Call(r1)
	}
}


