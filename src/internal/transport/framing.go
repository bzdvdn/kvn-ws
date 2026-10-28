package transport

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"io"
	"net/http"
)

func writeDatagram(w io.Writer, payload []byte, pad bool, size int) error {
	if len(payload) > MaxDatagramSize {
		payload = payload[:MaxDatagramSize]
	}
	padLen := 0
	if pad {
		padLen = adaptivePadding(len(payload), size)
	}
	if pad {
		var hdr [4]byte
		binary.BigEndian.PutUint16(hdr[:2], uint16(len(payload)))
		binary.BigEndian.PutUint16(hdr[2:], uint16(padLen))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
	} else {
		var hdr [2]byte
		binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if padLen > 0 {
		buf := make([]byte, padLen)
		if _, err := cryptorand.Read(buf); err != nil {
			return err
		}
		_, err := w.Write(buf)
		return err
	}
	return nil
}

func readDatagram(r io.Reader, pad bool) ([]byte, error) {
	if !pad {
		var hdr [2]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint16(hdr[:]))
		if n > MaxDatagramSize {
			return nil, errDatagramTooLarge
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
		return payload, nil
	}
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(hdr[:2]))
	padLen := int(binary.BigEndian.Uint16(hdr[2:]))
	if n > MaxDatagramSize || padLen > maxPaddingSize {
		return nil, errDatagramTooLarge
	}
	buf := make([]byte, n+padLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func readDatagramStream(r io.Reader, pad bool, fn func([]byte)) {
	for {
		payload, err := readDatagram(r, pad)
		if err != nil {
			return
		}
		fn(payload)
	}
}

type flushWriter struct {
	w  io.Writer
	fw http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.fw != nil {
		fw.fw.Flush()
	}
	return n, err
}
