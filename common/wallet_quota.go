package common

import (
	"errors"

	"github.com/shopspring/decimal"
)

// MaxWalletQuota is the largest integer that remains exact in both browser
// JavaScript and Redis Lua. Wallets use 64-bit database storage on supported
// amd64/arm64 builds; individual API charges still use the int32 MaxQuota limit.
const MaxWalletQuota = 1<<53 - 1

var ErrWalletQuotaOutOfRange = errors.New("wallet quota out of range")

// WalletQuotaFromDecimalStrict rounds a non-negative wallet credit half away
// from zero. Compare decimals before conversion so purchased quota is never
// silently saturated or rounded through float64 at the wallet limit.
func WalletQuotaFromDecimalStrict(value decimal.Decimal) (int, error) {
	if value.IsNegative() {
		return 0, ErrWalletQuotaOutOfRange
	}
	rounded := value.Round(0)
	if rounded.GreaterThan(decimal.NewFromInt(MaxWalletQuota)) {
		return 0, ErrWalletQuotaOutOfRange
	}
	return int(rounded.IntPart()), nil
}
