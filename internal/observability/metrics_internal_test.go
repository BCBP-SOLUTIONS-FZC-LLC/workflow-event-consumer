package observability

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

func TestMust_ACollectorThatCannotBeRegisteredStopsStartUp(t *testing.T) {
	assert.PanicsWithValue(t, "metrics: register service collector: boom", func() {
		must(0, errors.New("boom"))
	})
}

func TestRegister_ASecondCallKeepsTheCollectorsOfTheFirst(t *testing.T) {
	_ = gincommon.ObservabilityMiddlewares(gincommon.Config{ServiceName: ServiceName, BuildVersion: "test", Domain: Domain, Environment: "test"})
	Register()
	forward, forwardDuration, handler := ForwardTotal, ForwardDurationSeconds, HandlerDurationSeconds

	assert.NotPanics(t, Register, "a second registration of the same names would panic on a duplicate")
	assert.NotNil(t, forward)
	assert.Same(t, forward, ForwardTotal)
	assert.Same(t, forwardDuration, ForwardDurationSeconds)
	assert.Same(t, handler, HandlerDurationSeconds)
}

func TestMust_ReturnsTheCollectorItWasGiven(t *testing.T) {
	assert.Equal(t, 7, must(7, nil))
}
