package types

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRWMapLoadersPreserveDataOnDecodeFailure(t *testing.T) {
	for _, loader := range []string{"unmarshal", "load", "callback"} {
		t.Run(loader, func(t *testing.T) {
			m := NewRWMap[string, map[string]int]()
			m.Set("old", map[string]int{"value": 7})
			calls := 0
			load := func(raw string) error {
				switch loader {
				case "unmarshal":
					return m.UnmarshalJSON([]byte(raw))
				case "load":
					return LoadFromJsonString(m, raw)
				default:
					return LoadFromJsonStringWithCallback(m, raw, func() { calls++ })
				}
			}
			require.Error(t, load(`{"new":{"first":1,"bad":"secret"}}`))
			assert.Equal(t, map[string]map[string]int{"old": {"value": 7}}, m.ReadAll())
			assert.Zero(t, calls)
			require.NoError(t, load(`{"new":{"value":2}}`))
			assert.Equal(t, map[string]map[string]int{"new": {"value": 2}}, m.ReadAll())
			require.NoError(t, load("null"))
			m.Set("after-null", map[string]int{"value": 3})
			assert.Equal(t, 1, m.Len())
			if loader == "callback" {
				assert.Equal(t, 2, calls)
			}
		})
	}
}
