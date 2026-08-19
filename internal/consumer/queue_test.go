package consumer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

func TestBuild_EmptyQueueURLErrors(t *testing.T) {
	fwd := forwarder.New()
	router := NewRouter(nil, nil)
	_, err := Build(QueueSpec{Name: "membership", URL: ""}, Options{}, fwd, router)
	assert.Error(t, err)
}
