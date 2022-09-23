package bufpipe

import (
	"bytes"
	"io"
	"sync"
)

type Pipe struct {
	maxSize int
	buf     *bytes.Buffer
	flag    chan struct{}
	closed  chan struct{}
	err     error

	mu sync.Mutex
}

func New(maxSize int) *Pipe {
	return &Pipe{
		maxSize: maxSize,
		buf:     bytes.NewBuffer(make([]byte, 0, maxSize)),
		flag:    make(chan struct{}, 1),
		closed:  make(chan struct{}),
		err:     nil,
	}
}

func (f *Pipe) raise() {
	select {
	case <-f.closed:
		return
	case f.flag <- struct{}{}:
	default:
	}
}

func (f *Pipe) close() {
	select {
	case <-f.closed:
		return
	default:
		f.raise()
		close(f.closed)
	}
}

func (f *Pipe) Close() error {
	f.close()
	return nil
}

func (f *Pipe) Write(p []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.buf.Len() > f.maxSize {
		f.err = io.ErrShortBuffer
		f.close()
		return 0, f.err
	}

	n, _ := f.buf.Write(p)

	f.raise()
	return n, nil
}

func (f *Pipe) Read(p []byte) (int, error) {

	// wait for whatever sygnal trick
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

func (f *Pipe) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.Len()
}

func (f *Pipe) Cap() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.Cap()
}
