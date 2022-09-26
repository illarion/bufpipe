package bufpipe

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

var (
	// ErrBufferFull is returned when the buffer is full
	ErrBufferFull = errors.New("bufpipe: buffer full")
	// ErrClosedPipe is returned when the pipe is closed
	ErrClosedPipe = errors.New("bufpipe: closed pipe")
)

const defaultBufSize = 4096

type pipe struct {
	maxSize        int
	blockOnMaxSize bool
	buf            *bytes.Buffer
	flag           chan struct{}
	closed         chan struct{}
	err            error

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
// data between goroutines without blocking. If maxSize > 0, the buffer will be limited to that size, and
// Write will return ErrBufferFull if the buffer is full. If maxSize is 0, buffer is unlimited.
func Pipe(maxSize int) (PipeReader, PipeWriter) {
	size := maxSize
	if maxSize <= 0 {
		size = defaultBufSize
	}
	buf := make([]byte, 0, size)

	p := &pipe{
		maxSize: maxSize,
		buf:     bytes.NewBuffer(buf),
		flag:    make(chan struct{}, 1),
		closed:  make(chan struct{}),
		err:     nil,
	}

	return p, p
}

func (f *pipe) raise() {
	select {
	case <-f.closed:
		return
	case f.flag <- struct{}{}:
	default:
	}
}

func (f *pipe) close() {
	select {
	case <-f.closed:
		return
	default:
		f.raise()
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

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.maxSize > 0 && f.buf.Len() > f.maxSize {
		f.err = ErrBufferFull
		f.close()
		return 0, f.err
	}

	n, _ := f.buf.Write(p)

	f.raise()
	return n, nil
}

// Read reads up to len(p) bytes into p. It returns the number of bytes
// read (0 <= n <= len(p)) and any error encountered on the write side of the pipe.
// Read returns io.EOF when the write side has been closed.
func (f *pipe) Read(p []byte) (int, error) {

	// wait for whatever signal trick
	select {
	case <-f.flag:
	case <-f.closed:
	}

	// check and report errors
	if f.err != nil {
		err := f.err
		f.err = nil
		return 0, err
	}

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
			f.raise()
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
