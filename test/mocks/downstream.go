package mocks

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"
)

type Downstream struct {
	Server *httptest.Server

	mu       sync.Mutex
	received []events.Envelope[json.RawMessage]
	headers  []http.Header
	bodies   [][]byte
	paths    []string
	status   int
}

func NewDownstream() *Downstream {
	d := &Downstream{status: http.StatusOK}
	d.Server = httptest.NewServer(http.HandlerFunc(d.handle))
	return d
}

func (d *Downstream) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var env events.Envelope[json.RawMessage]
	_ = json.Unmarshal(body, &env)

	d.mu.Lock()
	d.received = append(d.received, env)
	d.headers = append(d.headers, r.Header.Clone())
	d.bodies = append(d.bodies, body)
	d.paths = append(d.paths, r.URL.Path)
	status := d.status
	d.mu.Unlock()

	w.WriteHeader(status)
}

func (d *Downstream) SetStatus(code int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = code
}

func (d *Downstream) URL() string {
	return d.Server.URL
}

func (d *Downstream) Close() {
	d.Server.Close()
}

func (d *Downstream) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.received)
}

func (d *Downstream) Received() []events.Envelope[json.RawMessage] {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]events.Envelope[json.RawMessage], len(d.received))
	copy(out, d.received)
	return out
}

func (d *Downstream) LastHeaders() http.Header {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.headers) == 0 {
		return nil
	}
	return d.headers[len(d.headers)-1].Clone()
}

func (d *Downstream) LastPath() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.paths) == 0 {
		return ""
	}
	return d.paths[len(d.paths)-1]
}

func (d *Downstream) Bodies() [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([][]byte, len(d.bodies))
	copy(out, d.bodies)
	return out
}
