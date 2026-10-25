package model

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	RewardTypeFixed     = "fixed"
	RewardTypeRandom    = "random"
	RewardStatusPending = "pending"
	RewardStatusClaimed = "claimed"
	RewardStatusExpired = "expired"
	maxRewardRecipients = 100000
)

var (
	ErrInvalidRewardCampaign = errors.New("invalid reward campaign")
	ErrInvalidRewardAudience = errors.New("invalid reward audience")
	ErrRewardRequestConflict = errors.New("reward request id already used for another campaign")
	ErrRewardAudienceChanged = errors.New("reward audience changed since preview")
	ErrRewardRecipientsEmpty = errors.New("no eligible reward recipients")
	ErrRewardRecipientsLimit = errors.New("reward recipient count or total quota exceeds limit")
	ErrRewardNotFound        = errors.New("reward not found")
	ErrRewardUserUnavailable = errors.New("reward user unavailable")
	ErrRewardExpired         = errors.New("reward expired")
	ErrInvalidRewardStatus   = errors.New("invalid reward status or type")
)

type RewardAudienceFilter struct {
	Audience  string `json:"audience"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

type RewardCampaignInput struct {
	Continuous             bool   `json:"continuous"`
	Title                  string `json:"title"`
	Description            string `json:"description"`
	Type                   string `json:"type"`
	MinQuota               int    `json:"min_quota"`
	MaxQuota               int    `json:"max_quota"`
	ExpiresInDays          int    `json:"expires_in_days"`
	Audience               string `json:"audience"`
	StartTime              int64  `json:"start_time"`
	EndTime                int64  `json:"end_time"`
	RequestID              string `json:"request_id"`
	ExpectedRecipientCount *int   `json:"expected_recipient_count,omitempty"`
}

type RewardRecipientPreview struct {
	RecipientCount int `json:"recipient_count"`
}

// Legacy campaigns retain their immutable recipient snapshot. New continuous
// campaigns explicitly opt in to recoverable distribution; old rows stay false.
type RewardCampaign struct {
	Id                 int    `json:"id" gorm:"primaryKey"`
	Continuous         bool   `json:"continuous" gorm:"not null;default:false;index:idx_reward_continuous"`
	DistributionStatus string `json:"distribution_status" gorm:"-"`
	Title              string `json:"title" gorm:"type:varchar(480);not null"`
	Description        string `json:"description" gorm:"type:text"`
	Type               string `json:"type" gorm:"type:varchar(16);not null"`
	MinQuota           int    `json:"min_quota" gorm:"type:bigint;not null"`
	MaxQuota           int    `json:"max_quota" gorm:"type:bigint;not null"`
	ExpiresInDays      int    `json:"expires_in_days" gorm:"not null;default:0"`
	ExpiresAt          int64  `json:"expires_at" gorm:"type:bigint;not null;default:0"`
	Audience           string `json:"audience" gorm:"type:varchar(16);not null"`
	StartTime          int64  `json:"start_time" gorm:"type:bigint;not null"`
	EndTime            int64  `json:"end_time" gorm:"type:bigint;not null"`
	RequestID          string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedBy          int    `json:"created_by" gorm:"not null"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	RecipientCount     int    `json:"recipient_count" gorm:"not null"`
	ClaimedCount       int64  `json:"claimed_count" gorm:"-"`
	ClaimedQuota       int64  `json:"claimed_quota" gorm:"-"`
	ClaimedQuotaExact  string `json:"claimed_quota_exact" gorm:"-"`
}

// MarshalJSON keeps legacy numeric statistics unchanged when exactly representable
// by JavaScript. Larger campaign totals are sent only as their exact decimal
// string; null never pretends that a saturated or rounded number is the total.
func (campaign RewardCampaign) MarshalJSON() ([]byte, error) {
	type rewardCampaignJSON RewardCampaign
	if campaign.ClaimedQuotaExact == "" {
		campaign.ClaimedQuotaExact = strconv.FormatInt(campaign.ClaimedQuota, 10)
	}
	total, valid := new(big.Int).SetString(campaign.ClaimedQuotaExact, 10)
	if !valid || total.Sign() < 0 {
		return nil, ErrInvalidRewardCampaign
	}
	var numeric *int64
	if total.IsInt64() && total.Int64() <= common.MaxWalletQuota {
		value := total.Int64()
		numeric = &value
	}
	return common.Marshal(struct {
		rewardCampaignJSON
		ClaimedQuota *int64 `json:"claimed_quota"`
	}{rewardCampaignJSON: rewardCampaignJSON(campaign), ClaimedQuota: numeric})
}

