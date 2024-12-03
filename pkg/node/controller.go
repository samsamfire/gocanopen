package node

import (
	"context"
	"sync"
	"time"

	"github.com/samsamfire/gocanopen/pkg/nmt"
	log "github.com/sirupsen/logrus"
)

// [NodeProcessor] is responsible for handling the node
// internal CANopen stack processing.
type NodeProcessor struct {
	node         Node
	cancel       context.CancelFunc
	resetHandler func(node Node, cmd uint8) error
	wg           *sync.WaitGroup
}

func NewNodeController(n Node) *NodeProcessor {
	return &NodeProcessor{node: n, wg: &sync.WaitGroup{}}
}

// background processing for [SYNC],[TPDO],[RPDO] services
func (c *NodeProcessor) background(ctx context.Context) {

	const PeriodUs = 10_000
	ticker := time.NewTicker(PeriodUs * time.Microsecond)
	log.Infof("[NETWORK][x%x] starting node background process", c.node.GetID())
	for {
		select {
		case <-ctx.Done():
			log.Infof("[NETWORK][x%x] exited node background process", c.node.GetID())
			ticker.Stop()
			return
		case <-ticker.C:
			syncWas := c.node.ProcessSYNC(PeriodUs, nil)
			c.node.ProcessTPDO(syncWas, PeriodUs, nil)
			c.node.ProcessRPDO(syncWas, PeriodUs, nil)
		}
	}
}

// Main node processing
func (c *NodeProcessor) main(ctx context.Context) {

	const PeriodUs = 1_000
	ticker := time.NewTicker(PeriodUs * time.Microsecond)
	log.Infof("[NETWORK][x%x] starting node main process", c.node.GetID())
	for {
		select {
		case <-ctx.Done():
			log.Infof("[NETWORK][x%x] exited node main process", c.node.GetID())
			ticker.Stop()
			return
		case <-ticker.C:
			// Process main
			state := c.node.ProcessMain(false, PeriodUs, nil)
			if state == nmt.ResetApp || state == nmt.ResetComm {
				if c.resetHandler != nil {
					err := c.resetHandler(c.node, state)
					if err != nil {
						log.Warn("failed to reset node")
					}
				} else {
					log.Warn("no reset handler for node")
				}
			}
		}
	}

}

// Start node processing, this will be run inside of a go routine
// Call Stop() to stop processing or cancel the context
// Call Wait() to wait for end of execution
func (c *NodeProcessor) Start(ctx context.Context) error {

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.background(ctx)
	}()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.main(ctx)
	}()
	return nil
}

// Stop node processing i.e. stop all tasks
// Wait should be called in order to make sure that all routines have been stopped
func (c *NodeProcessor) Stop() error {
	// Cancel any on-going tasks (background, main loop)
	// And wait for them to finish
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Wait for processing to finish (blocking)
func (c *NodeProcessor) Wait() error {
	c.wg.Wait()
	return nil
}

// Add a specific handler to be called on reset events
func (c *NodeProcessor) AddResetHandler(handler func(node Node, cmd uint8) error) {
	c.resetHandler = handler
}

// Get underlying [Node] object
func (c *NodeProcessor) GetNode() Node {
	return c.node
}
