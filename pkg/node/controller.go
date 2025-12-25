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
	period       time.Duration
}

func NewNodeProcessor(n Node, logger *slog.Logger, processingPeriod time.Duration) *NodeProcessor {

	if logger == nil {
		logger = slog.Default()
	}

	return &NodeProcessor{
		logger: logger.With("service", "[CTRLR]", "id", n.GetID()),
		node:   n,
		wg:     &sync.WaitGroup{},
		period: processingPeriod,
	}
}

// background processing for [SYNC],[TPDO],[RPDO] services
func (c *NodeProcessor) background(ctx context.Context) {

	ticker := time.NewTicker(c.period)
	periodUs := uint32(c.period.Microseconds())
	c.logger.Info("starting node background process")
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("exited node background process")
			ticker.Stop()
			return
		case <-ticker.C:
			syncWas := c.node.ProcessSYNC(periodUs)
			c.node.ProcessPDO(syncWas, periodUs)
		}
	}
}

// Main node processing
func (c *NodeProcessor) main(ctx context.Context) {

	ticker := time.NewTicker(c.period)
	periodUs := uint32(c.period.Microseconds())
	c.logger.Info("starting node main process")
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("exited node main process")
			ticker.Stop()
			return
		case <-ticker.C:
			// Process main
			state := c.node.ProcessMain(false, periodUs)
			if state == nmt.ResetComm {
				// Currently nothing specific is done here.
				// We could in the future "recreate" the node here.
				break
			}
			if state == nmt.ResetApp {
				c.logger.Info("reset has been requested")
				if c.resetHandler != nil {
					// Custom logic to apply
					c.logger.Info("executing custom reset handler")
					err := c.resetHandler(c.node, state)
					if err != nil {
						c.logger.Error("error occured executing custom reset handler", "err", err)
					}
				}
				// Do simple NMT boot up
				// TODO : we should re-create the node here for a fresh start (in particular)
				// Currently node Reset only restarts NMT part
				err := c.node.Reset()
				if err != nil {
					c.logger.Info("error occured during reset", "err", err)
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
// after this handler is called, node will reboot automatically
func (c *NodeProcessor) AddResetHandler(handler func(node Node, cmd uint8) error) {
	c.resetHandler = handler
}

// Get underlying [Node] object
func (c *NodeProcessor) GetNode() Node {
	return c.node
}