// UserReward is also the durable credit ledger: the claim and wallet increment
// commit in the same main-database transaction, even with a separate log DB.
type UserReward struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	CampaignID   int    `json:"campaign_id" gorm:"not null;uniqueIndex:idx_reward_campaign_user,priority:1"`
	UserID       int    `json:"user_id" gorm:"not null;uniqueIndex:idx_reward_campaign_user,priority:2;index:idx_reward_user_status,priority:1"`
	Title        string `json:"title" gorm:"type:varchar(480);not null"`
	Description  string `json:"description" gorm:"type:text"`
	Type         string `json:"type" gorm:"type:varchar(16);not null"`
	MinQuota     int    `json:"min_quota" gorm:"type:bigint;not null"`
	MaxQuota     int    `json:"max_quota" gorm:"type:bigint;not null"`
	QuotaAwarded int    `json:"quota_awarded" gorm:"type:bigint;not null"`
	Status       string `json:"status" gorm:"type:varchar(16);not null;index:idx_reward_user_status,priority:2"`
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint;not null"`
	ClaimedAt    int64  `json:"claimed_at" gorm:"type:bigint;not null"`
	ExpiresAt    int64  `json:"expires_at" gorm:"type:bigint;not null;default:0"`
}

type RewardSummary struct {
	FixedCount   int64 `json:"fixed_count"`
	RandomCount  int64 `json:"random_count"`
	PendingCount int64 `json:"pending_count"`
	FixedQuota   int64 `json:"fixed_quota"`
}

func validateRewardAudience(filter RewardAudienceFilter) error {
	if filter.StartTime < 0 || filter.EndTime <= filter.StartTime {
		return ErrInvalidRewardAudience
	}
	switch filter.Audience {
	case "topup", "usage", "both", "either":
		return nil
	default:
		return ErrInvalidRewardAudience
	}
}

// rewardRecipientIDs queries each database independently; no cross-database JOIN
// is assumed. Bounded, distinct source sets also cap memory and SQL parameters.
func rewardRecipientIDs(filter RewardAudienceFilter) ([]int, error) {
	if err := validateRewardAudience(filter); err != nil {
		return nil, err
	}
	candidates := make(map[int]struct{})
	if filter.Audience != "usage" {
		var ids []int
		err := DB.Model(&TopUp{}).Distinct("user_id").
			Where("site_id = ? AND status = ? AND clawed_back_quota = ? AND complete_time >= ? AND complete_time < ?", 0, common.TopUpStatusSuccess, 0, filter.StartTime, filter.EndTime).
			Limit(maxRewardRecipients+1).Pluck("user_id", &ids).Error
		if err != nil {
			return nil, err
		}
		// Check each source before intersecting: truncation must never look
		// like a complete, smaller audience when the sources are combined.
		if len(ids) > maxRewardRecipients {
			return nil, ErrRewardRecipientsLimit
		}
		for _, id := range ids {
			candidates[id] = struct{}{}
		}
	}
	if filter.Audience == "both" && len(candidates) == 0 {
		return []int{}, nil
	}
	if filter.Audience != "topup" {
		var ids []int
		err := LOG_DB.Model(&Log{}).Distinct("user_id").
			Where("site_id = ? AND type = ? AND created_at >= ? AND created_at < ?", 0, LogTypeConsume, filter.StartTime, filter.EndTime).
			Limit(maxRewardRecipients+1).Pluck("user_id", &ids).Error
		if err != nil {
			return nil, err
		}
		if len(ids) > maxRewardRecipients {
			return nil, ErrRewardRecipientsLimit
		}
		if filter.Audience == "both" {
			intersection := make(map[int]struct{})
			for _, id := range ids {
				if _, exists := candidates[id]; exists {
					intersection[id] = struct{}{}
				}
			}
			candidates = intersection
		} else {
			for _, id := range ids {
				candidates[id] = struct{}{}
			}
		}
	}
	if len(candidates) > maxRewardRecipients {
		return nil, ErrRewardRecipientsLimit
	}
	ids := make([]int, 0, len(candidates))
	for id := range candidates {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	eligible := make([]int, 0, len(ids))
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		var batch []int
		if err := DB.Model(&User{}).Where("id IN ? AND site_id = ? AND status = ?", ids[start:end], 0, common.UserStatusEnabled).
			Order("id").Pluck("id", &batch).Error; err != nil {
			return nil, err
		}
		eligible = append(eligible, batch...)
	}
	return eligible, nil
}

