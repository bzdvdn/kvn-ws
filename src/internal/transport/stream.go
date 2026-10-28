package transport

import (
	"io"
)

// Stream is a bidirectional TCP stream over one HTTP/2 request.
type Stream struct {
	body      io.ReadCloser
	writer    *io.PipeWriter
	padding   bool
	padSize   int
	frameRead *frameReader
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) {
	if !s.padding {
		return s.body.Read(p)
	}
	if s.frameRead == nil {
		s.frameRead = newFrameReader(s.body)
	}
	return s.frameRead.Read(p)
}

// Write implements io.Writer.
func (s *Stream) Write(p []byte) (int, error) {
	if !s.padding {
		return s.writer.Write(p)
	}
	return newFrameWriter(s.writer, s.padSize).Write(p)
}

// CloseWrite half-closes the request body.
func (s *Stream) CloseWrite() error { return s.writer.Close() }

// Close closes both directions.
func (s *Stream) Close() error {
	_ = s.writer.Close()
	return s.body.Close()
}

// DatagramStream is a best-effort UDP flow over one HTTP/2 request.
type DatagramStream struct {
	body    io.ReadCloser
	writer  *io.PipeWriter
	padding bool
	padSize int
}

// WritePacket sends one datagram.
func (s *DatagramStream) WritePacket(payload []byte) error {
	return writeDatagram(s.writer, payload, s.padding, s.padSize)
}

// ReadPacket receives one datagram.
func (s *DatagramStream) ReadPacket() ([]byte, error) {
	return readDatagram(s.body, s.padding)
}

// Close closes both directions.
func (s *DatagramStream) Close() error {
	_ = s.writer.Close()
	return s.body.Close()
}
