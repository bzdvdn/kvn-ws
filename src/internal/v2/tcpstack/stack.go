// Package tcpstack terminates TCP flows from the TUN device with gVisor netstack
// and bridges each flow to a v2 stream.
package tcpstack

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"go.uber.org/zap"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

// DialFunc opens a v2 stream to target.
type DialFunc func(ctx context.Context, target string) (io.ReadWriteCloser, error)

// Stack terminates TCP from TUN and forwards each flow through DialFunc.
type Stack struct {
	ep     *channel.Endpoint
	stack  *stack.Stack
	dial   DialFunc
	logger *zap.Logger
	mu     sync.Mutex
	closed bool
}

// New creates a gVisor netstack for one local TUN address.
func New(localIP net.IP, mtu int, dial DialFunc, logger *zap.Logger) (*Stack, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if mtu <= 0 {
		mtu = 1400
	}
	ep := channel.New(1024, uint32(mtu), "")
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	if err := s.CreateNIC(1, ep); err != nil {
		return nil, fmt.Errorf("create nic: %v", err)
	}
	ip4 := localIP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("v2 TUN address must be IPv4")
	}
	addr := tcpip.AddrFrom4([4]byte{ip4[0], ip4[1], ip4[2], ip4[3]})
	if err := s.AddProtocolAddress(1, tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: addr, PrefixLen: 24},
	}, stack.AddressProperties{}); err != nil {
		return nil, fmt.Errorf("add address: %v", err)
	}
	s.SetRouteTable([]tcpip.Route{{
		Destination: header.IPv4EmptySubnet,
		NIC:         1,
	}})
	if err := s.SetPromiscuousMode(1, true); err != nil {
		return nil, fmt.Errorf("promiscuous mode: %v", err)
	}
	if err := s.SetSpoofing(1, true); err != nil {
		return nil, fmt.Errorf("spoofing mode: %v", err)
	}
	ts := &Stack{ep: ep, stack: s, dial: dial, logger: logger}
	forwarder := tcp.NewForwarder(s, 0, 4096, ts.handle)
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, forwarder.HandlePacket)
	return ts, nil
}

func (s *Stack) handle(req *tcp.ForwarderRequest) {
	id := req.ID()
	target := net.JoinHostPort(id.LocalAddress.String(), strconv.Itoa(int(id.LocalPort)))
	var wq waiter.Queue
	ep, err := req.CreateEndpoint(&wq)
	if err != nil {
		req.Complete(true)
		return
	}
	local := gonet.NewTCPConn(&wq, ep)
	go s.bridge(local, target)
}

func (s *Stack) bridge(local net.Conn, target string) {
	defer func() { _ = local.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := s.dial(ctx, target)
	if err != nil {
		s.logger.Debug("v2 tcp flow dial failed", zap.String("target", target), zap.Error(err))
		return
	}
	defer func() { _ = stream.Close() }()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(stream, local); done <- struct{}{} }()
	go func() { _, _ = io.Copy(local, stream); done <- struct{}{} }()
	<-done
}

// Inject feeds one raw IPv4 packet from TUN into netstack.
func (s *Stack) Inject(packet []byte) {
	if len(packet) < 20 || packet[0]>>4 != 4 || packet[9] != 6 {
		return
	}
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(packet),
	})
	defer pkt.DecRef()
	s.ep.InjectInbound(ipv4.ProtocolNumber, pkt)
}

// Run pumps outgoing TCP packets from netstack to write.
func (s *Stack) Run(ctx context.Context, write func([]byte) error) {
	for {
		pkt := s.ep.ReadContext(ctx)
		if pkt == nil {
			return
		}
		var packet []byte
		for _, part := range pkt.AsSlices() {
			packet = append(packet, part...)
		}
		pkt.DecRef()
		if len(packet) > 0 {
			if err := write(packet); err != nil {
				s.logger.Debug("v2 tun write failed", zap.Error(err))
				return
			}
		}
	}
}
