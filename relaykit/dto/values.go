package dto

import (
	"encoding/json"
	"math/big"
	"strconv"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

type StringValue string

func (s *StringValue) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = StringValue(str)
		return nil
	}

	var raw json.Number
	if err := json.Unmarshal(data, &raw); err == nil {
		*s = StringValue(raw.String())
		return nil
	}

	return json.Unmarshal(data, &str)
}

func (s StringValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

type IntValue int

func (i *IntValue) UnmarshalJSON(b []byte) error {
	var n int
	if err := kitutil.Unmarshal(b, &n); err == nil {
		*i = IntValue(n)
		return nil
	}
	var s string
	if err := kitutil.Unmarshal(b, &s); err == nil {
		v, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*i = IntValue(v)
		return nil
	}
	var number json.Number
	if err := kitutil.Unmarshal(b, &number); err != nil {
		return err
	}
	// math/big scans the entire mantissa before rounding. Bound provider numeric
	// literals so finite fractions cannot trigger unbounded big-integer work.
	if len(number.String()) > 256 {
		return strconv.ErrRange
	}
	// Reject enormous exponents before arbitrary-precision parsing. Tiny finite
	// numbers underflowing float64 still have an exact truncated integer of zero.
	approximate, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return err
	}
	if approximate == 0 {
		*i = 0
		return nil
	}
	// Keep every possible host-int bit, rounding toward zero before discarding
	// fractional bits. float64 alone would round MaxInt64.0 up to 2^63.
	value, _, err := big.ParseFloat(number.String(), 10, uint(strconv.IntSize), big.ToZero)
	if err != nil {
		return err
	}
	if value.MantExp(nil) > strconv.IntSize {
		return strconv.ErrRange
	}
	integer, _ := value.Int(nil) // Bounded above: never allocate from a huge exponent.
	if !integer.IsInt64() || int64(int(integer.Int64())) != integer.Int64() {
		return strconv.ErrRange
	}
	*i = IntValue(integer.Int64())
	return nil
}

func (i IntValue) MarshalJSON() ([]byte, error) {
	return kitutil.Marshal(int(i))
}

type BoolValue bool

func (b *BoolValue) UnmarshalJSON(data []byte) error {
	var boolean bool
	if err := json.Unmarshal(data, &boolean); err == nil {
		*b = BoolValue(boolean)
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	if str == "true" {
		*b = BoolValue(true)
	} else if str == "false" {
		*b = BoolValue(false)
	} else {
		return json.Unmarshal(data, &boolean)
	}
	return nil
}
func (b BoolValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(b))
}
