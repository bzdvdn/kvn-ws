//go:build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance    = modole32.NewProc("CoCreateInstance")
)

var (
	iidShellLink  = &windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	clsidShellLink = &windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPersistFile = &windows.GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

func maybeRegisterShortcut() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	startMenu, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return err
	}

	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return err
	}

	lnkName := "KVN Desktop.lnk"
	paths := []string{
		filepath.Join(startMenu, lnkName),
		filepath.Join(desktop, lnkName),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := createShortcut(exe, p); err != nil {
			return err
		}
	}
	return nil
}

func coCreateInstance(clsid, iid *windows.GUID, ppv unsafe.Pointer) error {
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)),
		0,
		windows.CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(iid)),
		uintptr(ppv),
	)
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

func createShortcut(target, path string) error {
	var shellLink *IShellLinkW
	if err := coCreateInstance(clsidShellLink, iidShellLink, unsafe.Pointer(&shellLink)); err != nil {
		return err
	}
	defer shellLink.Release()

	shellLink.SetPath(target)

	var persistFile *IPersistFile
	if err := shellLink.QueryInterface(iidPersistFile, unsafe.Pointer(&persistFile)); err != nil {
		return err
	}
	defer persistFile.Release()

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return persistFile.Save(pathPtr, true)
}

type IShellLinkW struct {
	lpVtbl *IShellLinkWVtbl
}

type IShellLinkWVtbl struct {
	QueryInterface       uintptr
	AddRef               uintptr
	Release              uintptr
	GetPath              uintptr
	SetPath              uintptr
	GetIDList            uintptr
	SetIDList            uintptr
	GetDescription       uintptr
	SetDescription       uintptr
	GetWorkingDirectory  uintptr
	SetWorkingDirectory  uintptr
	GetArguments         uintptr
	SetArguments         uintptr
	GetHotkey            uintptr
	SetHotkey            uintptr
	GetShowCmd           uintptr
	SetShowCmd           uintptr
	GetIconLocation      uintptr
	SetIconLocation      uintptr
}

func (s *IShellLinkW) SetPath(path string) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	ret, _, _ := syscall.Syscall(s.lpVtbl.SetPath, 2, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(pathPtr)), 0)
	if ret != 0 {
		return windows.Errno(ret)
	}
	return nil
}

func (s *IShellLinkW) QueryInterface(iid *windows.GUID, out unsafe.Pointer) error {
	ret, _, _ := syscall.Syscall(s.lpVtbl.QueryInterface, 3, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(iid)), uintptr(out))
	if ret != 0 {
		return windows.Errno(ret)
	}
	return nil
}

func (s *IShellLinkW) Release() {
	syscall.Syscall(s.lpVtbl.Release, 1, uintptr(unsafe.Pointer(s)), 0, 0)
}

type IPersistFile struct {
	lpVtbl *IPersistFileVtbl
}

type IPersistFileVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetClassID     uintptr
	IsDirty        uintptr
	Load           uintptr
	Save           uintptr
	SaveCompleted  uintptr
	GetCurFile     uintptr
}

func (p *IPersistFile) Save(path *uint16, remember bool) error {
	rememberVal := uint32(0)
	if remember {
		rememberVal = 1
	}
	ret, _, _ := syscall.Syscall(p.lpVtbl.Save, 3, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(path)), uintptr(rememberVal))
	if ret != 0 {
		return windows.Errno(ret)
	}
	return nil
}

func (p *IPersistFile) Release() {
	syscall.Syscall(p.lpVtbl.Release, 1, uintptr(unsafe.Pointer(p)), 0, 0)
}
