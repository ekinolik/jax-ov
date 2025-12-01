package websocket

import (
	"context"
	"fmt"
	"log"

	massivews "github.com/massive-com/client-go/v2/websocket"
	"github.com/massive-com/client-go/v2/websocket/models"
)

// Client wraps the massive.com WebSocket client
type Client struct {
	client *massivews.Client
}

// NewClient creates a new WebSocket client
func NewClient(apiKey string) (*Client, error) {
	c, err := massivews.New(massivews.Config{
		APIKey: apiKey,
		Feed:   massivews.RealTime,
		Market: massivews.Options,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create WebSocket client: %w", err)
	}

	return &Client{
		client: c,
	}, nil
}

// Connect establishes the WebSocket connection
func (c *Client) Connect() error {
	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	return nil
}

// Close closes the WebSocket connection
func (c *Client) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// Subscribe subscribes to options aggregates per second for the given ticker(s)
// ticker can be a specific option contract (e.g., "O:AAPL230616C00150000")
// or a wildcard pattern (e.g., "*" for all options, or "O:AAPL*" for all AAPL options)
func (c *Client) Subscribe(ticker string) error {
	if err := c.client.Subscribe(massivews.OptionsSecAggs, ticker); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	return nil
}

// Run starts listening for messages and calls the handler function for each message
func (c *Client) Run(ctx context.Context, handler func(models.EquityAgg)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-c.client.Error():
			if err != nil {
				return fmt.Errorf("WebSocket error: %w", err)
			}
		case out, more := <-c.client.Output():
			if !more {
				return fmt.Errorf("output channel closed")
			}

			switch msg := out.(type) {
			case models.EquityAgg:
				handler(msg)
			default:
				log.Printf("Received unexpected message type: %T", out)
			}
		}
	}
}
