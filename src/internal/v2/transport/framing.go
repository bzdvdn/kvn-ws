package transport

import (
	"encoding/binary"
	"io"
	"net/http"
)

func writeDatagram(w io.Writer, payload []byte) error {
	if len(payload) > MaxDatagramSize {
		payload = payload[:MaxDatagramSize]
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readDatagramStream(r io.Reader, fn func([]byte)) {
	var hdr [2]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return
		}
		n := int(binary.BigEndian.Uint16(hdr[:]))
		if n > MaxDatagramSize {
			return
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
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
