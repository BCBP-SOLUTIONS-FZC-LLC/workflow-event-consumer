package topology

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

func FuzzLoad_AnAcceptedFileIsAlwaysAValidTopology(f *testing.F) {
	example, err := os.ReadFile("../../config/topology.example.yaml")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(example)
	f.Add([]byte(validYAML))
	f.Add([]byte("version: 1\ntargets: []\n"))
	f.Add([]byte("targets:\n  - name: a\n    addr: x\nqueues:\n  - name: q\n    url: u\nroutes:\n  T: [a, a]\n"))
	f.Add([]byte("targets:\n  - name: execution\n    addr: x\nqueues:\n  - name: q\n    url: u\nroutes:\n  NotCategorised: [execution]\n"))
	f.Add([]byte("route:\n  T: [nobody]\n"))
	f.Add([]byte("{"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		path := filepath.Join(t.TempDir(), "topology.yaml")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			return
		}
		if len(cfg.Targets) == 0 || len(cfg.Queues) == 0 {
			t.Fatalf("accepted a topology with %d targets and %d queues", len(cfg.Targets), len(cfg.Queues))
		}
		declared := map[string]bool{}
		for _, tg := range cfg.Targets {
			if tg.Name == "" || tg.Addr == "" || declared[tg.Name] {
				t.Fatalf("accepted target %+v (names so far %v)", tg, declared)
			}
			declared[tg.Name] = true
		}
		queues := map[string]bool{}
		for _, q := range cfg.Queues {
			if q.Name == "" || q.URL == "" || queues[q.Name] {
				t.Fatalf("accepted queue %+v", q)
			}
			queues[q.Name] = true
		}
		for eventType, names := range cfg.Routes {
			for _, name := range names {
				if !declared[name] {
					t.Fatalf("route %q names undeclared target %q", eventType, name)
				}
				if name == forwarder.ExecutionTarget && !forwarder.Categorised(eventType) {
					t.Fatalf("route %q sends an uncategorised type to the execution target", eventType)
				}
			}
		}
	})
}

func FuzzResolveTargets_AnAddressAcceptedOutsideDevNamesAHostAndAnAllowedScheme(f *testing.F) {
	for _, seed := range []string{
		"https://execution.workflow.svc", "http://execution:8080", "http://execution", "ftp://x:21",
		"https://", "http://:8080", "//host:80", "https://user@host", "HTTP://Host:1", "http://[::1]:8080", " https://x", "https://h?x", "https://h#f", "https://h/p", "http://h:0",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, addr string) {
		cfg := Config{Targets: []Target{{Name: "definition", Addr: addr}}}
		_, err := cfg.ResolveTargets("prod", time.Second)
		if err != nil {
			return
		}
		u, perr := url.Parse(addr)
		if perr != nil {
			t.Fatalf("accepted %q, which does not parse: %v", addr, perr)
		}
		if u.Host == "" || u.Hostname() == "" {
			t.Fatalf("accepted %q with no host", addr)
		}
		if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Port() == "0" {
			t.Fatalf("accepted %q, which carries a path, query, fragment or port 0", addr)
		}
		switch u.Scheme {
		case "https":
		case "http":
			if u.Port() == "" {
				t.Fatalf("accepted http address %q with no port", addr)
			}
		default:
			t.Fatalf("accepted %q with scheme %q", addr, u.Scheme)
		}
	})
}
