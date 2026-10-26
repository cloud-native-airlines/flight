// Package ops is a small client for reporting a leg's lifecycle events
// (departure, arrival) to the Operations service. Like the ADS-B client, it uses
// a bounded deadline and limited retries; a failure is returned (and logged by
// the caller) but never changes the flight's physical state. Operations dedups on
// (run_id, event_id), so retries with the same stable event_id are idempotent.
package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Event matches the Operations POST /api/v1/runs/{runId}/events contract.
type Event struct {
	EventID          string `json:"event_id"`
	FlightInstanceID string `json:"flight_instance_id"`
	AircraftID       string `json:"aircraft_id"`
	Type             string `json:"type"` // "departure" | "arrival"
	OccurredAt       string `json:"occurred_at"`
}

type Client struct {
	baseURL          string
	flightInstanceID string
	http             *http.Client
	maxRetries       int
}

// New returns a client bound to one flight instance. timeout bounds each attempt;
// maxRetries is the number of additional attempts after the first.
func New(baseURL, flightInstanceID string, timeout time.Duration, maxRetries int) *Client {
	return &Client{
		baseURL:          baseURL,
		flightInstanceID: flightInstanceID,
		http:             &http.Client{Timeout: timeout},
		maxRetries:       maxRetries,
	}
}

// Emit records a lifecycle event for the given run. The event_id is derived from
// run, instance, and type so retries (and re-launches of the same leg) collapse
// to one event in Operations.
func (c *Client) Emit(ctx context.Context, runID, eventType, aircraftID string, occurredAt time.Time) error {
	event := Event{
		EventID:          fmt.Sprintf("%s-%s-%s", runID, c.flightInstanceID, eventType),
		FlightInstanceID: c.flightInstanceID,
		AircraftID:       aircraftID,
		Type:             eventType,
		OccurredAt:       occurredAt.UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	url := fmt.Sprintf("%s/api/v1/runs/%s/events", c.baseURL, runID)

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
		}
		lastErr = c.attempt(ctx, url, body)
		if lastErr == nil {
			return nil
		}
		if permanent(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func (c *Client) attempt(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err // transient (network); will retry
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 500:
		return &httpError{status: resp.StatusCode, perm: false}
	default: // 4xx — a bad/conflicting request won't succeed on retry
		return &httpError{status: resp.StatusCode, perm: true}
	}
}

type httpError struct {
	status int
	perm   bool
}

func (e *httpError) Error() string { return fmt.Sprintf("operations responded %d", e.status) }

func permanent(err error) bool {
	he, ok := err.(*httpError)
	return ok && he.perm
}
