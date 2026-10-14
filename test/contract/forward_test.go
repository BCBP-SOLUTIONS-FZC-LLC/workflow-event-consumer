package contract_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability/obstest"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/topology"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/contract"
)

const token = "target-token"

func TestMain(m *testing.M) {
	obstest.Setup()
	os.Exit(m.Run())
}

var specs = map[string]string{
	"definition": "../../api/contracts/definition-internal-openapi.yaml",
	"execution":  "../../api/contracts/execution-internal-openapi.yaml",
}

var permanentStatuses = []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity}

var retriedStatuses = []int{
	http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusUnsupportedMediaType,
}

func envelope(eventType string) []byte {
	return []byte(`{"id":"` + uuid.NewString() + `","type":"` + eventType + `","source":"iam","subject":"tenant/1",` +
		`"specversion":"1","tenant_id":"` + uuid.NewString() + `","time":"` + time.Now().UTC().Format(time.RFC3339) +
		`","data":{}}`)
}

func routedTo(t *testing.T, target string) []string {
	t.Helper()
	cfg, err := topology.Load("../../config/topology.example.yaml")
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}
	var types []string
	for eventType, targets := range cfg.Routes {
		if slices.Contains(targets, target) {
			types = append(types, eventType)
		}
	}
	sort.Strings(types)
	if len(types) == 0 {
		t.Fatalf("no event type is routed to %s", target)
	}
	return types
}

func forwardOnce(t *testing.T, spec *contract.Spec, target, eventType string, status int) (string, error) {
	t.Helper()
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		spec.Request(t, r)
		if r.Header.Get("x-internal-token") != token {
			t.Errorf("x-internal-token = %q, want the target's token", r.Header.Get("x-internal-token"))
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	f := forwarder.New(forwarder.NewTarget(target, srv.URL, "", token, time.Second))
	err := f.Forward(t.Context(), "q", []string{target}, eventType, envelope(eventType))
	return path, err
}

func TestForward_RequestsMatchTheTargetsSpec(t *testing.T) {
	for _, target := range []string{"definition", "execution"} {
		spec := contract.Load(t, specs[target])
		for _, eventType := range routedTo(t, target) {
			t.Run(target+"/"+eventType, func(t *testing.T) {
				if _, err := forwardOnce(t, spec, target, eventType, http.StatusOK); err != nil {
					t.Fatalf("Forward: %v", err)
				}
			})
		}
	}
}

func TestForward_EveryDeclaredStatusIsClassified(t *testing.T) {
	for _, target := range []string{"definition", "execution"} {
		spec := contract.Load(t, specs[target])
		for _, eventType := range routedTo(t, target) {
			path, err := forwardOnce(t, spec, target, eventType, http.StatusOK)
			if err != nil {
				t.Fatalf("%s/%s: Forward: %v", target, eventType, err)
			}
			for _, status := range spec.Statuses(t, http.MethodPost, path) {
				t.Run(target+"/"+eventType+"/"+http.StatusText(status), func(t *testing.T) {
					permanent := slices.Contains(permanentStatuses, status)
					if status >= 300 && !permanent && !slices.Contains(retriedStatuses, status) {
						t.Fatalf("%s declares %d for %s and the consumer has no rule for it", target, status, path)
					}
					_, err := forwardOnce(t, spec, target, eventType, status)
					switch {
					case status < 300:
						if err != nil {
							t.Errorf("%d is a success, Forward = %v", status, err)
						}
					case permanent:
						if !forwarder.Permanent(err) {
							t.Errorf("%d must go to the DLQ after one delivery, Forward = %v", status, err)
						}
					default:
						if err == nil || forwarder.Permanent(err) {
							t.Errorf("%d must be retried, Forward = %v", status, err)
						}
					}
				})
			}
		}
	}
}
