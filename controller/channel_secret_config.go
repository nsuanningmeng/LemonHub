package controller

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// channelConfigField counts every occurrence, including identical duplicate
// keys which a map would collapse. A single raw value can be preserved exactly;
// multiple values may have string-null or struct-merge decoding semantics.
type channelConfigField struct {
	value       json.RawMessage
	occurrences int
}

func (field *channelConfigField) UnmarshalJSON(data []byte) error {
	field.occurrences++
	field.value = append(field.value[:0], data...)
	return nil
}

func readChannelConfigField(raw, field string) (channelConfigField, error) {
	var stored struct {
		Proxy          channelConfigField `json:"proxy"`
		AdvancedCustom channelConfigField `json:"advanced_custom"`
	}
	if common.Unmarshal([]byte(raw), &stored) != nil {
		return channelConfigField{}, errors.New("stored channel settings could not be read safely")
	}
	switch field {
	case "proxy":
		return stored.Proxy, nil
	case "advanced_custom":
		return stored.AdvancedCustom, nil
	default:
		return channelConfigField{}, errors.New("unsupported channel setting")
	}
}

// Sensitive configurations remain in storage and internal relay readers. Only
// their public representation is removed for administrators without secret read.
func redactChannelConfigField(raw, field string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var config map[string]json.RawMessage
	if common.Unmarshal([]byte(raw), &config) != nil || config == nil {
		return "{}", true
	}
	configured := false
	for name, value := range config {
		// encoding/json accepts case-insensitive struct field names. Every
		// accepted alias must be hidden, including duplicate spellings.
		if strings.EqualFold(name, field) {
			configured = configured || (string(value) != "null" && string(value) != `""`)
			delete(config, name)
		}
	}
	// A map collapses identical duplicate keys. A later null may not erase an
	// earlier typed value, so ambiguity must never be advertised as unconfigured.
	if stored, err := readChannelConfigField(raw, field); err != nil || stored.occurrences > 1 {
		configured = true
	}
	encoded, err := common.Marshal(config)
	if err != nil {
		return "{}", configured
	}
	return string(encoded), configured
}

// A write-only administrator can replace or explicitly clear a hidden field,
// but an omitted field must not erase its stored value when editing other JSON
// settings. Keep the original raw field so unknown provider options survive.
func preserveOmittedChannelConfigField(raw, original, field string) (string, error) {
	config := make(map[string]json.RawMessage)
	if strings.TrimSpace(raw) != "" {
		if common.Unmarshal([]byte(raw), &config) != nil || config == nil {
			return "", errors.New("invalid channel settings")
		}
	}
	for name := range config {
		if strings.EqualFold(name, field) {
			return raw, nil
		}
	}
	if strings.TrimSpace(original) == "" {
		return raw, nil
	}
	// A typed decoder can ignore a later null for strings or merge successive
	// objects into a pointer field. Do not guess those semantics from a map or
	// keep only the last raw value: reject ambiguous implicit edits unchanged.
	value, err := readChannelConfigField(original, field)
	if err != nil {
		return "", err
	}
	if value.occurrences > 1 {
		return "", errors.New("stored channel settings contain repeated sensitive fields; explicitly replace the hidden configuration")
	}
	if value.occurrences == 1 {
		config[field] = value.value
	}
	encoded, err := common.Marshal(config)
	if err != nil {
		return "", errors.New("invalid channel settings")
	}
	return string(encoded), nil
}
