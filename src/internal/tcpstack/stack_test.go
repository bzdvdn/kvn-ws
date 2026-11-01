package tcpstack

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	gvisorstack "gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

const (
	sutIP   = "10.66.0.1"
	peerMTU = 1400
)

// testPeer is a second gVisor stack that acts as the remote TCP endpoint.
type testPeer struct {
	stack *gvisorstack.Stack
	ep    *channel.Endpoint
}

func newTestPeer(t *testing.T) *testPeer {
	t.Helper()
	ep := channel.New(1024, peerMTU, "")
	s := gvisorstack.New(gvisorstack.Options{
		NetworkProtocols:   []gvisorstack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []gvisorstack.TransportProtocolFactory{tcp.NewProtocol},
	})
	if err := s.CreateNIC(1, ep); err != nil {
		t.Fatalf("create peer nic: %v", err)
	}
	if err := s.AddProtocolAddress(1, tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: tcpip.AddrFrom4([4]byte{10, 66, 0, 2}), PrefixLen: 24},
	}, gvisorstack.AddressProperties{}); err != nil {
		t.Fatalf("add peer address: %v", err)
	}
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})
	return &testPeer{stack: s, ep: ep}
}

// inject copies raw IP bytes into a fresh inbound packet buffer. Packet buffers
// taken from an outbound channel queue cannot be re-injected directly.
func inject(ep *channel.Endpoint, packet []byte) {
	pkt := gvisorstack.NewPacketBuffer(gvisorstack.PacketBufferOptions{
		Payload: buffer.MakeWithData(append([]byte(nil), packet...)),
	})
	defer pkt.DecRef()
	ep.InjectInbound(ipv4.ProtocolNumber, pkt)
}

// wire connects the stack under test with the peer and runs both directions.
func wire(ctx context.Context, sut *Stack, peer *testPeer) {
	go sut.Run(ctx, func(packet []byte) error {
		inject(peer.ep, packet)
		return nil
	})
	go func() {
		for {
			pkt := peer.ep.ReadContext(ctx)
			if pkt == nil {
				return
			}
			var packet []byte
			for _, part := range pkt.AsSlices() {
				packet = append(packet, part...)
			}
			pkt.DecRef()
			inject(sut.ep, packet)
		}
	}()
}

func echoDial(context.Context, string) (io.ReadWriteCloser, error) {
	a, b := net.Pipe()
	go func() {
		defer func() { _ = b.Close() }()
		_, _ = io.Copy(b, b)
	}()
	return a, nil
}

func dialPeer(t *testing.T, peer *testPeer, port uint16) *gonet.TCPConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addr := tcpip.FullAddress{Addr: tcpip.AddrFrom4([4]byte{10, 66, 0, 1}), Port: port}
	conn, err := gonet.DialTCPWithBind(ctx, peer.stack, tcpip.FullAddress{}, addr, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	return conn
}

// TestForwardRequestsReleased proves that every accepted connection releases
// its forwarder slot: more connections than the in-flight limit still succeed.
func TestForwardRequestsReleased(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	peer := newTestPeer(t)
	const limit = 2
	sut, err := newWithForwardLimit(ctx, net.ParseIP(sutIP), peerMTU, limit, echoDial, zap.NewNop())
	if err != nil {
		t.Fatalf("new stack: %v", err)
	}
	wire(ctx, sut, peer)

	for i := 0; i < 5; i++ {
		conn := dialPeer(t, peer, uint16(8080+i))
		if _, err := conn.Write([]byte("ping")); err != nil {
			t.Fatalf("conn %d write: %v", i, err)
		}
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Fatalf("conn %d read: %v", i, err)
		}
		_ = conn.Close()
		time.Sleep(50 * time.Millisecond)
	}

	if got := sut.stack.Stats().TCP.ForwardMaxInFlightDrop.Value(); got != 0 {
		t.Fatalf("ForwardMaxInFlightDrop = %d, want 0", got)
	}
}

// TestStackDialContextCanceled proves forwarded dials inherit the stack
// context, so canceling the session cancels in-flight dials.
func TestStackDialContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialCtx := make(chan context.Context, 1)
	dial := func(dctx context.Context, _ string) (io.ReadWriteCloser, error) {
		select {
		case dialCtx <- dctx:
		default:
		}
		<-dctx.Done()
		return nil, dctx.Err()
	}

	peer := newTestPeer(t)
	sut, err := newWithForwardLimit(ctx, net.ParseIP(sutIP), peerMTU, defaultMaxInFlight, dial, zap.NewNop())
	if err != nil {
		t.Fatalf("new stack: %v", err)
	}
	wire(ctx, sut, peer)

	conn := dialPeer(t, peer, 9090)
	defer func() { _ = conn.Close() }()

	var received context.Context
	select {
	case received = <-dialCtx:
	case <-time.After(3 * time.Second):
		t.Fatal("forwarded dial was not invoked")
	}

	cancel()
	select {
	case <-received.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("forwarded dial context was not canceled with the stack context")
	}
}
