package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

const defaultPathPrefix = "/api/v1/internal/events"

type Target struct {
	Name       string
	addr       string
	pathPrefix string
	token      string
	http       *http.Client
}

func NewTarget(name, addr, pathPrefix, token string, timeout time.Duration) *Target {
	if pathPrefix == "" {
		pathPrefix = defaultPathPrefix
	}
	return &Target{
		Name:       name,
		addr:       strings.TrimRight(addr, "/"),
		pathPrefix: strings.TrimRight(pathPrefix, "/"),
		token:      token,
		http:       &http.Client{Timeout: timeout},
	}
}

func (t *Target) forward(ctx context.Context, eventType string, body []byte) error {
	path := t.pathPrefix
	if t.Name == executionTargetName {
		if c, ok := categoryFor(eventType); ok {
			path = t.pathPrefix + "/" + c
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.addr+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-internal-token", t.token)

	gincommon.InjectTraceContext(ctx, req.Header)

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
