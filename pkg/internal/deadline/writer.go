package deadline

import (
	"io"
	"os"
	"time"
)

// Writer allows to set a deadline for Write calls.
type Writer interface {
	io.WriteCloser
	SetDeadline(t time.Time) error
}

type deadlineWriter struct {
	writeRequest chan []byte
	writeErr     chan error
	errored      error
	deadline
}

type WriteFn func(b []byte) (err error)

// NewWriter returns a Writer that can only be used until an error is returned.
func NewWriter(write WriteFn) Writer {
	w := &deadlineWriter{
		deadline:     deadline{timeout: make(chan struct{})},
		writeRequest: make(chan []byte),
		writeErr:     make(chan error),
	}
	go func() {
		var err error
		for {
			r, ok := <-w.writeRequest
			if !ok {
				return // closed
			}
			err = write(r)
			w.writeErr <- err
			if err != nil {
				return
			}
		}
	}()
	return w
}

func (w *deadlineWriter) Close() error {
	if w.writeRequest != nil {
		select {
		case <-w.writeRequest:
		default:
			close(w.writeRequest)
		}
	}
	return nil
}

func (w *deadlineWriter) Write(b []byte) (int, error) {
	if w.errored != nil {
		return 0, w.errored
	}
	// Request write
	select {
	case w.writeRequest <- b:
	case <-w.timeout:
		return 0, os.ErrDeadlineExceeded
	case w.errored = <-w.writeErr:
		return 0, w.errored
	}
	// Acknowledge write
	select {
	case w.errored = <-w.writeErr:
		return len(b), w.errored
	case <-w.timeout:
		return 0, os.ErrDeadlineExceeded
	}
}
