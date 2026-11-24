package observability

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMust_ACollectorThatCannotBeRegisteredStopsStartUp(t *testing.T) {
	assert.PanicsWithValue(t, "metrics: register service collector: boom", func() {
		must(0, errors.New("boom"))
	})
}

func TestMust_ReturnsTheCollectorItWasGiven(t *testing.T) {
	assert.Equal(t, 7, must(7, nil))
}
