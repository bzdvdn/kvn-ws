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
type noopTray struct {
	ch chan TrayAction
}

func newNoopTray() *noopTray {
	return &noopTray{ch: make(chan TrayAction, 1)}
}

func (t *noopTray) Run()                {}
func (t *noopTray) Stop()               { close(t.ch) }
func (t *noopTray) Show()               {}
func (t *noopTray) Hide()               {}
func (t *noopTray) SetStatus(bool)      {}
func (t *noopTray) ActionCh() <-chan TrayAction { return t.ch }
