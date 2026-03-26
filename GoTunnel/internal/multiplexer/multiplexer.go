package multiplexer

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// StreamState represents the state of a stream
type StreamState string

const (
	StreamStateIdle    StreamState = "idle"
	StreamStateOpen    StreamState = "open"
	StreamStateClosing StreamState = "closing"
	StreamStateClosed  StreamState = "closed"
	StreamStateError   StreamState = "error"
)

// DeliverySemantics defines message delivery semantics
type DeliverySemantics string

const (
	DeliveryAtMostOnce  DeliverySemantics = "at_most_once"
	DeliveryAtLeastOnce DeliverySemantics = "at_least_once"
	DeliveryExactlyOnce DeliverySemantics = "exactly_once"
)

// Frame represents a multiplexed frame
type Frame struct {
	StreamID    uint32
	Type        FrameType
	Flags       uint8
	Payload     []byte
	Timestamp   time.Time
	SequenceNum uint64
}

// FrameType represents the type of frame
type FrameType uint8

const (
	FrameTypeData         FrameType = 0x00
	FrameTypeHeader       FrameType = 0x01
	FrameTypeSettings     FrameType = 0x02
	FrameTypePing         FrameType = 0x03
	FrameTypePong         FrameType = 0x04
	FrameTypeGoAway       FrameType = 0x05
	FrameTypeReset        FrameType = 0x06
	FrameTypeWindowUpdate FrameType = 0x07
	FrameTypeAck          FrameType = 0x08
	FrameTypeNack         FrameType = 0x09
)

// Frame flags
const (
	FlagEndStream  uint8 = 0x01
	FlagAck        uint8 = 0x02
	FlagCompressed uint8 = 0x04
)

// Stream represents a multiplexed stream
type Stream struct {
	ID           uint32
	State        StreamState
	ReadBuffer   *RingBuffer
	WriteBuffer  *RingBuffer
	WindowSize   uint32
	UsedWindow   uint32
	SequenceNum  uint64
	AckNum       uint64
	SentFrames   map[uint64]Frame
	Pending      map[uint64]Frame
	Delivered    map[uint64]struct{}
	CreatedAt    time.Time
	LastActivity time.Time
	Error        error
	mu           sync.RWMutex
}

// Connection represents a multiplexed connection
type Connection struct {
	conn          io.ReadWriteCloser
	streams       map[uint32]*Stream
	maxStreams    uint32
	nextStreamID  uint32
	windowSize    uint32
	usedWindow    uint32
	deliveryMode  DeliverySemantics
	mu            sync.RWMutex
	stopCh        chan struct{}
	closeOnce     sync.Once
	wg            sync.WaitGroup
	onStreamOpen  func(stream *Stream)
	onStreamClose func(stream *Stream, err error)
	onData        func(stream *Stream, data []byte)
}

// ConnectionConfig holds connection configuration
type ConnectionConfig struct {
	MaxStreams    uint32
	WindowSize    uint32
	DeliveryMode  DeliverySemantics
	OnStreamOpen  func(stream *Stream)
	OnStreamClose func(stream *Stream, err error)
	OnData        func(stream *Stream, data []byte)
}

// NewConnection creates a new multiplexed connection
func NewConnection(conn io.ReadWriteCloser, cfg ConnectionConfig) *Connection {
	if cfg.MaxStreams == 0 {
		cfg.MaxStreams = 1000
	}
	if cfg.WindowSize == 0 {
		cfg.WindowSize = 65535
	}
	if cfg.DeliveryMode == "" {
		cfg.DeliveryMode = DeliveryAtLeastOnce
	}

	return &Connection{
		conn:          conn,
		streams:       make(map[uint32]*Stream),
		maxStreams:    cfg.MaxStreams,
		nextStreamID:  1,
		windowSize:    cfg.WindowSize,
		deliveryMode:  cfg.DeliveryMode,
		stopCh:        make(chan struct{}),
		onStreamOpen:  cfg.OnStreamOpen,
		onStreamClose: cfg.OnStreamClose,
		onData:        cfg.OnData,
	}
}

// Start starts the connection
func (c *Connection) Start(ctx context.Context) error {
	c.wg.Add(1)
	go c.readLoop(ctx)

	c.wg.Add(1)
	go c.writeLoop(ctx)

	return nil
}

