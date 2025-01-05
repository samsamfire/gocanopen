package node

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/samsamfire/gocanopen/pkg/nmt"
)

// [NodeProcessor] is responsible for handling the node
// internal CANopen stack processing.
type NodeProcessor struct {
	logger       *slog.Logger
	node         Node
	cancel       context.CancelFunc
	resetHandler func(node Node, cmd uint8) error
	wg           *sync.WaitGroup
}

func NewNodeProcessor(n Node, logger *slog.Logger) *NodeProcessor {

	if logger == nil {
		logger = slog.Default()
	}

	return &NodeProcessor{logger: logger.With("service", "[CTRLR]", "id", n.GetID()), node: n, wg: &sync.WaitGroup{}}
}

// background processing for [SYNC],[TPDO],[RPDO] services
func (c *NodeProcessor) background(ctx context.Context) {

	const PeriodUs = 10_000
	ticker := time.NewTicker(PeriodUs * time.Microsecond)
	c.logger.Info("starting node background process")
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("exited node background process")
			ticker.Stop()
			return
		case <-ticker.C:
			syncWas := c.node.ProcessSYNC(PeriodUs)
			c.node.ProcessPDO(syncWas, PeriodUs)
		}
	}
}

// Main node processing
func (c *NodeProcessor) main(ctx context.Context) {

	const PeriodUs = 1_000
	ticker := time.NewTicker(PeriodUs * time.Microsecond)
	c.logger.Info("starting node main process")
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("exited node main process")
			ticker.Stop()
			return
		case <-ticker.C:
			// Process main
			state := c.node.ProcessMain(false, PeriodUs, nil)
			if state == nmt.ResetApp || state == nmt.ResetComm {
				c.logger.Info("node reset requested")
				if c.resetHandler != nil {
					err := c.resetHandler(c.node, state)
					if err != nil {
						c.logger.Info("failed to reset node", "error", err)
					}
				} else {
					c.logger.Warn("no reset handler registered")
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

	for _, server := range c.node.Servers() {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			server.Process(ctx)
		}()
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.node.LSSSlave().Process(ctx)
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
