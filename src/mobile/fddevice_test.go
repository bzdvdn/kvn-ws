package mobile

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func newPipe(t *testing.T) (readFD, writeFD int) {
	t.Helper()
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = unix.Close(fds[0])
		_ = unix.Close(fds[1])
	})
	return fds[0], fds[1]
}

func TestFDDeviceReadWrite(t *testing.T) {
	rfd, wfd := newPipe(t)
	dev := newFDDevice(rfd)
	if _, err := unix.Write(wfd, []byte("hello")); err != nil {
		t.Fatalf("write pipe: %v", err)
	}

	buf := make([]byte, 16)
	deadline := time.Now().Add(2 * time.Second)
	for {
		n, err := dev.Read(buf)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if n > 0 {
			if got := string(buf[:n]); got != "hello" {
				t.Fatalf("read %q, want %q", got, "hello")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for data")
		}
	}
}

func TestFDDeviceReadStopsOnClose(t *testing.T) {
	rfd, _ := newPipe(t)
	dev := newFDDevice(rfd)

	done := make(chan error, 1)
	go func() {
		_, err := dev.Read(make([]byte, 16))
		done <- err
	}()

	// Let the reader block in poll on the empty pipe.
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	if err := dev.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read returned nil error after close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read did not return after close")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("read stopped after %v, want bounded by poll timeout", elapsed)
	}

	// Subsequent reads must fail fast too.
	start = time.Now()
	if _, err := dev.Read(make([]byte, 4)); err == nil {
		t.Fatal("read after close returned nil error")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("read after close took %v", elapsed)
	}
}

func TestFDDeviceCloseIdempotent(t *testing.T) {
	rfd, _ := newPipe(t)
	dev := newFDDevice(rfd)
	if err := dev.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := dev.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
