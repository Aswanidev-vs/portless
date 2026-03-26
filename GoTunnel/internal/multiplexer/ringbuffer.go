package multiplexer

import (
	"errors"
	"io"
	"sync"
)

// RingBuffer implements a circular buffer for stream data
type RingBuffer struct {
	buf    []byte
	size   int
	head   int
	tail   int
	count  int
	mu     sync.RWMutex
	closed bool
}

// NewRingBuffer creates a new ring buffer with the given size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// Write writes data to the ring buffer
func (rb *RingBuffer) Write(data []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return 0, errors.New("buffer closed")
	}

	n := 0
	for i := 0; i < len(data); i++ {
		if rb.count == rb.size {
			// Buffer full, overwrite oldest data
			rb.head = (rb.head + 1) % rb.size
			rb.count--
		}

		rb.buf[rb.tail] = data[i]
		rb.tail = (rb.tail + 1) % rb.size
		rb.count++
		n++
	}

	return n, nil
}

// Read reads data from the ring buffer
func (rb *RingBuffer) Read(data []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		if rb.closed {
			return 0, io.EOF
		}
		return 0, nil
	}

	n := 0
	for i := 0; i < len(data) && rb.count > 0; i++ {
		data[i] = rb.buf[rb.head]
		rb.head = (rb.head + 1) % rb.size
		rb.count--
		n++
	}

	return n, nil
}

// Len returns the number of bytes available for reading
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Cap returns the total capacity of the buffer
func (rb *RingBuffer) Cap() int {
	return rb.size
}

// Available returns the number of bytes available for writing
func (rb *RingBuffer) Available() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size - rb.count
}

// Close closes the buffer
func (rb *RingBuffer) Close() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	return nil
}

// Reset resets the buffer
func (rb *RingBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.head = 0
	rb.tail = 0
	rb.count = 0
	rb.closed = false
}

// Peek returns data without removing it from the buffer
func (rb *RingBuffer) Peek(data []byte) (int, error) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return 0, nil
	}

	n := 0
	head := rb.head
	for i := 0; i < len(data) && n < rb.count; i++ {
		data[i] = rb.buf[head]
		head = (head + 1) % rb.size
		n++
	}

	return n, nil
}
