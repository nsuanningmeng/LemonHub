package model

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequestBodyLog 存储用户的完整原始请求体，仅对在用户设置中显式开启
// RecordRequestBody 的用户生效。按 request_id 与 logs 表关联，供管理员在日志详情
// 中查看。总占用空间由性能设置中的上限约束，超出后自动删除最旧记录。
//
// Body 使用无长度标签的 string，GORM 会在 MySQL 上映射为 LONGTEXT、在 PostgreSQL
// 与 SQLite 上映射为 TEXT（均可容纳大请求体），与 logs.content 的处理方式一致。
type RequestBodyLog struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	RequestId   string `json:"request_id" gorm:"type:varchar(64);uniqueIndex:idx_rbl_request_id;default:''"`
	UserId      int    `json:"user_id" gorm:"index:idx_rbl_user_id"`
	Username    string `json:"username" gorm:"type:varchar(64);default:''"`
	ChannelId   int    `json:"channel_id" gorm:"default:0"`
	ModelName   string `json:"model_name" gorm:"type:varchar(128);default:''"`
	TokenName   string `json:"token_name" gorm:"type:varchar(128);default:''"`
	RequestPath string `json:"request_path" gorm:"type:varchar(255);default:''"`
	ContentType string `json:"content_type" gorm:"type:varchar(128);default:''"`
	Body        string `json:"body"`
	Size        int    `json:"size" gorm:"default:0;index:idx_rbl_created_at,priority:2"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index:idx_rbl_created_at,priority:1"`
}

// RequestBodyCaptureMeta 记录一条请求体时需要的元数据（除 gin.Context 外的信息）。
type RequestBodyCaptureMeta struct {
	ChannelId int
	ModelName string
	TokenName string
}