// Close closes the connection
func (c *Connection) Close() error {
	c.stop()
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, stream := range c.streams {
		stream.mu.Lock()
		stream.State = StreamStateClosed
		stream.mu.Unlock()
	}

	return c.conn.Close()
}

func (c *Connection) stop() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
	})
}

// OpenStream opens a new stream
func (c *Connection) OpenStream(ctx context.Context) (*Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if uint32(len(c.streams)) >= c.maxStreams {
		return nil, fmt.Errorf("max streams reached")
	}

	streamID := c.nextStreamID
	c.nextStreamID += 2 // Client streams are odd, server streams are even

	stream := &Stream{
		ID:           streamID,
		State:        StreamStateOpen,
		ReadBuffer:   NewRingBuffer(int(c.windowSize)),
		WriteBuffer:  NewRingBuffer(int(c.windowSize)),
		WindowSize:   c.windowSize,
		SentFrames:   make(map[uint64]Frame),
		Pending:      make(map[uint64]Frame),
		Delivered:    make(map[uint64]struct{}),
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	c.streams[streamID] = stream

	// Send header frame
	frame := Frame{
		StreamID:  streamID,
		Type:      FrameTypeHeader,
		Flags:     0,
		Payload:   []byte{},
		Timestamp: time.Now(),
	}
	c.writeFrame(frame)

	if c.onStreamOpen != nil {
		c.onStreamOpen(stream)
	}

	return stream, nil
}

// GetStream retrieves a stream by ID
func (c *Connection) GetStream(streamID uint32) (*Stream, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stream, ok := c.streams[streamID]
	if !ok {
		return nil, fmt.Errorf("stream %d not found", streamID)
	}
	return stream, nil
}

// Write writes data to a stream
func (c *Connection) Write(streamID uint32, data []byte) error {
	stream, err := c.GetStream(streamID)
	if err != nil {
		return err
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	if stream.State != StreamStateOpen {
		return fmt.Errorf("stream %d is not open", streamID)
	}

	// Check backpressure
	if stream.UsedWindow+uint32(len(data)) > stream.WindowSize {
		return fmt.Errorf("window size exceeded for stream %d", streamID)
	}

	// Write to buffer
	if _, err := stream.WriteBuffer.Write(data); err != nil {
		return err
	}

	stream.UsedWindow += uint32(len(data))
	stream.SequenceNum++
	stream.LastActivity = time.Now()

	// Send data frame
	frame := Frame{
		StreamID:    streamID,
		Type:        FrameTypeData,
		Flags:       0,
		Payload:     data,
		Timestamp:   time.Now(),
		SequenceNum: stream.SequenceNum,
	}
	stream.SentFrames[frame.SequenceNum] = cloneFrame(frame)

	return c.writeFrame(frame)
}

// Read reads data from a stream
func (c *Connection) Read(streamID uint32, buf []byte) (int, error) {
	stream, err := c.GetStream(streamID)
	if err != nil {
		return 0, err
	}

	stream.mu.RLock()
	state := stream.State
	stream.mu.RUnlock()

	if state == StreamStateClosed || state == StreamStateError {
		return 0, io.EOF
	}

	n, err := stream.ReadBuffer.Read(buf)
	if err == io.EOF && state == StreamStateOpen {
		return n, nil
	}
	return n, err
}

// CloseStream closes a stream
func (c *Connection) CloseStream(streamID uint32) error {
	c.mu.Lock()
	stream, ok := c.streams[streamID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("stream %d not found", streamID)
	}
	delete(c.streams, streamID)
	c.mu.Unlock()

	stream.mu.Lock()
	stream.State = StreamStateClosing
	stream.mu.Unlock()

	// Send reset frame
	frame := Frame{
		StreamID:  streamID,
		Type:      FrameTypeReset,
		Flags:     0,
		Payload:   []byte{},
		Timestamp: time.Now(),
	}
	c.writeFrame(frame)

	stream.mu.Lock()
	stream.State = StreamStateClosed
	stream.mu.Unlock()

	if c.onStreamClose != nil {
		c.onStreamClose(stream, nil)
	}

	return nil
}

func (c *Connection) readLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		default:
			frame, err := c.readFrame()
			if err != nil {
				if err == io.EOF {
					return
				}
				continue
			}
			c.handleFrame(frame)
		}
	}
}

