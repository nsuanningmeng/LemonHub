package common

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletQuotaFromDecimalStrict(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  int
		fails bool
	}{
		{name: "zero", value: "0"},
		{name: "round down", value: "42.4", want: 42},
		{name: "round half up", value: "42.5", want: 43},
		{name: "5000 units", value: "2500000000", want: 2500000000},
		{name: "30000 units", value: "15000000000", want: 15000000000},
		{name: "existing large wallet", value: "999994138683436", want: 999994138683436},
		{name: "maximum exact integer", value: "9007199254740991", want: MaxWalletQuota},
		{name: "fraction below maximum", value: "9007199254740990.5", want: MaxWalletQuota},
		{name: "rounded overflow", value: "9007199254740991.5", fails: true},
		{name: "integer overflow", value: "9007199254740992", fails: true},
		{name: "int64 overflow", value: "9223372036854775808", fails: true},
		{name: "negative", value: "-1", fails: true},
		{name: "negative fraction", value: "-0.1", fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := decimal.NewFromString(tc.value)
			require.NoError(t, err)
			got, err := WalletQuotaFromDecimalStrict(value)
			if tc.fails {
				require.ErrorIs(t, err, ErrWalletQuotaOutOfRange)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