func PreviewRewardRecipients(filter RewardAudienceFilter) (*RewardRecipientPreview, error) {
	ids, err := rewardRecipientIDs(filter)
	if err != nil {
		return nil, err
	}
	return &RewardRecipientPreview{RecipientCount: len(ids)}, nil
}

func (input *RewardCampaignInput) normalizeAndValidate() error {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(input.Title) < 1 || utf8.RuneCountInString(input.Title) > 120 || utf8.RuneCountInString(input.Description) > 1000 {
		return ErrInvalidRewardCampaign
	}
	if len(input.RequestID) < 1 || len(input.RequestID) > 64 {
		return ErrInvalidRewardCampaign
	}
	for _, char := range input.RequestID {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
			return ErrInvalidRewardCampaign
		}
	}
	if input.Type != RewardTypeFixed && input.Type != RewardTypeRandom {
		return ErrInvalidRewardCampaign
	}
	if input.MinQuota <= 0 || input.MaxQuota < input.MinQuota || input.MaxQuota > common.MaxWalletQuota {
		return ErrInvalidRewardCampaign
	}
	if input.Type == RewardTypeFixed && input.MinQuota != input.MaxQuota {
		return ErrInvalidRewardCampaign
	}
	if input.ExpiresInDays < 1 || input.ExpiresInDays > 3650 {
		return ErrInvalidRewardCampaign
	}
	if !input.Continuous && input.ExpectedRecipientCount != nil && (*input.ExpectedRecipientCount < 0 || *input.ExpectedRecipientCount > maxRewardRecipients) {
		return ErrInvalidRewardCampaign
	}
	return validateRewardAudience(RewardAudienceFilter{Audience: input.Audience, StartTime: input.StartTime, EndTime: input.EndTime})
}

func (campaign *RewardCampaign) matchesRequest(input RewardCampaignInput, createdBy int) bool {
	return campaign.CreatedBy == createdBy && campaign.Title == input.Title && campaign.Description == input.Description &&
		campaign.Type == input.Type && campaign.MinQuota == input.MinQuota && campaign.MaxQuota == input.MaxQuota &&
		campaign.Audience == input.Audience && campaign.StartTime == input.StartTime && campaign.EndTime == input.EndTime &&
		campaign.ExpiresInDays == input.ExpiresInDays && campaign.Continuous == input.Continuous
}