// CaptureUserRequestBody 在开启记录的前提下，快照当前请求的完整原始请求体并异步持久化。
// 返回值表示本次是否已捕获（用于在日志 other.admin_info 中标记 has_request_body）。
//
// 快照必须同步完成：BodyStorageCleanup 会在 handler 返回后立即关闭底层存储，异步读取会
// 与清理竞争。持久化（写库 + 淘汰）放到 goroutine，避免拖慢请求主链路。
func CaptureUserRequestBody(c *gin.Context, userId int, meta RequestBodyCaptureMeta) bool {
	if c == nil {
		return false
	}
	requestId := c.GetString(common.RequestIdKey)
	if requestId == "" {
		return false
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil || storage == nil {
		return false
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return false
	}

	// 单条请求体超过总上限时不记录，避免写入后立刻把所有历史记录淘汰。
	maxBytes := common.GetRequestBodyRecordMaxSizeBytes()
	if int64(len(body)) > maxBytes {
		common.SysLog("request body record skipped: single body exceeds total size limit")
		return false
	}

	// []byte -> string 转换本身即生成独立的不可变副本，可安全存活于底层存储被复用/关闭之后，
	// 因此无需再显式 make+copy 一遍（那会多一次全长度分配与内存拷贝）。
	record := &RequestBodyLog{
		RequestId:   requestId,
		UserId:      userId,
		Username:    c.GetString("username"),
		ChannelId:   meta.ChannelId,
		ModelName:   meta.ModelName,
		TokenName:   meta.TokenName,
		RequestPath: requestPathForRecord(c),
		ContentType: c.Request.Header.Get("Content-Type"),
		Body:        string(body),
		Size:        len(body),
		CreatedAt:   common.GetTimestamp(),
	}

	gopool.Go(func() {
		if err := persistRequestBodyLog(record); err != nil {
			common.SysError("failed to persist request body log: " + err.Error())
		}
	})
	return true
}

func requestPathForRecord(c *gin.Context) string {
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

// persistRequestBodyLog 写入一条记录并按总空间上限淘汰最旧记录。
func persistRequestBodyLog(record *RequestBodyLog) error {
	// 幂等：同一 request_id 已存在时（例如同一请求既记 consume 又记 error），跳过重复写入。
	if err := DB.Where("request_id = ?", record.RequestId).FirstOrCreate(record).Error; err != nil {
		return err
	}
	enforceRequestBodyRecordLimit(false)
	return nil
}

const (
	requestBodyEvictionBatchSize = 1000
	requestBodyEvictionInterval  = 5 * time.Second
)

var (
	// requestBodyEvictionMu 串行化淘汰：并发的异步持久化不会各自重复地对主库做全表 SUM
	// 扫描并删除同一批最旧记录（否则会放大主库扫描/行锁竞争 M 倍）。
	requestBodyEvictionMu sync.Mutex
	// requestBodyLastEvictionNano 节流：避免每次写入都对全表做一次 SUM 聚合。
	requestBodyLastEvictionNano atomic.Int64
)

// enforceRequestBodyRecordLimit 在总占用超过上限时，按创建时间从旧到新循环删除记录直至不超限。
// 用 TryLock 串行化（并发写入下至多一个淘汰过程在运行，其余直接跳过），并按固定间隔节流，
// 避免高并发下对主库反复做全表 SUM 扫描与对同一批行的重复删除。总空间上限为软上限（最终一致）。
// force=true 跳过节流（例如上限被调低后希望立即回收）。
func enforceRequestBodyRecordLimit(force bool) {
	maxBytes := common.GetRequestBodyRecordMaxSizeBytes()
	if maxBytes <= 0 {
		return
	}
	if !force {
		last := requestBodyLastEvictionNano.Load()
		now := time.Now().UnixNano()
		if last != 0 && now-last < int64(requestBodyEvictionInterval) {
			return
		}
	}
	if !requestBodyEvictionMu.TryLock() {
		// 已有淘汰过程在运行，交给它把总量降到上限以下即可。
		return
	}
	defer requestBodyEvictionMu.Unlock()
	requestBodyLastEvictionNano.Store(time.Now().UnixNano())

	total, err := sumRequestBodyRecordBytes()
	if err != nil {
		common.SysError("failed to read request body record total: " + err.Error())
		return
	}

	type idSize struct {
		Id   int64
		Size int
	}
	// 循环删除最旧批次直至不超限；本地累减 total，不每批重新 SUM。
	for total > maxBytes {
		var candidates []idSize
		if err := DB.Model(&RequestBodyLog{}).
			Select("id", "size").
			Order("id asc").
			Limit(requestBodyEvictionBatchSize).
			Find(&candidates).Error; err != nil {
			common.SysError("failed to list request body records for eviction: " + err.Error())
			return
		}
		if len(candidates) == 0 {
			return
		}
		ids := make([]int64, 0, len(candidates))
		freed := int64(0)
		for _, cand := range candidates {
			ids = append(ids, cand.Id)
			freed += int64(cand.Size)
			if total-freed <= maxBytes {
				break
			}
		}
		if err := DB.Where("id IN ?", ids).Delete(&RequestBodyLog{}).Error; err != nil {
			common.SysError("failed to evict request body records: " + err.Error())
			return
		}
		total -= freed
		// 本批不足一整批（已无更多可删）却仍超限：退出，避免空转。
		if len(candidates) < requestBodyEvictionBatchSize {
			return
		}
	}
}

// sumRequestBodyRecordBytes 仅聚合已记录请求体的总字节数（淘汰路径用，不做 COUNT 全表扫描）。
func sumRequestBodyRecordBytes() (int64, error) {
	var total int64
	err := DB.Model(&RequestBodyLog{}).Select("COALESCE(SUM(size), 0)").Scan(&total).Error
	return total, err
}

// GetRequestBodyRecordStats 返回当前记录的总字节数与记录条数（供管理端性能页展示，调用频率低）。
func GetRequestBodyRecordStats() (totalBytes int64, count int64, err error) {
	if totalBytes, err = sumRequestBodyRecordBytes(); err != nil {
		return 0, 0, err
	}
	if err = DB.Model(&RequestBodyLog{}).Count(&count).Error; err != nil {
		return 0, 0, err
	}
	return totalBytes, count, nil
}

// GetRequestBodyLogByRequestId 按 request_id 查询单条请求体记录（管理员查看用）。
func GetRequestBodyLogByRequestId(requestId string) (*RequestBodyLog, error) {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var record RequestBodyLog
	if err := DB.Where("request_id = ?", requestId).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// ClearAllRequestBodyLogs 清空全部请求体记录，返回删除的条数。
func ClearAllRequestBodyLogs() (int64, error) {
	res := DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RequestBodyLog{})
	return res.RowsAffected, res.Error
}

// DeleteRequestBodyLogsByUserId 删除指定用户的全部已记录请求体。用于关闭记录时立即清理、
// 以及删除用户时避免其原始请求体（可能含密钥/PII）在库中无限滞留。userId 为 0 时不做任何操作。
func DeleteRequestBodyLogsByUserId(userId int) (int64, error) {
	if userId == 0 {
		return 0, nil
	}
	res := DB.Where("user_id = ?", userId).Delete(&RequestBodyLog{})
	return res.RowsAffected, res.Error
}

// MarkHasRequestBody 在日志 other 的 admin_info 中标记该请求已记录完整请求体。
// 嵌套在 admin_info 下，formatUserLogs 会对非管理员视图整体剥离，因此天然仅管理员可见。
func MarkHasRequestBody(other map[string]interface{}) {
	if other == nil {
		return
	}
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	if !ok || adminInfo == nil {
		adminInfo = map[string]interface{}{}
		other["admin_info"] = adminInfo
	}
	adminInfo["has_request_body"] = true
}
