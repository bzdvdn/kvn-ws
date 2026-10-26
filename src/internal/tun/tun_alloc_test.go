package tun

import (
	"os"
	"testing"

	"golang.zx2c4.com/wireguard/tun"
)

type fakeWireguardDevice struct{}

func (f *fakeWireguardDevice) File() *os.File { return nil }
func (f *fakeWireguardDevice) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(bufs) == 0 || len(sizes) == 0 || len(bufs[0]) < offset+20 {
		return 0, nil
	}
	bufs[0][offset] = 0x45
	sizes[0] = 20
	return 1, nil
}
func (f *fakeWireguardDevice) Write(bufs [][]byte, offset int) (int, error) {
	return len(bufs), nil
}
func (f *fakeWireguardDevice) MTU() (int, error)        { return 1400, nil }
func (f *fakeWireguardDevice) Name() (string, error)    { return "fake", nil }
func (f *fakeWireguardDevice) Events() <-chan tun.Event { return nil }
func (f *fakeWireguardDevice) Close() error             { return nil }
func (f *fakeWireguardDevice) BatchSize() int           { return 1 }

// @sk-test tunnel-primary-backpressure#T2.3: TUN read/write reuse buffers (AC-003)
func TestTunReadWriteAllocs(t *testing.T) {
	dev := &tunDevice{device: &fakeWireguardDevice{}}
	buf := make([]byte, 1400)

	for i := 0; i < 20; i++ {
		if _, err := dev.Write(buf); err != nil {
			t.Fatalf("write warmup: %v", err)
		}
		if _, err := dev.Read(buf); err != nil {
			t.Fatalf("read warmup: %v", err)
		}
	}

	allocs := testing.AllocsPerRun(200, func() {
		_, _ = dev.Write(buf)
		_, _ = dev.Read(buf)
	})
	if allocs > 1 {
		t.Fatalf("allocs/op = %.2f, want <= 1", allocs)
	}
}
