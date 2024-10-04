package bufpipe

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"time"
)

var (
	// ErrBufferFull is returned when the buffer is full
	ErrBufferFull = errors.New("bufpipe: buffer is full")
	// ErrClosedPipe is returned when the pipe is closed
	ErrClosedPipe = errors.New("bufpipe: closed pipe")
	// ErrFirstReadTimeout is returned when the pipe is set to block writes until the first read, but the timeout is reached
	ErrFirstReadTimeout = errors.New("bufpipe: write blocked because first read didn't happen until timeout")
)

const internalBufSize = 4096

type Options struct {
	// MaxSize is the maximum size of the buffer. Write will return ErrBufferFull
	// if the buffer is full. If MaxSize is 0, buffer is unlimited. Default is 0.
	MaxSize int
	// BlockWritesUntilFirstReadTimeout if set to a non-zero value, Write will block until the first Read is called. This is useful
	// if you want to make sure that the reader is ready to read before writing to the pipe. Default is false.
	BlockWritesUntilFirstReadTimeout time.Duration
	// BlockWritesOnFullBufferTimeout if set to a non-zero value, Write will block until the buffer is emptied by some Read() calls.
	// This is useful if you want to make sure that the reader has read some data before writing more. Default is no timeout - return ErrBufferFull immediately.
	// Only works if MaxSize is set to a non-zero value. If the timeout is reached, Write will return ErrBufferFull
	BlockWritesOnFullBufferTimeout time.Duration
}

type pipe struct {
	Options

	buf *bytes.Buffer

	flagWritten chan struct{}
	flagRead    chan struct{}

	closed    chan struct{}
	firstRead chan struct{}

	err error

	mu sync.Mutex
}

type PipeReader interface {
	io.Reader
	io.Closer
}

type PipeWriter interface {
	io.Writer
	io.Closer
	Len() int
}

// Pipe creates a buffered in-memory pipe. It can be used to connect code expecting an io.Reader with
// code expecting an io.Writer, but unlike io.Pipe it has internal buffer and can be used to pass
// data between goroutines without blocking.
func Pipe(options Options) (PipeReader, PipeWriter) {
	internalSize := options.MaxSize
	if internalSize <= 0 {
		internalSize = internalBufSize
	}
	buf := make([]byte, 0, internalSize)

	p := &pipe{
		Options: options,
		buf:     bytes.NewBuffer(buf),

		flagWritten: make(chan struct{}, 1),
		flagRead:    make(chan struct{}, 1),
		firstRead:   make(chan struct{}),

		closed: make(chan struct{}),
		err:    nil,
	}

	if options.BlockWritesUntilFirstReadTimeout == 0 {
		close(p.firstRead)
	}

	return p, p
}

func (f *pipe) raiseWritten() {
	select {
	case <-f.closed:
		return
	case f.flagWritten <- struct{}{}:
	default:
	}
}

func (f *pipe) raiseRead() {
	select {
	case <-f.closed:
		return
	case f.flagRead <- struct{}{}:
	default:
	}
}

func (f *pipe) close() {
	select {
	case <-f.closed:
		return
	default:
		f.raiseWritten()
		f.raiseRead()
		close(f.closed)
	}
}

// Close closes the pipe, rendering it unusable for I/O.
func (f *pipe) Close() error {
	f.close()
	return nil
}

// Write writes len(p) bytes from p to the pipe. It returns the number of bytes written. If pipe is closed
// or buffer is full, it returns ErrClosedPipe or ErrBufferFull respectively.
func (f *pipe) Write(p []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, ErrClosedPipe
	default:
	}

	if f.BlockWritesUntilFirstReadTimeout > 0 {
		select {
		case <-f.closed:
			return 0, ErrClosedPipe
		case <-time.After(f.BlockWritesUntilFirstReadTimeout):
			return 0, ErrFirstReadTimeout
		case <-f.firstRead:
		}
	}

	if f.MaxSize > 0 && f.buf.Len()+len(p) > f.MaxSize {

		if f.BlockWritesOnFullBufferTimeout == 0 {
			f.err = ErrBufferFull
			f.close()
			return 0, f.err
		}

		// block until timeout --> error, or until read happened and freed enough space --> continue
		haveSpace := false
		deadline := time.Now().Add(f.BlockWritesOnFullBufferTimeout)

		for !haveSpace {
			f.raiseWritten() // make sure the reader is ready to read
			select {
			case <-f.closed:
				return 0, ErrClosedPipe
			case <-time.After(time.Until(deadline)):
				f.err = ErrBufferFull
				f.close()
				return 0, f.err
			case <-f.flagRead:
				if f.buf.Len()+len(p) <= f.MaxSize {
					haveSpace = true
				}
			}
		}

	}

	f.mu.Lock()
	defer f.mu.Unlock()

	n, _ := f.buf.Write(p)

	f.raiseWritten()
	return n, nil
}

// Read reads up to len(p) bytes into p. It returns the number of bytes
// read (0 <= n <= len(p)) and any error encountered on the write side of the pipe.
// Read returns io.EOF when the write side has been closed.
func (f *pipe) Read(p []byte) (int, error) {

	if f.BlockWritesUntilFirstReadTimeout > 0 {
		select {
		case <-f.firstRead:
		default:
			close(f.firstRead)
		}
	}

	// wait for whatever signal trick
	select {
	case <-f.flagWritten:
	case <-f.closed:
	}

	// check and report errors
	if f.err != nil {
		err := f.err
		f.err = nil
		return 0, err
	}

	defer f.raiseRead()

	// check if closed
	select {
	case <-f.closed:
		return 0, io.EOF
	default:
		// actual read
		f.mu.Lock()
		defer f.mu.Unlock()

		n, _ := f.buf.Read(p)
		if f.buf.Len() != 0 {
			f.raiseWritten() // signal that there is more data to read
		}

		return n, nil
	}

}

// Len returns the number of bytes of the unread portion of the underlying buffer
func (f *pipe) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.Len()
}