func CreateRewardCampaign(input RewardCampaignInput, createdBy int) (*RewardCampaign, error) {
	if createdBy <= 0 {
		return nil, ErrInvalidRewardCampaign
	}
	if err := input.normalizeAndValidate(); err != nil {
		return nil, err
	}
	if err := requireMainRewardUser(DB, createdBy); err != nil {
		return nil, err
	}
	var existing RewardCampaign
	err := DB.Where("request_id = ?", input.RequestID).First(&existing).Error
	if err == nil {
		if !existing.matchesRequest(input, createdBy) {
			return nil, ErrRewardRequestConflict
		}
		items := []RewardCampaign{existing}
		if err := populateRewardDistributionStatuses(items); err != nil {
			return nil, err
		}
		return &items[0], nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if input.Continuous {
		return createContinuousRewardCampaign(input, createdBy)
	}
	ids, err := rewardRecipientIDs(RewardAudienceFilter{Audience: input.Audience, StartTime: input.StartTime, EndTime: input.EndTime})
	if err != nil {
		return nil, err
	}
	if input.ExpectedRecipientCount != nil && len(ids) != *input.ExpectedRecipientCount {
		return nil, ErrRewardAudienceChanged
	}
	if len(ids) == 0 {
		return nil, ErrRewardRecipientsEmpty
	}
	if input.MaxQuota > common.MaxWalletQuota/len(ids) {
		return nil, ErrRewardRecipientsLimit
	}
	campaign := &RewardCampaign{
		Title: input.Title, Description: input.Description, Type: input.Type,
		MinQuota: input.MinQuota, MaxQuota: input.MaxQuota, Audience: input.Audience,
		StartTime: input.StartTime, EndTime: input.EndTime, RequestID: input.RequestID,
		CreatedBy: createdBy, ExpiresInDays: input.ExpiresInDays, RecipientCount: len(ids),
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		// The audience was queried before this transaction. Recheck its user
		// rows under locks so disabling/deleting an account cannot race issuance.
		// IDs are sorted, giving concurrent issuers a consistent lock order.
		for start := 0; start < len(ids); start += 500 {
			end := min(start+500, len(ids))
			var activeUsers []User
			if err := lockForUpdate(tx).Select("id").
				Where("id IN ? AND site_id = ? AND status = ?", ids[start:end], 0, common.UserStatusEnabled).
				Order("id").Find(&activeUsers).Error; err != nil {
				return err
			}
			if len(activeUsers) != end-start {
				return ErrRewardAudienceChanged
			}
		}
		// Start the validity period after waiting for recipient locks. Every
		// recipient receives the same immutable deadline, including on retries.
		campaign.CreatedAt = common.GetTimestamp()
		campaign.ExpiresAt = campaign.CreatedAt + int64(input.ExpiresInDays)*86400
		if err := tx.Create(campaign).Error; err != nil {
			return err
		}
		// Keep each insert below SQLite's bind-parameter limit as well as the
		// packet limits on MySQL. All batches still share one transaction.
		for start := 0; start < len(ids); start += 50 {
			end := min(start+50, len(ids))
			rewards := make([]UserReward, 0, end-start)
			for _, id := range ids[start:end] {
				rewards = append(rewards, UserReward{
					CampaignID: campaign.Id, UserID: id, Title: input.Title, Description: input.Description,
					Type: input.Type, MinQuota: input.MinQuota, MaxQuota: input.MaxQuota,
					Status: RewardStatusPending, CreatedAt: campaign.CreatedAt, ExpiresAt: campaign.ExpiresAt,
				})
			}
			if err := tx.Create(&rewards).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// A simultaneous retry may have won the unique request ID. Read only
		// after rollback, which is also safe after PostgreSQL constraint errors.
		if lookupErr := DB.Where("request_id = ?", input.RequestID).First(&existing).Error; lookupErr == nil {
			if !existing.matchesRequest(input, createdBy) {
				return nil, ErrRewardRequestConflict
			}
			return &existing, nil
		}
		return nil, err
	}
	campaign.DistributionStatus = "legacy"
	return campaign, nil
}

func rewardPagination(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	// Reject huge offsets before int multiplication can wrap.
	if page > 1000000 {
		page = 1000000
	}
	return (page - 1) * size, size
}

func ListRewardCampaigns(page, size int) ([]RewardCampaign, int64, error) {
	items := make([]RewardCampaign, 0)
	var total int64
	if err := DB.Model(&RewardCampaign{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limit := rewardPagination(page, size)
	if err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return items, total, nil
	}
	if err := populateRewardClaimStatistics(items); err != nil {
		return nil, 0, err
	}
	if err := populateRewardDistributionStatuses(items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func requireMainRewardUser(tx *gorm.DB, userID int) error {
	var user User
	err := tx.Select("id").Where("id = ? AND site_id = ? AND status = ?", userID, 0, common.UserStatusEnabled).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRewardUserUnavailable
	}
	return err
}

func ListUserRewards(userID int, status string, rewardType string, page, size int) ([]UserReward, int64, error) {
	if status != "" && status != RewardStatusPending && status != RewardStatusClaimed && status != RewardStatusExpired {
		return nil, 0, ErrInvalidRewardStatus
	}
	if rewardType != "" && rewardType != RewardTypeFixed && rewardType != RewardTypeRandom {
		return nil, 0, ErrInvalidRewardStatus
	}
	if err := requireMainRewardUser(DB, userID); err != nil {
		return nil, 0, err
	}
	now := common.GetTimestamp()
	query := DB.Model(&UserReward{}).Where("user_id = ?", userID)
	switch status {
	case RewardStatusPending:
		query = query.Where("status = ? AND (expires_at = 0 OR expires_at > ?)", RewardStatusPending, now)
	case RewardStatusExpired:
		query = query.Where("status = ? AND expires_at > 0 AND expires_at <= ?", RewardStatusPending, now)
	case RewardStatusClaimed:
		query = query.Where("status = ?", RewardStatusClaimed)
	}
	if rewardType != "" {
		query = query.Where("type = ?", rewardType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]UserReward, 0)
	offset, limit := rewardPagination(page, size)
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		if items[i].Status == RewardStatusPending && items[i].ExpiresAt > 0 && items[i].ExpiresAt <= now {
			items[i].Status = RewardStatusExpired
		}
	}
	return items, total, nil
}

func GetUserRewardSummary(userID int) (*RewardSummary, error) {
	if err := requireMainRewardUser(DB, userID); err != nil {
		return nil, err
	}
	var groups []struct {
		Type  string
		Count int64
		Quota int64
	}
	err := DB.Model(&UserReward{}).Where("user_id = ? AND status = ?", userID, RewardStatusPending).
		Where("expires_at = 0 OR expires_at > ?", common.GetTimestamp()).
		Select("type, COUNT(*) AS count, SUM(CASE WHEN type = 'fixed' THEN min_quota ELSE 0 END) AS quota").Group("type").Scan(&groups).Error
	if err != nil {
		return nil, err
	}
	summary := &RewardSummary{}
	for _, group := range groups {
		if group.Type == RewardTypeFixed {
			if group.Quota < 0 || group.Quota > common.MaxWalletQuota {
				return nil, ErrRewardRecipientsLimit
			}
			summary.FixedCount = group.Count
			summary.FixedQuota = group.Quota
		}
		if group.Type == RewardTypeRandom {
			summary.RandomCount = group.Count
		}
	}
	summary.PendingCount = summary.FixedCount + summary.RandomCount
	return summary, nil
}

func ClaimUserReward(userID, rewardID int) (*UserReward, bool, error) {
	return claimUserReward(userID, rewardID, common.GetTimestamp)
}

// claimUserReward checks eligibility against the server clock after acquiring
// locks, before recording the claim, and after crediting the wallet. The last
// check authorizes the transaction to commit; it is not the physical commit time.
func claimUserReward(userID, rewardID int, now func() int64) (*UserReward, bool, error) {
	if rewardID <= 0 {
		return nil, false, ErrRewardNotFound
	}
	var reward UserReward
	var cacheGeneration string
	newClaim := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := requireMainRewardUser(lockForUpdate(tx), userID); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", rewardID, userID).First(&reward).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRewardNotFound
			}
			return err
		}
		if reward.Status == RewardStatusClaimed {
			return nil
		}
		// Read the clock only after obtaining both locks, since a request may
		// have waited beyond its deadline. Legacy rewards use zero for no expiry.
		if reward.ExpiresAt > 0 && reward.ExpiresAt <= now() {
			return ErrRewardExpired
		}
		if reward.Status != RewardStatusPending || reward.MinQuota <= 0 || reward.MaxQuota < reward.MinQuota || reward.MaxQuota > common.MaxWalletQuota {
			return ErrInvalidRewardCampaign
		}
		reward.QuotaAwarded = reward.MinQuota
		switch reward.Type {
		case RewardTypeFixed:
			if reward.MinQuota != reward.MaxQuota {
				return ErrInvalidRewardCampaign
			}
		case RewardTypeRandom:
			if reward.MaxQuota > reward.MinQuota {
				draw, err := rand.Int(rand.Reader, big.NewInt(int64(reward.MaxQuota-reward.MinQuota)+1))
				if err != nil {
					return err
				}
				reward.QuotaAwarded += int(draw.Int64())
			}
		default:
			return ErrInvalidRewardCampaign
		}
		reward.ClaimedAt = now()
		if reward.ExpiresAt > 0 && reward.ExpiresAt <= reward.ClaimedAt {
			return ErrRewardExpired
		}
		reward.Status = RewardStatusClaimed
		result := tx.Model(&UserReward{}).Where("id = ? AND user_id = ? AND status = ?", reward.Id, userID, RewardStatusPending).
			Where("expires_at = 0 OR expires_at > ?", reward.ClaimedAt).
			Updates(map[string]interface{}{"status": reward.Status, "claimed_at": reward.ClaimedAt, "quota_awarded": reward.QuotaAwarded})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			if reward.ExpiresAt > 0 {
				return ErrRewardExpired
			}
			return ErrRewardNotFound
		}
		if err := creditTopUpQuota(tx, userID, reward.QuotaAwarded, nil, &cacheGeneration); err != nil {
			return err
		}
		// Quota cache generation capture may wait on Redis. If that crosses
		// the deadline, roll back both the credit and the claim before commit.
		if reward.ExpiresAt > 0 && reward.ExpiresAt <= now() {
			return ErrRewardExpired
		}
		newClaim = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if newClaim {
		syncCreditUserQuotaCache(userID, reward.QuotaAwarded, "reward claim", cacheGeneration)
	}
	return &reward, newClaim, nil
}
