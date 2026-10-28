package webui

import (
	"context"
	"testing"
)

type fakeManagedClient struct{ stopped bool }

func (f *fakeManagedClient) StopTun() { f.stopped = true }

// @sk-test kvn-v2-transport#T3.3: stale v2 client is cancelled and TUN released (AC-005)
func TestStopStaleClientReleasesV2Tun(t *testing.T) {
	s := &Server{state: NewAppState()}
	cl := &fakeManagedClient{}
	done := make(chan struct{})
	close(done)
	_, cancel := context.WithCancel(context.Background())
	s.state.setClient(cl)
	s.state.SetCancel(cancel)
	s.state.SetDoneCh(done)
	s.stopStaleClientLocked()
	if !cl.stopped {
		t.Fatal("v2 StopTun was not called")
	}
	if s.state.Client() != nil || s.state.Cancel() != nil || s.state.DoneCh() != nil {
		t.Fatal("stale v2 client state was not cleared")
	}
}
