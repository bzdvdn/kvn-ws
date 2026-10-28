package transport

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxPaddingSize = 4096

type frameWriter struct {
	w    io.Writer
	size int
}

func newFrameWriter(w io.Writer, size int) *frameWriter {
	if size < 0 {
		size = 0
	}
	if size > maxPaddingSize {
		size = maxPaddingSize
	}
	return &frameWriter{w: w, size: size}
}

func (w *frameWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	pad := adaptivePadding(len(p), w.size)
	var hdr [6]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(p))) // #nosec G115 -- frame length is bounded by reader limit
	binary.BigEndian.PutUint16(hdr[4:], uint16(pad))    // #nosec G115 -- pad is capped at maxPaddingSize
	if _, err := w.w.Write(hdr[:]); err != nil {
		return 0, err
	}
	if _, err := w.w.Write(p); err != nil {
		return 0, err
	}
	if pad > 0 {
		buf := make([]byte, pad)
		if _, err := cryptorand.Read(buf); err != nil {
			return 0, err
		}
		if _, err := w.w.Write(buf); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

type frameReader struct {
	r   io.Reader
	buf []byte
	off int
}

func newFrameReader(r io.Reader) *frameReader { return &frameReader{r: r} }

func (r *frameReader) Read(p []byte) (int, error) {
	for r.off >= len(r.buf) {
		var hdr [6]byte
		if _, err := io.ReadFull(r.r, hdr[:]); err != nil {
			return 0, err
		}
		n := int(binary.BigEndian.Uint32(hdr[:4]))
		pad := int(binary.BigEndian.Uint16(hdr[4:]))
		if n < 0 || n > 16<<20 || pad > maxPaddingSize {
			return 0, fmt.Errorf("invalid padded frame length=%d pad=%d", n, pad)
		}
		frame := make([]byte, n+pad)
		if _, err := io.ReadFull(r.r, frame); err != nil {
			return 0, err
		}
		r.buf = frame[:n]
		r.off = 0
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

func adaptivePadding(n, size int) int {
	if n <= 0 || size <= 0 {
		return 0
	}
	// Only pad small/control frames; large data keeps its native size.
	if n > 512 {
		return 0
	}
	if size > maxPaddingSize {
		size = maxPaddingSize
	}
	return size
}

func paddingRequested(header string) bool {
	return strings.Contains(header, "pad=")
}

func requestedPaddingSize(header string, fallback int) int {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "pad=") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(part, "pad="))
		if err != nil || n < 0 {
			return fallback
		}
		if n > maxPaddingSize {
			return maxPaddingSize
		}
		return n
	}
	return fallback
}
