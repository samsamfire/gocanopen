package virtual

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	canopen "github.com/samsamfire/gocanopen"
	can "github.com/samsamfire/gocanopen/pkg/can"
)

// Virtual CAN bus implementation with TCP primarily used for testing
// This needs a broker server to send CAN frames to all connected clients
// More information : https://github.com/windelbouwman/virtualcan

func init() {
	can.RegisterInterface("virtual", NewVirtualCanBus)
	can.RegisterInterface("virtualcan", NewVirtualCanBus)
}

type Bus struct {
	logger        *slog.Logger
	mu            sync.Mutex
	channel       string
	conn          net.Conn
	receiveOwn    bool
	framehandler  canopen.FrameListener
	stopChan      chan bool
	wg            sync.WaitGroup
	isRunning     bool
	errSubscriber bool
}

func NewVirtualCanBus(channel string) (canopen.Bus, error) {
	return &Bus{channel: channel, stopChan: make(chan bool), isRunning: false}, nil
}

// Helper function for serializing a CAN frame into the expected binary format
func serializeFrame(frame canopen.Frame) ([]byte, error) {
	buffer := new(bytes.Buffer)
	err := binary.Write(buffer, binary.BigEndian, frame)
	if err != nil {
		return nil, err
	}
	dataBytes := buffer.Bytes()
	frameBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(frameBytes, uint32(len(dataBytes)))
	frameBytes = append(frameBytes, dataBytes...)
	return frameBytes, nil
}

// Helper function for deserializing a CAN frame from expected binary format
func deserializeFrame(buffer []byte) (*canopen.Frame, error) {
	var frame canopen.Frame
	buf := bytes.NewBuffer(buffer)
	err := binary.Read(buf, binary.BigEndian, &frame)
	if err != nil {
		return nil, err
	}
	return &frame, nil
}

// "Connect" to server e.g. localhost:18000
func (b *Bus) Connect(...any) error {
	conn, err := net.Dial("tcp", b.channel)
	if err != nil {
		return err
	}
	b.conn = conn
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		err := tcpConn.SetNoDelay(true)
		if err != nil {
			return err
		}
	}
	return nil
}

// "Disconnect" from server
func (b *Bus) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.errSubscriber && b.isRunning {
		b.stopChan <- true
		b.wg.Wait()
	}
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

// "Send" implementation of Bus interface
func (b *Bus) Send(frame canopen.Frame) error {
	// Local loopback
	if b.receiveOwn && b.framehandler != nil {
		b.framehandler.Handle(frame)
	} else if b.conn == nil {
		return errors.New("error : no active connection, abort send")
	}
	if b.conn != nil {
		frameBytes, err := serializeFrame(frame)
		if err != nil {
			return err
		}
		_ = b.conn.SetWriteDeadline(time.Now().Add(10 * time.Millisecond))
		_, err = b.conn.Write(frameBytes)
		return err
	}
	return nil
}

// "Subscribe" implementation of Bus interface
func (b *Bus) Subscribe(framehandler canopen.FrameListener) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.framehandler = framehandler
	if b.isRunning {
		return nil
	}
	// Start go routine that receives incoming traffic and passes it to frameHandler
	b.wg.Add(1)
	b.isRunning = true
	b.errSubscriber = false
	go b.handleReception()
	return nil
}

// Receive new CAN message
func (b *Bus) Recv() (*canopen.Frame, error) {
	if b.conn == nil {
		return nil, fmt.Errorf("error : no active connection, abort receive")
	}
	_ = b.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	headerBytes := make([]byte, 4)
	n, err := b.conn.Read(headerBytes)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return nil, err
	}
	if n < 4 || err != nil {
		return nil, fmt.Errorf("error deserializing : expected %v, got %v, err : %v", 4, n, err)
	}
	length := binary.BigEndian.Uint32(headerBytes)
	frameBytes := make([]byte, length)
	_ = b.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	n, err = b.conn.Read(frameBytes)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return nil, err
	}
	if n != int(length) || err != nil {
		return nil, fmt.Errorf("error deserializing : expected %v, got %v", length, n)
	}
	frame, err := deserializeFrame(frameBytes)
	if err != nil {
		return nil, err
	}
	return frame, err
}

// Handle incoming traffic
func (client *Bus) handleReception() {
	defer func() {
		client.isRunning = false
		client.wg.Done()
	}()
	for {
		select {
		case <-client.stopChan:
			return
		default:
			// Avoid blocking if lock is already taken (in particular for disconnect, subscribe, etc)
			success := client.mu.TryLock()
			if !success {
				break
			}
			frame, err := client.Recv()
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// No message received, this is OK
			} else if err != nil {
				client.logger.Error("listening routine has closed because", "err", err)
				client.errSubscriber = true
				client.mu.Unlock()
				return
			} else if client.framehandler != nil {
				client.framehandler.Handle(*frame)
			}
			client.mu.Unlock()
		}
	}
}

func (b *Bus) SetReceiveOwn(receiveOwn bool) {
	b.receiveOwn = receiveOwn
}