func (c *Connection) writeLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			// Send window updates
			c.sendWindowUpdates()
		}
	}
}

func (c *Connection) handleFrame(frame Frame) {
	switch frame.Type {
	case FrameTypeData:
		c.handleDataFrame(frame)
	case FrameTypeHeader:
		c.handleHeaderFrame(frame)
	case FrameTypeReset:
		c.handleResetFrame(frame)
	case FrameTypeWindowUpdate:
		c.handleWindowUpdateFrame(frame)
	case FrameTypeAck:
		c.handleAckFrame(frame)
	case FrameTypeNack:
		c.handleNackFrame(frame)
	case FrameTypePing:
		c.handlePingFrame(frame)
	case FrameTypeGoAway:
		c.handleGoAwayFrame(frame)
	}
}

func (c *Connection) handleDataFrame(frame Frame) {
	stream, err := c.GetStream(frame.StreamID)
	if err != nil {
		return
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	// Handle delivery semantics
	switch c.deliveryMode {
	case DeliveryAtMostOnce:
		// Just write to buffer
		stream.ReadBuffer.Write(frame.Payload)

	case DeliveryAtLeastOnce:
		// Write and send ack
		stream.ReadBuffer.Write(frame.Payload)
		c.sendAck(frame.StreamID, frame.SequenceNum)

	case DeliveryExactlyOnce:
		if _, seen := stream.Delivered[frame.SequenceNum]; seen {
			c.sendAck(frame.StreamID, frame.SequenceNum)
			return
		}
		stream.Pending[frame.SequenceNum] = cloneFrame(frame)
		if frame.SequenceNum > stream.AckNum+1 {
			c.sendNack(frame.StreamID, stream.AckNum+1)
			return
		}
		for {
			next := stream.AckNum + 1
			pending, ok := stream.Pending[next]
			if !ok {
				break
			}
			stream.ReadBuffer.Write(pending.Payload)
			delete(stream.Pending, next)
			stream.Delivered[next] = struct{}{}
			stream.AckNum = next
		}
		c.sendAck(frame.StreamID, stream.AckNum)
	}

	// Update window
	payloadSize := uint32(len(frame.Payload))
	if payloadSize >= stream.UsedWindow {
		stream.UsedWindow = 0
	} else {
		stream.UsedWindow -= payloadSize
	}
	stream.LastActivity = time.Now()

	if c.onData != nil {
		c.onData(stream, frame.Payload)
	}
}

func (c *Connection) handleHeaderFrame(frame Frame) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.streams[frame.StreamID]; !exists {
		stream := &Stream{
			ID:           frame.StreamID,
			State:        StreamStateOpen,
			ReadBuffer:   NewRingBuffer(int(c.windowSize)),
			WriteBuffer:  NewRingBuffer(int(c.windowSize)),
			WindowSize:   c.windowSize,
			SentFrames:   make(map[uint64]Frame),
			Pending:      make(map[uint64]Frame),
			Delivered:    make(map[uint64]struct{}),
			CreatedAt:    time.Now(),
			LastActivity: time.Now(),
		}
		c.streams[frame.StreamID] = stream

		if c.onStreamOpen != nil {
			c.onStreamOpen(stream)
		}
	}
}

func (c *Connection) handleResetFrame(frame Frame) {
	stream, err := c.GetStream(frame.StreamID)
	if err != nil {
		return
	}

	stream.mu.Lock()
	stream.State = StreamStateClosed
	stream.mu.Unlock()

	if c.onStreamClose != nil {
		c.onStreamClose(stream, nil)
	}
}

func (c *Connection) handleWindowUpdateFrame(frame Frame) {
	stream, err := c.GetStream(frame.StreamID)
	if err != nil {
		return
	}

	stream.mu.Lock()
	increment := binary.BigEndian.Uint32(frame.Payload)
	stream.WindowSize += increment
	stream.mu.Unlock()
}

func (c *Connection) handleAckFrame(frame Frame) {
	stream, err := c.GetStream(frame.StreamID)
	if err != nil {
		return
	}

	stream.mu.Lock()
	ackNum := binary.BigEndian.Uint64(frame.Payload)
	for seq := range stream.SentFrames {
		if seq <= ackNum {
			delete(stream.SentFrames, seq)
		}
	}
	stream.mu.Unlock()
}

