package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSliceContainers(t *testing.T) {
	assert.True(t, strSliceContains([]string{"hello", "thar"}, "thar"))
	assert.False(t, strSliceContains([]string{"hello", "thar"}, "th"))
}
