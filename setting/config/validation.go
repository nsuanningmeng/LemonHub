package config

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ValidateConfigFromMap checks recognized fields without touching live values.
// Unknown fields preserve the existing passthrough contract.
func ValidateConfigFromMap(cfg interface{}, values map[string]string) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	v = v.Elem()
	for i := 0; i < v.NumField(); i++ {
		sf := v.Type().Field(i)
		if !sf.IsExported() || !v.Field(i).CanSet() {
			continue
		}
		key := configFieldKey(sf)
		raw, ok := values[key]
		if !ok {
			continue
		}
		if _, err := decodeConfigField(v.Field(i).Type(), raw); err != nil {
			return fmt.Errorf("invalid configuration field %s", sf.Name)
		}
	}
	return nil
}

func configFieldKey(sf reflect.StructField) string {
	key := sf.Tag.Get("json")
	if key == "" || key == "-" {
		return sf.Name
	}
	return key
}

func decodeConfigField(typ reflect.Type, raw string) (reflect.Value, error) {
	value := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.String:
		value.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return value, err
		}
		value.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, typ.Bits())
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return value, fmt.Errorf("integer overflow")
			}
			number, parseErr := strconv.ParseFloat(raw, 64)
			number = math.Trunc(number)
			limit := math.Ldexp(1, typ.Bits()-1)
			if parseErr != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < -limit || number >= limit {
				return value, fmt.Errorf("invalid integer")
			}
			// A decimal just below the lower bound may round to that bound
			// in float64. Check its exact truncated integer before conversion.
			if number == -limit {
				precise, _, preciseErr := big.ParseFloat(raw, 0, 256, big.ToZero)
				if preciseErr != nil {
					return value, fmt.Errorf("invalid integer")
				}
				integer, _ := precise.Int(nil)
				minimum := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), uint(typ.Bits()-1)))
				if integer.Cmp(minimum) < 0 {
					return value, fmt.Errorf("integer overflow")
				}
			}
			parsed = int64(number)
		}
		value.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, typ.Bits())
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return value, fmt.Errorf("integer overflow")
			}
			number, parseErr := strconv.ParseFloat(raw, 64)
			// Negative values are rejected before truncation, retaining the old rule.
			if parseErr != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
				return value, fmt.Errorf("invalid unsigned integer")
			}
			number = math.Trunc(number)
			if number >= math.Ldexp(1, typ.Bits()) {
				return value, fmt.Errorf("integer overflow")
			}
			parsed = uint64(number)
		}
		value.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, typ.Bits())
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return value, fmt.Errorf("invalid finite number")
		}
		value.SetFloat(parsed)
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Struct:
		// A fresh value prevents partial JSON decoder errors from mutating aliases.
		if err := common.Unmarshal([]byte(raw), value.Addr().Interface()); err != nil {
			return value, err
		}
	default:
		return reflect.Value{}, nil // Existing unsupported fields are ignored.
	}
	return value, nil
}

// ValidateOptions validates all registered modules before any caller publishes.
func (cm *ConfigManager) ValidateOptions(options map[string]string) error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.validateOptionsLocked(options)
}
func (cm *ConfigManager) validateOptionsLocked(options map[string]string) error {
	for name, cfg := range cm.configs {
		values := make(map[string]string)
		for key, value := range options {
			if strings.HasPrefix(key, name+".") {
				values[strings.TrimPrefix(key, name+".")] = value
			}
		}
		if err := ValidateConfigFromMap(cfg, values); err != nil {
			return fmt.Errorf("invalid configuration module %s: %w", name, err)
		}
	}
	return nil
}
