package cpi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tenants send ResourceSize as a string ("ResourceSize":"1234"), others as a
// number; both must decode.
func TestFlexNumber(t *testing.T) {
	for in, want := range map[string]float64{`"1234"`: 1234, `1234`: 1234, `"1.5"`: 1.5, `""`: 0, `null`: 0, `" 7 "`: 7} {
		var v struct {
			N flexNumber `json:"n"`
		}
		require.NoError(t, json.Unmarshal([]byte(`{"n":`+in+`}`), &v), in)
		assert.Equal(t, want, float64(v.N), in)
	}
	var v struct {
		N flexNumber `json:"n"`
	}
	assert.Error(t, json.Unmarshal([]byte(`{"n":"12 kB"}`), &v))
}
