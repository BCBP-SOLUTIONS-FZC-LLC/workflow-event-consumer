package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Target struct {
	Name  string
	addr  string
	token string
	http  *http.Client
}

func NewTarget(name, addr, token string, timeout time.Duration) *Target {
	return &Target{
		Name:  name,
		addr:  strings.TrimRight(addr, "/"),
		token: token,
		http:  &http.Client{Timeout: timeout},
	}
}

func (t *Target) forward(ctx context.Context, eventType string, body []byte) error {
	path := "/internal/events"
	if c, ok := categoryFor(eventType); ok {
		path = "/events/" + c
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.addr+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-internal-token", t.token)

	resp, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", t.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: status %d", t.Name, resp.StatusCode)
	}
	return nil
}