func (c *Connection) handleNackFrame(frame Frame) {
	stream, err := c.GetStream(frame.StreamID)
	if err != nil {
		return
	}

	stream.mu.Lock()
	requested := binary.BigEndian.Uint64(frame.Payload)
	frames := make([]Frame, 0, len(stream.SentFrames))
	for seq, sent := range stream.SentFrames {
		if seq >= requested {
			frames = append(frames, cloneFrame(sent))
		}
	}
	stream.mu.Unlock()
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].SequenceNum < frames[j].SequenceNum
	})
	for _, resend := range frames {
		_ = c.writeFrame(resend)
	}
}

func (c *Connection) handlePingFrame(frame Frame) {
	// Send pong
	pong := Frame{
		StreamID:  0,
		Type:      FrameTypePong,
		Flags:     0,
		Payload:   frame.Payload,
		Timestamp: time.Now(),
	}
	c.writeFrame(pong)
}

func (c *Connection) handleGoAwayFrame(frame Frame) {
	// Connection closing
	c.stop()
	_ = c.conn.Close()
}

func (c *Connection) sendAck(streamID uint32, sequenceNum uint64) {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, sequenceNum)

	frame := Frame{
		StreamID:  streamID,
		Type:      FrameTypeAck,
		Flags:     0,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	c.writeFrame(frame)
}

func (c *Connection) sendNack(streamID uint32, requestedSeq uint64) {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, requestedSeq)

	frame := Frame{
		StreamID:  streamID,
		Type:      FrameTypeNack,
		Flags:     0,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	c.writeFrame(frame)
}

func (c *Connection) sendWindowUpdates() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, stream := range c.streams {
		stream.mu.RLock()
		available := stream.WindowSize - stream.UsedWindow
		stream.mu.RUnlock()

		if available < stream.WindowSize/2 {
			payload := make([]byte, 4)
			binary.BigEndian.PutUint32(payload, stream.WindowSize-available)

			frame := Frame{
				StreamID:  stream.ID,
				Type:      FrameTypeWindowUpdate,
				Flags:     0,
				Payload:   payload,
				Timestamp: time.Now(),
			}
			c.writeFrame(frame)
		}
	}
}

func (c *Connection) readFrame() (Frame, error) {
	// Simplified frame reading - in production, use proper framing
	header := make([]byte, 18)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return Frame{}, err
	}

	frame := Frame{
		StreamID:    binary.BigEndian.Uint32(header[0:4]),
		Type:        FrameType(header[4]),
		Flags:       header[5],
		SequenceNum: binary.BigEndian.Uint64(header[6:14]),
		Timestamp:   time.Now(),
	}

	length := binary.BigEndian.Uint32(header[14:18])
	if length > 0 {
		frame.Payload = make([]byte, length)
		if _, err := io.ReadFull(c.conn, frame.Payload); err != nil {
			return Frame{}, err
		}
	}

	return frame, nil
}

func (c *Connection) writeFrame(frame Frame) error {
	header := make([]byte, 18)
	binary.BigEndian.PutUint32(header[0:4], frame.StreamID)
	header[4] = byte(frame.Type)
	header[5] = frame.Flags
	binary.BigEndian.PutUint64(header[6:14], frame.SequenceNum)
	binary.BigEndian.PutUint32(header[14:18], uint32(len(frame.Payload)))

	if _, err := c.conn.Write(header); err != nil {
		return err
	}

	if len(frame.Payload) > 0 {
		if _, err := c.conn.Write(frame.Payload); err != nil {
			return err
		}
	}

	return nil
}

func cloneFrame(frame Frame) Frame {
	frame.Payload = append([]byte(nil), frame.Payload...)
	return frame
}

// GetStreamCount returns the number of active streams
func (c *Connection) GetStreamCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.streams)
}

// GetStreamStates returns the state of all streams
func (c *Connection) GetStreamStates() map[uint32]StreamState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	states := make(map[uint32]StreamState)
	for id, stream := range c.streams {
		stream.mu.RLock()
		states[id] = stream.State
		stream.mu.RUnlock()
	}
	return states
}
