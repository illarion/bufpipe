package bufpipe

import (
	"errors"
	"testing"
	"time"
)

func TestPipeWriteRead(t *testing.T) {

	r, w := Pipe(Options{
		MaxSize: 0,
	})

	data := []byte("Hello, World!")
	w.Write(data)

	buf := make([]byte, len(data))
	n, err := r.Read(buf)
	if err != nil {
		t.Errorf("Error reading from pipe: %v", err)
		return
	}

	if n != len(data) {
		t.Errorf("Expected %d bytes, got %d", len(data), n)
		return
	}

	if string(buf) != string(data) {
		t.Errorf("Expected %s, got %s", string(data), string(buf))
		return
	}

}

func TestPipeWriteFailsOnMaxSize(t *testing.T) {

	_, w := Pipe(Options{
		MaxSize: 5,
	})

	data := []byte("Hello, World!")
	_, err := w.Write(data)
	if !errors.Is(err, ErrBufferFull) {
		t.Errorf("Expected ErrBufferFull, got %v", err)
		return
	}
}

func TestPipeWriteBlocksUntilFirstRead(t *testing.T) {
	r, w := Pipe(Options{
		BlockWritesUntilFirstReadTimeout: 200 * time.Millisecond,
	})

	data := []byte("Hello, World!")
	writeDone := make(chan struct{})

	go func() {
		defer close(writeDone)
		_, err := w.Write(data)
		if err != nil {
			t.Errorf("Error writing to pipe: %v", err)
		}
	}()

	select {
	case <-writeDone:
		t.Errorf("Write completed before read")
	case <-time.After(100 * time.Millisecond):
		// Expected timeout, write should block
	}

	buf := make([]byte, len(data))
	n, err := r.Read(buf)
	if err != nil {
		t.Errorf("Error reading from pipe: %v", err)
		return
	}

	if n != len(data) {
		t.Errorf("Expected %d bytes, got %d", len(data), n)
		return
	}

	if string(buf) != string(data) {
		t.Errorf("Expected %s, got %s", string(data), string(buf))
		return
	}

	select {
	case <-writeDone:
		// Write should complete after read
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Write did not complete after read")
	}
}

func TestPipeBlocksOnFullBufferTimeout(t *testing.T) {
	_, w := Pipe(Options{
		MaxSize:                        5,
		BlockWritesOnFullBufferTimeout: 200 * time.Millisecond,
	})

	data := []byte("Hello, World!")
	writeDone := make(chan struct{})
	var writeErr error

	go func() {
		defer close(writeDone)
		_, writeErr = w.Write(data)
	}()

	select {
	case <-writeDone:
		t.Errorf("Write completed before buffer timeout")
	case <-time.After(150 * time.Millisecond):
		// Expected timeout, write should block
	}

	<-writeDone

	if !errors.Is(writeErr, ErrBufferFull) {
		t.Errorf("Expected ErrBufferFull, got %v", writeErr)
	}

}

func TestPipeBlocksOnFullBufferTimeoutAndCompletesWithNoErrorIfReadEmptiesBuffer(t *testing.T) {

	r, w := Pipe(Options{
		MaxSize:                        5,
		BlockWritesOnFullBufferTimeout: 2000 * time.Millisecond,
	})

	data := []byte("1")
	writeDone := make(chan struct{})
	var writeErr error

	w.Write(data) //1
	w.Write(data) //2
	w.Write(data) //3
	w.Write(data) //4
	w.Write(data) //5

	go func() {
		defer close(writeDone)
		_, writeErr = w.Write(data) //6, should block until read empties buffer
	}()

	select {
	case <-writeDone:
		t.Errorf("Write completed before buffer timeout")
		return
	case <-time.After(150 * time.Millisecond):
		// Expected timeout, write should block
	}

	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Errorf("Error reading from pipe: %v", err)
		return
	}

	if n != 5 {
		t.Errorf("Expected 5 bytes, got %d", n)
		return
	}

	<-writeDone

	if writeErr != nil {
		t.Errorf("Expected no error, got %v", writeErr)
	}
}

func TestPipeBlocksOnFullBufferAndBlockUntilFirstRead(t *testing.T) {

	r, w := Pipe(Options{
		MaxSize:                          5,
		BlockWritesOnFullBufferTimeout:   200 * time.Millisecond,
		BlockWritesUntilFirstReadTimeout: 400 * time.Millisecond,
	})

	write1Done := make(chan struct{})
	var writeErr error

	go func() {
		// Write should block until first read
		defer close(write1Done)
		_, writeErr = w.Write([]byte("1"))
	}()

	select {
	case <-write1Done:
		t.Errorf("Write completed before first read timeout")
		return
	case <-time.After(300 * time.Millisecond):
		// Expected timeout, bigger than BlockWritesOnFullBufferTimeout, write should block
	}

	buf := make([]byte, 5)
	_, err := r.Read(buf)
	if err != nil {
		t.Errorf("Error reading from pipe: %v", err)
		return
	}

	// Write should complete after first read
	<-write1Done

	if writeErr != nil {
		t.Errorf("Expected no error, got %v", writeErr)
	}

	write2Done := make(chan struct{})

	go func() {
		// Write should block until first read
		defer close(write2Done)
		_, writeErr = w.Write([]byte("12345")) // 5 bytes, buffer full
		if writeErr != nil {
			return
		}
		_, writeErr = w.Write([]byte("6")) // 6th byte, should block until read empties buffer
	}()

	select {
	case <-write2Done:
		t.Errorf("Write completed before buffer timeout, with error: %v", writeErr)
		return
	case <-time.After(100 * time.Millisecond):
		// write will block until read empties buffer
	}

	buf = make([]byte, 5)
	_, err = r.Read(buf)
	if err != nil {
		t.Errorf("Error reading from pipe: %v", err)
		return
	}

	<-write2Done

	if writeErr != nil {
		t.Errorf("Expected no error, got %v", writeErr)
	}

}
