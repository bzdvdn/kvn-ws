package quic

import (
	"encoding/binary"
	"io"
	"math"
)

// @sk-task whitelist-obfuscation#T4.1: TLS Exporter nonce + full payload XOR (AC-006)
type ObfuscatedQUICConn struct {
	*QUICConn
	nonce     [8]byte
	nonceInit bool
}

// @sk-task whitelist-obfuscation#T4.1: NewObfuscatedQUICConn without isClient param (AC-006)
func NewObfuscatedQUICConn(core *QUICConn) (*ObfuscatedQUICConn, error) {
	return &ObfuscatedQUICConn{QUICConn: core}, nil
}

// @sk-task whitelist-obfuscation#T4.1: SetNonce for test injection (AC-006)
func (oc *ObfuscatedQUICConn) SetNonce(n [8]byte) {
	oc.nonce = n
	oc.nonceInit = true
}

// @sk-task whitelist-obfuscation#T4.1: deferred init via TLS Exporter (AC-006)
func (oc *ObfuscatedQUICConn) initNonce() error {
	if oc.nonceInit {
		return nil
	}
	if oc.conn == nil {
		// test mode — zero nonce (no obfuscation)
		oc.nonceInit = true
		return nil
	}
	tlsState := oc.conn.ConnectionState().TLS
	material, err := tlsState.ExportKeyingMaterial("kvn-obfuscation", nil, 8)
	if err != nil {
		return err
	}
	copy(oc.nonce[:], material)
	oc.nonceInit = true
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
	if oc.maxMessageSize >= 0 && msgLen > uint32(oc.maxMessageSize) { //nolint:gosec // maxMessageSize >= 0 per SetMaxMessageSize
		return nil, ErrMessageTooLarge
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(oc.stream, buf); err != nil {
		return nil, err
	}
	xorBytes(buf, buf, oc.nonce[:])
	return buf, nil
}

// @sk-task whitelist-obfuscation#T4.1: full payload XOR in WriteMessage (AC-006)
func (oc *ObfuscatedQUICConn) WriteMessage(data []byte) error {
	if len(data) > math.MaxUint32 {
		return io.ErrShortWrite
	}
	if err := oc.initNonce(); err != nil {
		return err
	}
	oc.mu.Lock()
	defer oc.mu.Unlock()
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data))) // #nosec G115 — checked at line 79
	xorBytes(lenBuf[:], lenBuf[:], oc.nonce[:])
	if _, err := oc.stream.Write(lenBuf[:]); err != nil {
		return err
	}
	xorBuf := make([]byte, len(data))
	xorBytes(xorBuf, data, oc.nonce[:])
	_, err := oc.stream.Write(xorBuf)
	return err
}
