package service

import (
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSensitiveWordMatchingPolicy(t *testing.T) {
	oldWords, oldWhole := setting.SensitiveWords, setting.SensitiveWordsWholeWordEnabled
	t.Cleanup(func() { setting.SensitiveWords, setting.SensitiveWordsWholeWordEnabled = oldWords, oldWhole })
	for _, tc := range []struct {
		name, text     string
		words          []string
		whole, blocked bool
	}{
		{"legacy substring", "AgenticChat", []string{"cch"}, false, true},
		{"legacy case insensitive", "AGENTICCHAT", []string{"CCH"}, false, true},
		{"whole word harmless identifier", "AgenticChat", []string{"cch"}, true, false},
		{"whole word standalone", " CCH ", []string{"cch"}, true, true},
		{"later valid hit", "AgenticChat and cch", []string{"cch"}, true, true},
		{"underscore is word", "prefix_cch_suffix", []string{"cch"}, true, false},
		{"number is word", "2cch3", []string{"cch"}, true, false},
		{"unicode letter left", "中cch", []string{"cch"}, true, false},
		{"unicode letter right", "cché", []string{"cch"}, true, false},
		{"unicode digit", "cch٣", []string{"cch"}, true, false},
		{"emoji boundary", "🙂cch🙂", []string{"cch"}, true, true},
		{"CJK substring", "这是敏感词例子", []string{"敏感词"}, true, true},
		{"mixed entry substring", "xx中cchxx", []string{"中cch"}, true, true},
		{"punctuated entry substring", "xxc-chxx", []string{"c-ch"}, true, true},
		{"Unicode normalizes to ASCII", "xxkxx", []string{"K"}, true, true},
		{"duplicate normalized Unicode entry", "xxkxx", []string{"K", "K"}, true, true},
		{"overlapping rejected and accepted", "concch abc", []string{"cch", "abc"}, true, true},
		{"empty dictionary", "cch", nil, true, false},
		{"empty entries", "cch", []string{"", "  "}, true, false},
		{"trimmed entry", " cch ", []string{" cch "}, true, true},
		{"empty input", "", []string{"cch"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting.SensitiveWords, setting.SensitiveWordsWholeWordEnabled = tc.words, tc.whole
			blocked, words := SensitiveWordContains(tc.text)
			assert.Equal(t, tc.blocked, blocked)
			if tc.blocked {
				assert.NotEmpty(t, words)
			} else {
				assert.Empty(t, words)
			}
		})
	}
	// The shared substring engine also powers automatic channel disable and error
	// overrides; opting in to prompt word boundaries must not change those callers.
	setting.SensitiveWordsWholeWordEnabled = true
	blocked, _ := AcSearch("agenticchat", []string{"cch"}, true)
	assert.True(t, blocked)
}
