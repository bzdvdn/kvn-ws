// @sk-task quic-obfuscation#T1.1: ObfuscatedQUICConn — 8-byte nonce + XOR length prefix (AC-001, AC-003)
package quic

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"math"
)

type ObfuscatedQUICConn struct {
	*QUICConn
	nonce    [8]byte
	nonceSet bool
}

func NewObfuscatedQUICConn(core *QUICConn, isClient bool) (*ObfuscatedQUICConn, error) {
	oc := &ObfuscatedQUICConn{QUICConn: core}
	if isClient {
		if _, err := rand.Read(oc.nonce[:]); err != nil {
			return nil, err
		}
		if _, err := core.stream.Write(oc.nonce[:]); err != nil {
			return nil, err
		}
		oc.nonceSet = true
	}
	return oc, nil
}

func (oc *ObfuscatedQUICConn) ReadMessage() ([]byte, error) {
	if !oc.nonceSet {
		if _, err := io.ReadFull(oc.stream, oc.nonce[:]); err != nil {
			return nil, err
		}
		oc.nonceSet = true
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(oc.stream, lenBuf[:]); err != nil {
		return nil, err
	}
	for i := range lenBuf {
		lenBuf[i] ^= oc.nonce[i]
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(oc.stream, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (oc *ObfuscatedQUICConn) WriteMessage(data []byte) error {
	if len(data) > math.MaxUint32 {
		return io.ErrShortWrite
	}
	oc.mu.Lock()
	defer oc.mu.Unlock()
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	for i := range lenBuf {
		lenBuf[i] ^= oc.nonce[i]
	}
	if _, err := oc.stream.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := oc.stream.Write(data)
	return err
}
