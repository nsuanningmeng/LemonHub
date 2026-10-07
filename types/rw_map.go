package types

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type RWMap[K comparable, V any] struct {
	data  map[K]V
	mutex sync.RWMutex
}

func decodeRWMap[K comparable, V any](b []byte) (map[K]V, error) {
	data := make(map[K]V)
	if err := common.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = make(map[K]V)
	}
	return data, nil
}

// ValidateJSON identifies identity-preserving configuration maps.
func (m *RWMap[K, V]) ValidateJSON(b []byte) error {
	_, err := decodeRWMap[K, V](b)
	return err
}
func (m *RWMap[K, V]) UnmarshalJSON(b []byte) error {
	data, err := decodeRWMap[K, V](b)
	if err != nil {
		return err
	}
	m.mutex.Lock()
	m.data = data
	m.mutex.Unlock()
	return nil
}

func (m *RWMap[K, V]) MarshalJSON() ([]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return common.Marshal(m.data)
}

func NewRWMap[K comparable, V any]() *RWMap[K, V] {
	return &RWMap[K, V]{
		data: make(map[K]V),
	}
}

func (m *RWMap[K, V]) Get(key K) (V, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	value, exists := m.data[key]
	return value, exists
}

func (m *RWMap[K, V]) Set(key K, value V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data[key] = value
}

func (m *RWMap[K, V]) AddAll(other map[K]V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, v := range other {
		m.data[k] = v
	}
}

func (m *RWMap[K, V]) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[K]V)
}

func (m *RWMap[K, V]) ReadAll() map[K]V {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	copiedMap := make(map[K]V)
	for k, v := range m.data {
		copiedMap[k] = v
	}
	return copiedMap
}

func (m *RWMap[K, V]) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.data)
}

func LoadFromJsonString[K comparable, V any](m *RWMap[K, V], jsonStr string) error {
	return m.UnmarshalJSON([]byte(jsonStr))
}

func LoadFromJsonStringWithCallback[K comparable, V any](m *RWMap[K, V], jsonStr string, onSuccess func()) error {
	data, err := decodeRWMap[K, V]([]byte(jsonStr))
	if err != nil {
		return err
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = data
	// Preserve the existing callback-after-commit, under-lock ordering.
	if onSuccess != nil {
		onSuccess()
	}
	return nil
}

func (m *RWMap[K, V]) MarshalJSONString() string {
	bytes, err := m.MarshalJSON()
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
