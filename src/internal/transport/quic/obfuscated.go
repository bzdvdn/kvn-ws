package quic

import (
	"encoding/binary"
	"io"
	"math"
	"sync"
	"sync/atomic"
)

var xorBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 1500)
		return &b
	},
}

func getXorBuf(size int) []byte {
	ptr := xorBufPool.Get().(*[]byte)
	buf := *ptr
	if cap(buf) < size {
		buf = make([]byte, size)
	}
	return buf[:size]
}

func putXorBuf(buf []byte) {
	xorBufPool.Put(&buf)
}

// @sk-task whitelist-obfuscation#T4.1: TLS Exporter nonce + full payload XOR (AC-006)
// @sk-task performance-scope-p2#T1.3: nonceInit atomic.Bool (AC-003)
// @sk-task performance-scope-p2#T1.2: xorBuf sync.Pool (AC-002)
// @sk-task performance-scope-p2#T2.2: WriteMessage без общего mu (AC-007)
type ObfuscatedQUICConn struct {
	*QUICConn
	nonce     [8]byte
	nonceInit atomic.Bool
}

// @sk-task whitelist-obfuscation#T4.1: NewObfuscatedQUICConn without isClient param (AC-006)
func NewObfuscatedQUICConn(core *QUICConn) (*ObfuscatedQUICConn, error) {
	return &ObfuscatedQUICConn{QUICConn: core}, nil
}

// @sk-task whitelist-obfuscation#T4.1: SetNonce for test injection (AC-006)
func (oc *ObfuscatedQUICConn) SetNonce(n [8]byte) {
	oc.nonce = n
	oc.nonceInit.Store(true)
}

// @sk-task whitelist-obfuscation#T4.1: deferred init via TLS Exporter (AC-006)
// @sk-task performance-scope-p2#T1.3: CompareAndSwap for one-shot init (AC-003)
func (oc *ObfuscatedQUICConn) initNonce() error {
	if oc.nonceInit.Load() {
		return nil
	}
	if oc.conn == nil {
		// test mode — zero nonce (no obfuscation)
		oc.nonceInit.Store(true)
		return nil
	}
	if !oc.nonceInit.CompareAndSwap(false, true) {
		return nil // another goroutine is initializing
	}
	tlsState := oc.conn.ConnectionState().TLS
	material, err := tlsState.ExportKeyingMaterial("kvn-obfuscation", nil, 8)
	if err != nil {
		return err
	}
	copy(oc.nonce[:], material)
	return nil
}

// @sk-task whitelist-obfuscation#T4.1: xorBytes helper for payload obfuscation (AC-006)
func xorBytes(dst, src, nonce []byte) {
	for i := range src {
		dst[i] = src[i] ^ nonce[i%len(nonce)]
	}
}

// @sk-task whitelist-obfuscation#T4.1: full payload XOR in ReadMessage (AC-006)
// @sk-task arch-refactoring#T2.1: add MaxMessageSize limit (AC-001, AC-002)
// @sk-task performance-scope-p2#T1.1: sync.Pool for read buffer (AC-001)
func (oc *ObfuscatedQUICConn) ReadMessage() ([]byte, error) {
	if err := oc.initNonce(); err != nil {
		return nil, err
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(oc.stream, lenBuf[:]); err != nil {
		return nil, err
	}
	xorBytes(lenBuf[:], lenBuf[:], oc.nonce[:])
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if mms := oc.maxMessageSize.Load(); mms >= 0 && msgLen > uint32(mms) { // #nosec G115
		return nil, ErrMessageTooLarge
	}
	buf := getReadBuf(int(msgLen))
	if _, err := io.ReadFull(oc.stream, buf); err != nil {
		putReadBuf(buf)
		return nil, err
	}
	xorBytes(buf, buf, oc.nonce[:])
	return buf, nil
}

// @sk-task whitelist-obfuscation#T4.1: full payload XOR in WriteMessage (AC-006)
// @sk-task performance-scope-p2#T1.2: xorBuf from sync.Pool (AC-002)
// @sk-task performance-scope-p2#T2.2: WriteMessage without mu (AC-007)
func (oc *ObfuscatedQUICConn) WriteMessage(data []byte) error {
	if len(data) > math.MaxUint32 {
		return io.ErrShortWrite
	}
	if err := oc.initNonce(); err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data))) // #nosec G115 — checked above
	xorBytes(lenBuf[:], lenBuf[:], oc.nonce[:])
	if _, err := oc.stream.Write(lenBuf[:]); err != nil {
		return err
	}
	xorBuf := getXorBuf(len(data))
	xorBytes(xorBuf, data, oc.nonce[:])
	_, err := oc.stream.Write(xorBuf)
	putXorBuf(xorBuf)
	return err
}
