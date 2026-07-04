package main

// @sk-task desktop-tray#T1.1: tray manager interface (AC-001, AC-002, AC-003)
type TrayAction int

const (
	TrayShow TrayAction = iota
	TrayHide
	TrayQuit
)

// @sk-task desktop-tray#T1.1: tray manager interface (AC-001, AC-002, AC-003)
type TrayManager interface {
	Run()
	Stop()
	Show()
	Hide()
	SetStatus(connected bool)
	ActionCh() <-chan TrayAction
}

// @sk-task desktop-tray#T1.1: tray manager interface (AC-001, AC-002, AC-003)
type noopTray struct { //nolint:unused // used on darwin/windows via build tags
	ch chan TrayAction
}

func newNoopTray() *noopTray { //nolint:unused // used on darwin/windows via build tags
	return &noopTray{ch: make(chan TrayAction, 1)}
}

//nolint:unused // used on darwin/windows via build tags
func (t *noopTray) Run() {}

//nolint:unused // used on darwin/windows via build tags
func (t *noopTray) Stop() { close(t.ch) }

//nolint:unused // used on darwin/windows via build tags
func (t *noopTray) Show() {}

//nolint:unused // used on darwin/windows via build tags
func (t *noopTray) Hide() {}

//nolint:unused // used on darwin/windows via build tags
func (t *noopTray) SetStatus(bool) {}

//nolint:unused // used on darwin/windows via build tags
func (t *noopTray) ActionCh() <-chan TrayAction { return t.ch }
