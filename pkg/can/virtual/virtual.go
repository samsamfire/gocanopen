package virtual

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	can "github.com/samsamfire/gocanopen/pkg/can"
	log "github.com/sirupsen/logrus"
)

// Virtual CAN bus implementation with TCP primarily used for testing
// This needs a broker server to send CAN frames to all connected clients
// More information : https://github.com/windelbouwman/virtualcan

func init() {
	can.RegisterInterface("virtual", NewVirtualCanBus)
}

// Helper function for serializing a CAN frame into the expected binary format
func serializeFrame(frame can.Frame) ([]byte, error) {
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
func deserializeFrame(buffer []byte) (*can.Frame, error) {
	var frame can.Frame
	buf := bytes.NewBuffer(buffer)
	err := binary.Read(buf, binary.BigEndian, &frame)
	if err != nil {
		return nil, err
	}
	return &frame, nil
}

type VirtualCanBus struct {
	mu            sync.Mutex
	channel       string
	conn          net.Conn
	receiveOwn    bool
	framehandler  can.FrameListener
	stopChan      chan bool
	wg            sync.WaitGroup
	isRunning     bool
	errSubscriber bool
}

// "Connect" to server e.g. localhost:18000
func (client *VirtualCanBus) Connect(...any) error {
	conn, err := net.Dial("tcp", client.channel)
	if err != nil {
		return err
	}
	client.conn = conn
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		err := tcpConn.SetNoDelay(true)
		if err != nil {
			return err
		}
	}
	return nil
}

// "Disconnect" from server
func (client *VirtualCanBus) Disconnect() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if !client.errSubscriber && client.isRunning {
		client.stopChan <- true
		client.wg.Wait()
	}
	if client.conn != nil {
		return client.conn.Close()
	}
	return nil
}

// "Send" implementation of Bus interface
func (client *VirtualCanBus) Send(frame can.Frame) error {
	// Local loopback
	if client.receiveOwn && client.framehandler != nil {
		client.framehandler.Handle(frame)
	} else if client.conn == nil {
		return errors.New("error : no active connection, abort send")
	}
	if client.conn != nil {
		frameBytes, err := serializeFrame(frame)
		if err != nil {
			return err
		}
		_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Millisecond))
		_, err = client.conn.Write(frameBytes)
		return err
	}
	return nil
}

// "Subscribe" implementation of Bus interface
func (client *VirtualCanBus) Subscribe(framehandler can.FrameListener) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.framehandler = framehandler
	if client.isRunning {
		return nil
	}
	// Start go routine that receives incoming traffic and passes it to frameHandler
	client.wg.Add(1)
	client.isRunning = true
	client.errSubscriber = false
	go client.handleReception()
	return nil
}

// Receive new CAN message
func (client *VirtualCanBus) Recv() (*can.Frame, error) {
	if client.conn == nil {
		return nil, fmt.Errorf("error : no active connection, abort receive")
	}
	_ = client.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	headerBytes := make([]byte, 4)
	n, err := client.conn.Read(headerBytes)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return nil, err
	}
	if n < 4 || err != nil {
		return nil, fmt.Errorf("error deserializing : expected %v, got %v, err : %v", 4, n, err)
	}
	length := binary.BigEndian.Uint32(headerBytes)
	frameBytes := make([]byte, length)
	_ = client.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	n, err = client.conn.Read(frameBytes)
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
func (client *VirtualCanBus) handleReception() {
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
				log.Errorf("[VIRTUAL DRIVER] listening routine has closed because : %v", err)
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

func (client *VirtualCanBus) SetReceiveOwn(receiveOwn bool) {
	client.receiveOwn = receiveOwn
}

func NewVirtualCanBus(channel string) (can.Bus, error) {
	return &VirtualCanBus{channel: channel, stopChan: make(chan bool), isRunning: false}, nil
}
