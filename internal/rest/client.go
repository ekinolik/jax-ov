package rest

import (
	"context"
	"fmt"
	"time"

	massiverest "github.com/massive-com/client-go/v2/rest"
	"github.com/massive-com/client-go/v2/rest/models"
)

// Millis is a type alias for time.Time representing Unix milliseconds
type Millis = models.Millis

// Date is a type alias for time.Time representing a date
type Date = models.Date

// Client wraps the massive.com REST client
type Client struct {
	client *massiverest.Client
}

// NewClient creates a new REST API client
func NewClient(apiKey string) *Client {
	return &Client{
		client: massiverest.New(apiKey),
	}
}

// OptionContract represents an options contract
type OptionContract struct {
	Ticker          string
	ContractType    string // "call" or "put"
	ExerciseStyle   string
	ExpirationDate  string
	StrikePrice     float64
	UnderlyingTicker string
}

// Aggregate represents a per-second aggregate matching the websocket format
type Aggregate struct {
	EventType         string  `json:"ev"` // "A" for aggregate
	Symbol            string  `json:"sym"`
	Volume            int64   `json:"v"`
	AccumulatedVolume int64   `json:"av"`
	OfficialOpenPrice float64 `json:"op"`
	VWAP              float64 `json:"vw"`
	Open              float64 `json:"o"`
	High              float64 `json:"h"`
	Low               float64 `json:"l"`
	Close             float64 `json:"c"`
	AggregateVWAP     float64 `json:"a"`
	AverageSize       int64   `json:"z"`
	StartTimestamp    int64   `json:"s"` // Unix milliseconds
	EndTimestamp      int64   `json:"e"` // Unix milliseconds
}

// ListOptionContracts fetches all option contracts for an underlying ticker
func (c *Client) ListOptionContracts(ctx context.Context, underlyingTicker string) ([]OptionContract, error) {
	params := models.ListOptionsContractsParams{}.
		WithUnderlyingTicker(models.EQ, underlyingTicker).
		WithLimit(1000)

	var contracts []OptionContract
	iter := c.client.ListOptionsContracts(ctx, params)
	
	for iter.Next() {
		contract := iter.Item()
		expDate := time.Time(contract.ExpirationDate).Format("2006-01-02")
		contracts = append(contracts, OptionContract{
			Ticker:           contract.Ticker,
			ContractType:     contract.ContractType,
			ExerciseStyle:    contract.ExerciseStyle,
			ExpirationDate:   expDate,
			StrikePrice:      contract.StrikePrice,
			UnderlyingTicker: contract.UnderlyingTicker,
		})
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("error listing option contracts: %w", err)
	}

	return contracts, nil
}

// GetOptionAggregates fetches per-second aggregates for an option contract on a specific date
func (c *Client) GetOptionAggregates(ctx context.Context, contractTicker string, date time.Time) ([]Aggregate, error) {
	// Calculate start and end of trading day (9:30 AM - 4:00 PM ET)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone: %w", err)
	}

	// Start: 9:30 AM ET on the specified date
	start := time.Date(date.Year(), date.Month(), date.Day(), 9, 30, 0, 0, loc)
	// End: 4:00 PM ET on the specified date
	end := time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, loc)

	limit := 50000
	adjusted := false
	order := models.Asc
	params := models.ListAggsParams{
		Ticker:     contractTicker,
		Multiplier:  1,
		Timespan:   models.Second,
		From:       models.Millis(start),
		To:         models.Millis(end),
		Order:      &order,
		Limit:      &limit,
		Adjusted:   &adjusted,
	}

	var aggregates []Aggregate
	var accumulatedVolume int64
	iter := c.client.ListAggs(ctx, &params)

	for iter.Next() {
		agg := iter.Item()
		volume := int64(agg.Volume)
		accumulatedVolume += volume
		
		// Calculate average size: if transactions > 0, use volume/transactions, otherwise use volume
		var avgSize int64
		if agg.Transactions > 0 {
			avgSize = volume / agg.Transactions
		} else {
			avgSize = volume
		}

		timestamp := int64(time.Time(agg.Timestamp).UnixMilli())
		aggregates = append(aggregates, Aggregate{
			EventType:         "A",
			Symbol:            contractTicker,
			Volume:            volume,
			AccumulatedVolume: accumulatedVolume,
			OfficialOpenPrice: agg.Open, // Use Open as official open (REST API doesn't provide separate field)
			VWAP:              agg.VWAP,
			Open:              agg.Open,
			High:              agg.High,
			Low:               agg.Low,
			Close:             agg.Close,
			AggregateVWAP:     agg.VWAP,
			AverageSize:       avgSize,
			StartTimestamp:    timestamp,
			EndTimestamp:      timestamp + 1000, // 1 second later
		})
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("error fetching aggregates for %s: %w", contractTicker, err)
	}

	return aggregates, nil
}

