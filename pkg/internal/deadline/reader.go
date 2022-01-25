package deadline

import (
	"bytes"
	"io"
	"os"
	"time"
)

// Reader allows to set a deadline for Read calls.
type Reader interface {
	io.Reader
	SetDeadline(t time.Time) error
}

type reader struct {
	readBuf bytes.Buffer

	sink    chan []byte
	readErr chan error
	errored error

	deadline
}

type ReadFn func() (b []byte, err error)

// NewReader returns a Reader that can only be used until an error is returned.
func NewReader(read ReadFn) Reader {
	r := &reader{
		deadline: deadline{timeout: make(chan struct{})},
		sink:     make(chan []byte, 1),
		readErr:  make(chan error, 1),
	}
	go func() {
		for {
			data, err := read()
			if err != nil {
				r.readErr <- err
				return
			}
			r.sink <- data
		}
	}()
	return r
}

func (r *reader) Read(b []byte) (int, error) {
	if r.errored != nil {
		return 0, r.errored
	}
	for r.readBuf.Len() < len(b) {
		select {
		case r.errored = <-r.readErr:
			return 0, r.errored
		case data := <-r.sink:
			_, err := r.readBuf.Write(data)
			if err != nil {
				return 0, err
			}
		case <-r.timeout:
			return 0, os.ErrDeadlineExceeded
		}
	}
	return r.readBuf.Read(b)
}
