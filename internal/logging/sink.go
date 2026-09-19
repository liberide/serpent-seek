package logging

import "sync"

// SinkFunc adapts a function to the Sink interface.
type SinkFunc func(Record)

// Write implements Sink.
func (f SinkFunc) Write(r Record) { f(r) }

// BufferSink asynchronously forwards records to a writer, dropping records when
// the buffer is full so logging never blocks the request path.
type BufferSink struct {
	ch     chan Record
	done   chan struct{}
	once   sync.Once
	writer func(Record)
}

// NewBufferSink starts an asynchronous sink with the given buffer size.
func NewBufferSink(size int, writer func(Record)) *BufferSink {
	if size <= 0 {
		size = 1024
	}
	b := &BufferSink{ch: make(chan Record, size), done: make(chan struct{}), writer: writer}
	go b.loop()
	return b
}

func (b *BufferSink) loop() {
	defer close(b.done)
	for r := range b.ch {
		b.writer(r)
	}
}

// Write enqueues a record without blocking.
func (b *BufferSink) Write(r Record) {
	select {
	case b.ch <- r:
	default:
	}
}

// Close flushes and stops the sink.
func (b *BufferSink) Close() {
	b.once.Do(func() { close(b.ch) })
	<-b.done
}
