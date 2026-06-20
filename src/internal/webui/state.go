package webui

import (
	"context"
	"sync"

	"github.com/bzdvdn/kvn-ws/src/internal/bootstrap/client"
)

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusError        Status = "error"
)

type LogEntry struct {
	Line   string `json:"line"`
	Level  string `json:"level"`
	Action int    `json:"action,omitempty"`
	IP     string `json:"ip,omitempty"`
	TS     string `json:"ts,omitempty"`
}

// @sk-task kvn-web#T1.3: app state management (AC-003, AC-004)
type AppState struct {
	mu          sync.Mutex
	cl          *client.Client
	cancel      context.CancelFunc
	doneCh      chan struct{}
	status      Status
	logCh       chan LogEntry
	subCh       chan chan LogEntry
	statusCh    chan Status
	statusSubCh chan chan Status
}

func NewAppState() *AppState {
	return &AppState{
		status:      StatusDisconnected,
		logCh:       make(chan LogEntry, 1000),
		subCh:       make(chan chan LogEntry),
		statusCh:    make(chan Status, 100),
		statusSubCh: make(chan chan Status),
	}
}

func (s *AppState) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *AppState) setStatus(st Status) {
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
	select {
	case s.statusCh <- st:
	default:
	}
}

func (s *AppState) Client() *client.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cl
}

func (s *AppState) setClient(cl *client.Client) {
	s.mu.Lock()
	s.cl = cl
	s.mu.Unlock()
}

func (s *AppState) Cancel() context.CancelFunc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel
}

func (s *AppState) SetCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
}

func (s *AppState) DoneCh() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doneCh
}

func (s *AppState) SetDoneCh(doneCh chan struct{}) {
	s.mu.Lock()
	s.doneCh = doneCh
	s.mu.Unlock()
}

func (s *AppState) PushLog(entry LogEntry) {
	select {
	case s.logCh <- entry:
	default:
	}
}

func (s *AppState) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)
	s.subCh <- ch
	return ch
}

func (s *AppState) Unsubscribe(ch chan LogEntry) {
	s.subCh <- ch
}

func (s *AppState) broadcastLogs(ctx context.Context) {
	subs := map[chan LogEntry]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-s.logCh:
			for ch := range subs {
				select {
				case ch <- entry:
				default:
				}
			}
		case ch := <-s.subCh:
			if _, ok := subs[ch]; ok {
				delete(subs, ch)
				close(ch)
			} else {
				subs[ch] = struct{}{}
			}
		}
	}
}

func (s *AppState) SubscribeStatus() chan Status {
	ch := make(chan Status, 10)
	s.statusSubCh <- ch
	return ch
}

func (s *AppState) UnsubscribeStatus(ch chan Status) {
	s.statusSubCh <- ch
}

func (s *AppState) broadcastStatus(ctx context.Context) {
	subs := map[chan Status]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-s.statusCh:
			for ch := range subs {
				select {
				case ch <- st:
				default:
				}
			}
		case ch := <-s.statusSubCh:
			if _, ok := subs[ch]; ok {
				delete(subs, ch)
				close(ch)
			} else {
				subs[ch] = struct{}{}
			}
		}
	}
}
