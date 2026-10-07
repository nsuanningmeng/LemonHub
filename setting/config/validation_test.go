package config

import (
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type nestedValidation struct {
	Name   string
	Values []int
}
type validationConfig struct {
	Small    int8                          `json:"small"`
	Wide     int64                         `json:"wide"`
	Unsigned uint64                        `json:"unsigned"`
	Float    float32                       `json:"float"`
	Flag     bool                          `json:"flag"`
	Text     string                        `json:"text"`
	Nested   nestedValidation              `json:"nested"`
	Pointer  *nestedValidation             `json:"pointer"`
	Map      map[string]int                `json:"map"`
	Locked   *types.RWMap[string, float64] `json:"locked"`
}

func TestValidationPreflightPreservesAliasesAndRejectsUnsafeNumbers(t *testing.T) {
	for _, bad := range []map[string]string{{"small": "128"}, {"wide": "-9223372036854775809"}, {"wide": "-9223372036854775809.0"}, {"wide": "9223372036854775808"}, {"unsigned": "18446744073709551616"}, {"unsigned": "-0.5"}, {"float": "NaN"}, {"float": "+Inf"}, {"float": "3.5e38"}, {"flag": "private-secret"}, {"nested": `{"Name":"changed","Values":[1,"private-secret"]}`}, {"locked": `{"new":"private-secret"}`}} {
		cfg := validationConfig{Text: "old", Nested: nestedValidation{Name: "old", Values: []int{7}}, Locked: types.NewRWMap[string, float64]()}
		cfg.Locked.Set("old", 3)
		bad["text"] = "would-change"
		err := UpdateConfigFromMap(&cfg, bad)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "private-secret")
		assert.Equal(t, "old", cfg.Text)
		assert.Equal(t, []int{7}, cfg.Nested.Values)
		assert.Equal(t, map[string]float64{"old": 3}, cfg.Locked.ReadAll())
	}
}
func TestValidationCompatibilityMergeNullAndRWMapIdentity(t *testing.T) {
	cfg := validationConfig{Nested: nestedValidation{Name: "retain", Values: []int{7}}, Pointer: &nestedValidation{Name: "retain"}, Locked: types.NewRWMap[string, float64](), Map: map[string]int{"old": 1}}
	alias, pointer := cfg.Locked, cfg.Pointer
	require.NoError(t, UpdateConfigFromMap(&cfg, map[string]string{"small": "127.9", "wide": "9223372036854775807", "unsigned": "18446744073709551615", "flag": "1", "text": "  unchanged spaces  ", "nested": `{"Values":[2]}`, "pointer": `{"Values":[3]}`, "locked": `{"new":2}`, "map": `{"new":2}`, "unknown": "{invalid"}))
	assert.Equal(t, int8(127), cfg.Small)
	assert.Equal(t, int64(9223372036854775807), cfg.Wide)
	assert.Equal(t, uint64(18446744073709551615), cfg.Unsigned)
	assert.Equal(t, "retain", cfg.Nested.Name)
	assert.Same(t, pointer, cfg.Pointer)
	assert.Same(t, alias, cfg.Locked)
	assert.Equal(t, map[string]int{"new": 2}, cfg.Map)
	require.NoError(t, UpdateConfigFromMap(&cfg, map[string]string{"small": "2.0000", "locked": "null", "pointer": "null", "map": "null"}))
	assert.Equal(t, int8(2), cfg.Small)
	assert.Nil(t, cfg.Pointer)
	assert.Nil(t, cfg.Map)
	assert.Same(t, alias, cfg.Locked)
	cfg.Locked.Set("after-null", 4)
	assert.Equal(t, map[string]float64{"after-null": 4}, alias.ReadAll())
}
func TestManagerLoadValidatesAllGroupsBeforePublication(t *testing.T) {
	cm := NewConfigManager()
	first, second := validationConfig{Text: "first"}, validationConfig{Text: "second"}
	cm.Register("a", &first)
	cm.Register("b", &second)
	require.Error(t, cm.LoadFromDB(map[string]string{"a.text": "changed", "b.small": "1000"}))
	assert.Equal(t, "first", first.Text)
	assert.Equal(t, "second", second.Text)
	require.NoError(t, cm.LoadFromDB(map[string]string{"a.text": "changed", "unknown.value": "{invalid"}))
	assert.Equal(t, "changed", first.Text)
}

func TestValidationUnsupportedFieldsKeepExistingValues(t *testing.T) {
	cfg := struct {
		Any      interface{}   `json:"any"`
		Array    [2]int        `json:"array"`
		Function func() string `json:"function"`
	}{Any: "keep", Array: [2]int{4, 5}, Function: func() string { return "keep" }}
	require.NoError(t, UpdateConfigFromMap(&cfg, map[string]string{"any": "null", "array": "[]", "function": "null"}))
	assert.Equal(t, "keep", cfg.Any)
	assert.Equal(t, [2]int{4, 5}, cfg.Array)
	assert.Equal(t, "keep", cfg.Function())
}

func TestManagerSaveCallbackDoesNotHoldRegistryLock(t *testing.T) {
	cm := NewConfigManager()
	cfg := validationConfig{Text: "preserved"}
	cm.Register("original", &cfg)
	calls := 0
	require.NoError(t, cm.SaveToDB(func(key, value string) error {
		calls++
		cm.Register("added", &validationConfig{})
		return nil
	}))
	assert.Positive(t, calls)
	assert.NotNil(t, cm.Get("added"))
}
