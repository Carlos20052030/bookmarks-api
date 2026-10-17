package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Event is the payload sent to the webhook-dispatcher.
type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	UserID     string         `json:"user_id"`
	OccurredAt time.Time      `json:"occurred_at"`
	Data       map[string]any `json:"data"`
}

// Client sends events asynchronously so the main API request is never blocked.
type Client struct {
	url    string
	token  string
	events chan Event
	http   *http.Client
}

// NewClient starts the background workers. Caller must call Close when shutting down.
func NewClient(url, token string) *Client {
	c := &Client{
		url:    url,
		token:  token,
		events: make(chan Event, 256), // Bounded channel as per scope.
		http: &http.Client{
			Timeout: 2 * time.Second, // Short timeout. Fire-and-forget.
		},
	}

	// Start 2 workers.
	for i := 0; i < 2; i++ {
		go c.worker()
	}

	return c
}

// Publish attempts to enqueue an event. If the channel is full, it drops
// the event and logs a warning. It never blocks the caller.
func (c *Client) Publish(evt Event) {
	select {
	case c.events <- evt:
	default:
		slog.Warn("dispatcher queue full, dropping event", "event_id", evt.ID, "type", evt.Type)
	}
}

func (c *Client) worker() {
	for evt := range c.events {
		c.send(evt)
	}
}

func (c *Client) send(evt Event) {
	body, err := json.Marshal(evt)
	if err != nil {
		slog.Error("failed to marshal event", "err", err, "event_id", evt.ID)
		return
	}

	// Use context.Background() because the HTTP request that triggered this
	// may already be finished and its context canceled.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.url+"/events", bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create dispatcher request", "err", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ingest-Token", c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Error("dispatcher request failed", "err", err, "event_id", evt.ID)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// We log non-2xx responses but don't retry. The dispatcher is fire-and-forget
	// from the Bookmarks API's perspective.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("dispatcher rejected event", "status", resp.StatusCode, "event_id", evt.ID)
	}
}

// Close stops the workers gracefully.
func (c *Client) Close() {
	close(c.events)
}
