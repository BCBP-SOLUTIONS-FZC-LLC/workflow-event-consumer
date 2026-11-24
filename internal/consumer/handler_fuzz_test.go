package consumer

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"
	eventsmock "github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/mocks"
)

func FuzzMakeHandler_TheOriginalBodyIsForwardedOrDeadLetteredUnchanged(f *testing.F) {
	f.Add("evt-1", "DepartmentMembershipRevoked", "iam", "t-1", []byte(`{"user_id":"u"}`), uint8(0))
	f.Add("evt-2", "UnknownType", "iam", "", []byte(`[1,2,3]`), uint8(1))
	f.Add("evt-3", "WorkflowTaskCreated", "execution", "t", []byte(`"\u0000"`), uint8(2))
	f.Add("e", "T", "s", "", []byte(`{"nested":{"deep":[null,true,1e308]}}`), uint8(1))

	d := mocks.NewDownstream()
	f.Cleanup(d.Close)
	fwd := forwarder.New(forwarder.NewTarget("definition", d.URL(), "", time.Second))
	statuses := []int{http.StatusOK, http.StatusBadRequest, http.StatusServiceUnavailable}

	f.Fuzz(func(t *testing.T, id, eventType, source, tenant string, payload []byte, pick uint8) {
		env := events.Envelope[json.RawMessage]{ID: id, Type: eventType, Source: source, TenantID: tenant, Payload: json.RawMessage(payload)}
		status := statuses[int(pick)%len(statuses)]
		d.SetStatus(status)
		dlq := &eventsmock.DLQPublisher{}
		c := &eventsmock.Consumer{}
		c.SetHandler(MakeHandler("fuzz", fwd, NewRouter(nil, []string{"definition"}), dlq))
		before := d.Count()

		if !sqsCanCarry(id, eventType, source, tenant, string(payload)) {
			return
		}
		err := c.Inject(env)
		want, jerr := env.JSON()
		if errors.Is(err, eventsmock.ErrMalformedEnvelope) || jerr != nil {
			return
		}
		if d.Count() != before+1 {
			t.Fatalf("forwarded %d times, want once (err=%v)", d.Count()-before, err)
		}
		bodies := d.Bodies()
		if got := bodies[len(bodies)-1]; !bytes.Equal(got, want) {
			t.Fatalf("forwarded body changed:\n got %s\nwant %s", got, want)
		}
		switch status {
		case http.StatusOK:
			if err != nil || len(dlq.Sent()) != 0 {
				t.Fatalf("a 2xx answer: err=%v, dead-lettered=%d", err, len(dlq.Sent()))
			}
		case http.StatusBadRequest:
			if err != nil || len(dlq.Sent()) != 1 || !bytes.Equal(dlq.Sent()[0].Body, bodies[len(bodies)-1]) {
				t.Fatalf("a permanent answer: err=%v, dead-lettered=%d", err, len(dlq.Sent()))
			}
		default:
			if err == nil || len(dlq.Sent()) != 0 {
				t.Fatalf("a transient answer: err=%v, dead-lettered=%d", err, len(dlq.Sent()))
			}
		}
	})
}

func TestMakeHandler_EveryTransientFailureIsRetriedAndNotDeadLettered(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, dlq, downstreams := newDeadLetterHarness(t, map[string]int{"definition": status})

			require.Error(t, c.Inject(deadLetterEnvelope()))
			require.Equal(t, 1, downstreams["definition"].Count())
			assert.Empty(t, dlq.Sent())
		})
	}
	t.Run("no answer", func(t *testing.T) {
		gone := mocks.NewDownstream()
		addr := gone.URL()
		gone.Close()
		dlq := &eventsmock.DLQPublisher{}
		c := &eventsmock.Consumer{}
		c.SetHandler(MakeHandler("membership", forwarder.New(forwarder.NewTarget("definition", addr, "", time.Second)),
			NewRouter(map[string][]string{"Evt": {"definition"}}, []string{"definition"}), dlq))

		require.Error(t, c.Inject(deadLetterEnvelope()))
		assert.Empty(t, dlq.Sent())
	})
}

func sqsCanCarry(values ...string) bool {
	for _, v := range values {
		if !utf8.ValidString(v) {
			return false
		}
		for _, r := range v {
			ok := r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000
			if !ok {
				return false
			}
		}
	}
	return true
}
