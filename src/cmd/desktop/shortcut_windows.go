//go:build windows

package main

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// @sk-task desktop-tray#T3.2: windows .lnk shortcut registration (AC-005)
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

func createShortcut(target, path string) error {
	// CLSID_ShellLink
	clsid := windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	// IID_IShellLinkW
	iid := windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	var shellLink *IShellLinkW
	if err := windows.CoCreateInstance(&clsid, nil, windows.CLSCTX_INPROC_SERVER, &iid, unsafe.Pointer(&shellLink)); err != nil {
		return err
	}
	defer shellLink.Release()

	shellLink.SetPath(target)

	var persistFile *IPersistFile
	if err := shellLink.QueryInterface(&iidPersistFile, unsafe.Pointer(&persistFile)); err != nil {
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
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetPath        uintptr
	SetPath        uintptr
	GetIDList      uintptr
	SetIDList      uintptr
	GetDescription uintptr
	SetDescription uintptr
	GetWorkingDirectory uintptr
	SetWorkingDirectory uintptr
	GetArguments    uintptr
	SetArguments    uintptr
	GetHotkey      uintptr
	SetHotkey      uintptr
	GetShowCmd     uintptr
	SetShowCmd     uintptr
	GetIconLocation uintptr
	SetIconLocation uintptr
}

func (s *IShellLinkW) SetPath(path string) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	ret, _, _ := windows.SyscallN(s.lpVtbl.SetPath, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(pathPtr)))
	if ret != 0 {
		return windows.Errno(ret)
	}
	return nil
}

func (s *IShellLinkW) QueryInterface(iid *windows.GUID, out unsafe.Pointer) error {
	ret, _, _ := windows.SyscallN(s.lpVtbl.QueryInterface, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(iid)), uintptr(out))
	if ret != 0 {
		return windows.Errno(ret)
	}
	return nil
}

func (s *IShellLinkW) Release() {
	windows.SyscallN(s.lpVtbl.Release, uintptr(unsafe.Pointer(s)))
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
	ret, _, _ := windows.SyscallN(p.lpVtbl.Save, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(path)), uintptr(rememberVal))
	if ret != 0 {
		return windows.Errno(ret)
	}
	return nil
}

func (p *IPersistFile) Release() {
	windows.SyscallN(p.lpVtbl.Release, uintptr(unsafe.Pointer(p)))
}

var iidPersistFile = windows.GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
