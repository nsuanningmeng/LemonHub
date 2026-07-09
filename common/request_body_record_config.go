package common

import "sync"

// RequestBodyRecordConfig 用户请求体记录配置（由 performance_setting 包更新）。
// 该功能仅对在用户设置中显式开启的用户生效，用于管理员排查问题。
type RequestBodyRecordConfig struct {
	// MaxSizeMB 已记录请求体的总空间上限（MB）；超出后自动删除最旧记录。
	MaxSizeMB int
}

// defaultRequestBodyRecordMaxSizeMB 默认总空间上限（MB）。
const defaultRequestBodyRecordMaxSizeMB = 256

var requestBodyRecordConfig = RequestBodyRecordConfig{
	MaxSizeMB: defaultRequestBodyRecordMaxSizeMB,
}
var requestBodyRecordConfigMu sync.RWMutex

// GetRequestBodyRecordConfig 获取请求体记录配置。
func GetRequestBodyRecordConfig() RequestBodyRecordConfig {
	requestBodyRecordConfigMu.RLock()
	defer requestBodyRecordConfigMu.RUnlock()
	return requestBodyRecordConfig
}

// SetRequestBodyRecordConfig 设置请求体记录配置。
func SetRequestBodyRecordConfig(config RequestBodyRecordConfig) {
	requestBodyRecordConfigMu.Lock()
	defer requestBodyRecordConfigMu.Unlock()
	requestBodyRecordConfig = config
}

// GetRequestBodyRecordMaxSizeBytes 获取总空间上限（字节）。上限 <= 0 时回退默认值，
// 避免误配置导致每次写入都触发全量删除。
func GetRequestBodyRecordMaxSizeBytes() int64 {
	requestBodyRecordConfigMu.RLock()
	defer requestBodyRecordConfigMu.RUnlock()
	mb := requestBodyRecordConfig.MaxSizeMB
	if mb <= 0 {
		mb = defaultRequestBodyRecordMaxSizeMB
	}
	return int64(mb) << 20
}
