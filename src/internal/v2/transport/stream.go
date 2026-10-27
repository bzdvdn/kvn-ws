package transport

import (
	"io"
)

// Stream is a bidirectional TCP stream over one HTTP/2 request.
type Stream struct {
	body   io.ReadCloser
	writer *io.PipeWriter
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) { return s.body.Read(p) }

// Write implements io.Writer.
func (s *Stream) Write(p []byte) (int, error) { return s.writer.Write(p) }

// Close closes both directions.
func (s *Stream) Close() error {
	_ = s.writer.Close()
	return s.body.Close()
}

// DatagramStream is a best-effort UDP flow over one HTTP/2 request.
type DatagramStream struct {
	body   io.ReadCloser
	writer *io.PipeWriter
}

// WritePacket sends one datagram.
func (s *DatagramStream) WritePacket(payload []byte) error {
	return writeDatagram(s.writer, payload)
}

// ReadPacket receives one datagram.
func (s *DatagramStream) ReadPacket() ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(s.body, hdr[:]); err != nil {
		return nil, err
	}
	n := int(hdr[0])<<8 | int(hdr[1])
	if n > MaxDatagramSize {
		return nil, errDatagramTooLarge
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(s.body, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// Close closes both directions.
func (s *DatagramStream) Close() error {
	_ = s.writer.Close()
	return s.body.Close()
}
