package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserNameMaxLength = 20

var ErrStaleUserUpdate = errors.New("user changed since it was loaded")

var userSortColumns = map[string]string{
	"id":            "id",
	"username":      "username",
	"quota":         "quota",
	"group":         "group",
	"created_at":    "created_at",
	"last_login_at": "last_login_at",
}

type UserSortOptions struct {
	SortBy    string
	SortOrder string
}

func NewUserSortOptions(sortBy string, sortOrder string) UserSortOptions {
	normalizedSortBy := strings.ToLower(strings.TrimSpace(sortBy))
	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	if _, ok := userSortColumns[normalizedSortBy]; !ok {
		normalizedSortBy = "id"
		normalizedSortOrder = "desc"
	} else if normalizedSortOrder != "asc" {
		normalizedSortOrder = "desc"
	}

	return UserSortOptions{
		SortBy:    normalizedSortBy,
		SortOrder: normalizedSortOrder,
	}
}

func (options UserSortOptions) Apply(query *gorm.DB) *gorm.DB {
	columnName, ok := userSortColumns[options.SortBy]
	if !ok {
		columnName = "id"
	}
	q := query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: columnName},
		Desc:   options.SortOrder != "asc",
	})
	if columnName != "id" {
		q = q.Order(clause.OrderByColumn{
			Column: clause.Column{Name: "id"},
			Desc:   true,
		})
	}
	return q
}

func resolveUserSortOptions(sortOptions []UserSortOptions) UserSortOptions {
	if len(sortOptions) == 0 {
		return NewUserSortOptions("", "")
	}
	return sortOptions[0]
}

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id int `json:"id"`
	// SiteId is the white-label sub-site this user belongs to (0 = main site).
	// Uniqueness of username/email is scoped per site via the composite indexes below,
	// so different sub-sites may have users with the same username/email.
	SiteId           int     `json:"site_id" gorm:"type:int;default:0;index;uniqueIndex:idx_users_site_username,priority:1;index:idx_users_site_email,priority:1"`
	Username         string  `json:"username" gorm:"index;uniqueIndex:idx_users_site_username,priority:2" validate:"max=20"`
	Password         string  `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	OriginalPassword string  `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName      string  `json:"display_name" gorm:"index" validate:"max=20"`
	Role             int     `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status           int     `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email            string  `json:"email" gorm:"index;index:idx_users_site_email,priority:2" validate:"max=50"`
	GitHubId         string  `json:"github_id" gorm:"column:github_id;index"`
	DiscordId        string  `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId           string  `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId         string  `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId       string  `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode string  `json:"verification_code" gorm:"-:all"`                         // this field is only for Email verification, don't save it to database!
	AccessToken      *string `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	Quota            int     `json:"quota" gorm:"type:int;default:0"`
	UsedQuota        int     `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount     int     `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group            string  `json:"group" gorm:"type:varchar(64);default:'default'"`
	AffCode          string  `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount         int     `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota         int     `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota  int     `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId        int     `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	// AffCommissionPercent is an optional per-inviter override (0-100) for the recharge
	// commission rate. nil inherits the global common.AffRechargeCommissionPercent; a non-nil
	// value (including 0) takes precedence for this inviter's referral commission payouts.
	AffCommissionPercent *float64 `json:"aff_commission_percent" gorm:"column:aff_commission_percent"`
	// AffCashSettled marks an inviter as a cash-settled promoter: their referral payouts are
	// handled off-platform as cash, computed from the commission ledger. When true: the one-time
	// first bonus (QuotaForInviter) is NOT credited to this inviter, and recharge commission is
	// still recorded in the AffiliateCommission ledger (the cash basis) but NOT credited to the
	// inviter's aff_quota/aff_history. The invitee's own bonus (QuotaForInvitee) is unaffected, as
	// is aff_count. No gorm default tag (see SubscriptionPlan.Enabled) — false is the business
	// default and a bool default tag triggers repeated AutoMigrate ALTER churn across MySQL/PG.
	AffCashSettled bool `json:"aff_cash_settled" gorm:"column:aff_cash_settled"`
	// AffCashPaid is the authoritative running total (quota units) of off-platform cash already
	// settled to this inviter. It is only ever advanced by RecordAffiliateCashPayout via a capped
	// conditional UPDATE (so concurrent settlements cannot over-pay without SELECT ... FOR UPDATE,
	// which SQLite rejects); the AffiliateCashPayout rows are the human-readable settlement history.
	AffCashPaid    int64          `json:"aff_cash_paid" gorm:"type:bigint;not null;default:0;column:aff_cash_paid"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	LinuxDOId      string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting        string         `json:"setting" gorm:"type:text;column:setting"`
	Remark         string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt      int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt    int64          `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	AuthVersion    int64          `json:"-" gorm:"type:bigint;not null;default:1;column:auth_version"`
	// AdminPermissions is a transient view of the fine-grained admin authz matrix
	// (module -> action -> allowed), populated from the casbin policy on read and
	// consumed by updateAdminPermissionsForUserInTx on write. Never persisted here.
	AdminPermissions map[string]map[string]bool `json:"admin_permissions,omitempty" gorm:"-:all"`
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:          user.Id,
		SiteId:      user.SiteId,
		Group:       user.Group,
		Quota:       user.Quota,
		Status:      user.Status,
		Role:        user.Role,
		Username:    user.Username,
		Setting:     user.Setting,
		Email:       user.Email,
		AuthVersion: user.AuthVersion,
		CacheSchema: userCacheSchemaVersion,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

// UpdateUserAccessToken rotates a dashboard personal access token without
// writing a stale user snapshot back over concurrently updated fields.
func UpdateUserAccessToken(id int, token string) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	result := DB.Model(&User{}).Where("id = ?", id).Update("access_token", token)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

func UpdateUserSetting(userId int, setting dto.UserSetting) error {
	if userId == 0 {
		return errors.New("id 为空！")
	}
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		return err
	}
	settingValue := string(settingBytes)
	if err = DB.Model(&User{}).Where("id = ?", userId).Update("setting", settingValue).Error; err != nil {
		return err
	}
	return updateUserSettingCache(userId, settingValue)
}

// userBindColumns 允许通过 UpdateUserBindColumn 更新的第三方账号绑定列白名单。
// 列名只可能来自代码内部的 provider 实现，白名单是防御纵深，不依赖调用方自律。
var userBindColumns = map[string]bool{
	"github_id":   true,
	"discord_id":  true,
	"oidc_id":     true,
	"linux_do_id": true,
	"wechat_id":   true,
}

func claimUserExternalIdentitiesWithTx(tx *gorm.DB, user *User) error {
	if tx == nil || user == nil || user.Id <= 0 {
		return errors.New("invalid user external identities")
	}
	for _, source := range externalIdentityUserSources {
		subject := strings.TrimSpace(source.Subject(user))
		if subject == "" {
			continue
		}
		if err := ClaimExternalIdentityWithTx(tx, source.Provider, subject, user.Id); err != nil {
			return err
		}
	}
	return nil
}

func normalizeUserExternalIdentities(user *User) error {
	if user == nil {
		return errors.New("invalid user external identities")
	}
	for _, subject := range []*string{
		&user.GitHubId,
		&user.DiscordId,
		&user.OidcId,
		&user.LinuxDOId,
		&user.WeChatId,
		&user.TelegramId,
	} {
		if *subject == "" {
			continue
		}
		normalized, err := NormalizeExternalIdentitySubject(*subject)
		if err != nil {
			return err
		}
		*subject = normalized
	}
	return nil
}

// BindUserExternalIdentityWithTx replaces one built-in provider binding and
// its durable ownership claim in the same transaction.
func BindUserExternalIdentityWithTx(tx *gorm.DB, userId int, column string, value string) error {
	provider, ok := externalIdentityProviderForUserColumn(column)
	if tx == nil || userId <= 0 || !ok {
		return errors.New("invalid user external identity binding")
	}
	var err error
	value, err = NormalizeExternalIdentitySubject(value)
	if err != nil {
		return err
	}
	var currentValue string
	if err := tx.Model(&User{}).Where("id = ?", userId).Pluck(column, &currentValue).Error; err != nil {
		return err
	}
	if strings.TrimSpace(currentValue) == value {
		return ClaimExternalIdentityWithTx(tx, provider, value, userId)
	}
	if err := ReleaseExternalIdentityWithTx(tx, provider, userId); err != nil {
		return err
	}
	if err := ClaimExternalIdentityWithTx(tx, provider, value, userId); err != nil {
		return err
	}
	result := tx.Model(&User{}).Where("id = ?", userId).Update(column, value)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserBindColumn 第三方账号绑定字段的专用更新。
// 绑定操作必须只写绑定列：若改为“读取完整用户 → 改一个字段 → 整体更新”，
// 读快照期间并发发生的封禁、降权或分组变更会被旧快照覆盖恢复。
// 角色、状态、分组只允许通过各自带锁/CAS 的专用方法修改。
func UpdateUserBindColumn(userId int, column string, value string) error {
	if userId <= 0 {
		return errors.New("id 为空！")
	}
	if !userBindColumns[column] {
		return fmt.Errorf("invalid user bind column: %s", column)
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return BindUserExternalIdentityWithTx(tx, userId, column, value)
	})
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
	defaultConfig := map[string]interface{}{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]interface{}{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]interface{}{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]interface{}{
		"enabled":  true,
		"topup":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":    true,
			"channel":    true,
			"models":     true,
			"redemption": true,
			"user":       true,
			"setting":    false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]interface{}{
			"enabled":    true,
			"channel":    true,
			"models":     true,
			"redemption": true,
			"user":       true,
			"setting":    true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := common.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil.
// The lookup is scoped to the given siteId so usernames/emails are only unique per sub-site.
func CheckUserExistOrDeleted(username string, email string, siteId int) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	// site_id must be matched explicitly (struct-query would drop site_id=0, the main site).
	var err error
	email = NormalizeEmail(email)
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ? AND site_id = ?", username, siteId).Error
	} else {
		err = DB.Unscoped().First(&user, "(username = ? OR LOWER(email) = ?) AND site_id = ?", username, email, siteId).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func emailQuery(tx *gorm.DB, email string, siteId int) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return tx.Unscoped().Model(&User{}).
		Where("LOWER(email) = ? AND site_id = ?", NormalizeEmail(email), siteId)
}

func IsEmailAvailable(email string, siteId int, excludeUserID int) (bool, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return true, nil
	}
	query := emailQuery(DB, email, siteId)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func EnsureEmailAvailable(email string, siteId int, excludeUserID int) error {
	available, err := IsEmailAvailable(email, siteId, excludeUserID)
	if err != nil {
		return err
	}
	if !available {
		return ErrEmailAlreadyTaken
	}
	return nil
}

// withNormalizedEmailLock serializes same-site writers targeting one normalized
// email. PostgreSQL uses a transaction advisory lock, MySQL uses a locking read,
// and SQLite relies on its single-writer transaction model.
func withNormalizedEmailLock(tx *gorm.DB, email string, siteId int, fn func(tx *gorm.DB) error) error {
	email = NormalizeEmail(email)
	if email == "" {
		return fn(tx)
	}
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		lockKey := fmt.Sprintf("%d:%s", siteId, email)
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey).Error; err != nil {
			return err
		}
	case common.UsingMainDatabase(common.DatabaseTypeMySQL):
		var ids []int
		if err := tx.Raw("SELECT id FROM users WHERE site_id = ? AND email = ? FOR UPDATE", siteId, email).Scan(&ids).Error; err != nil {
			return err
		}
	}
	return fn(tx)
}

func ensureEmailAvailableWithTx(tx *gorm.DB, email string, siteId int, excludeUserID int) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	query := emailQuery(tx, email, siteId)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrEmailAlreadyTaken
	}
	return nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

// GetAllUsers returns a page of users. When siteScope != SiteScopeAll the result is
// restricted to that site_id (sub-site admins); SiteScopeAll keeps the global view
// (main-site admins / root). site_id is filtered with an explicit condition so the
// main site (site_id=0) is matched correctly.
func GetAllUsers(pageInfo *common.PageInfo, siteScope int, sortOptions ...UserSortOptions) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	countQuery := tx.Unscoped().Model(&User{})
	if siteScope != SiteScopeAll {
		countQuery = countQuery.Where("site_id = ?", siteScope)
	}
	err = countQuery.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	order := resolveUserSortOptions(sortOptions)
	dataQuery := order.Apply(tx.Unscoped()).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password", "access_token")
	if siteScope != SiteScopeAll {
		dataQuery = dataQuery.Where("site_id = ?", siteScope)
	}
	err = dataQuery.Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// SearchUsers searches users. When siteScope != SiteScopeAll the result is restricted
// to that site_id (sub-site admins); SiteScopeAll keeps the global view. site_id is
// filtered with an explicit condition so the main site (site_id=0) is matched correctly.
func SearchUsers(keyword string, group string, role *int, status *int, startIdx int, num int, siteScope int, sortOptions ...UserSortOptions) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})
	if siteScope != SiteScopeAll {
		query = query.Where("site_id = ?", siteScope)
	}

	// 构建搜索条件
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"
	likeArgs := []interface{}{"%" + keyword + "%", "%" + keyword + "%", "%" + keyword + "%"}

	// 尝试将关键字转换为整数ID
	keywordInt, err := strconv.Atoi(keyword)
	if err == nil {
		// 如果是数字，同时搜索ID和其他字段
		likeCondition = "id = ? OR " + likeCondition
		likeArgs = append([]interface{}{keywordInt}, likeArgs...)
	}

	query = query.Where("("+likeCondition+")", likeArgs...)
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	if role != nil {
		query = query.Where("role = ?", *role)
	}
	if status != nil {
		if *status == -1 {
			query = query.Where("deleted_at IS NOT NULL")
		} else {
			query = query.Where("deleted_at IS NULL").Where("status = ?", *status)
		}
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	order := resolveUserSortOptions(sortOptions)
	err = order.Apply(query.Omit("password", "access_token")).Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password", "access_token").First(&user, "id = ?", id).Error
	}
	return &user, err
}

// GetUserSiteId returns the sub-site the user belongs to (0 = main site). The
// stateless dashboard auth flow validates against UserBase, which does not carry
// site_id, so the auth middleware resolves the operator's site from the
// authoritative user row here. First (not Find) so a missing row surfaces as an
// error and the caller can fail closed instead of defaulting to main site 0.
func GetUserSiteId(id int) (int, error) {
	if id == 0 {
		return 0, errors.New("id 为空！")
	}
	var user User
	if err := DB.Select("site_id").First(&user, "id = ?", id).Error; err != nil {
		return 0, err
	}
	return user.SiteId, nil
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.HardDelete()
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(common.QuotaFromFloat(common.QuotaPerUnit)))
	}

	// Single conditional UPDATE: the aff_quota >= ? guard makes concurrent
	// transfers safe, and touching only the two balance columns can never
	// clobber a concurrent consumption update to quota/used_quota/request_count
	// (a stale full-row Save here previously could restore pre-consumption
	// values and erase usage).
	maxCurrentQuota, err := topUpQuotaMaxCurrent(quota)
	if err != nil {
		return err
	}
	result := DB.Model(&User{}).
		Where("id = ? AND aff_quota >= ? AND quota <= ?", user.Id, quota, maxCurrentQuota).
		Updates(map[string]interface{}{
			"aff_quota": gorm.Expr("aff_quota - ?", quota),
			"quota":     gorm.Expr("quota + ?", quota),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var current User
		if err := DB.Select("quota", "aff_quota").Where("id = ?", user.Id).First(&current).Error; err != nil {
			return err
		}
		if current.AffQuota < quota {
			return errors.New("邀请额度不足！")
		}
		return ErrTopUpQuotaLimitExceeded
	}
	syncCreditUserQuotaCache(user.Id, quota, "affiliate quota transfer")
	return nil
}

func (user *User) prepareForInsert(tx *gorm.DB) error {
	if err := normalizeUserExternalIdentities(user); err != nil {
		return err
	}
	user.Email = NormalizeEmail(user.Email)
	if err := ensureEmailAvailableWithTx(tx, user.Email, user.SiteId, 0); err != nil {
		return err
	}
	if user.Password == "" {
		return nil
	}
	var err error
	user.Password, err = common.Password2Hash(user.Password)
	return err
}

func (user *User) Insert(inviterId int) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, user.Email, user.SiteId, func(tx *gorm.DB) error {
			if err := user.prepareForInsert(tx); err != nil {
				return err
			}
			user.Quota = common.QuotaForNewUser
			user.AffCode = common.GetRandomString(4)
			// LemonHub defers referral rewards until the invitee's first paid
			// top-up, but the relationship itself must be persisted at signup.
			if inviterId != 0 {
				user.InviterId = inviterId
			}

			// 初始化用户设置，包括默认的边栏配置
			if user.Setting == "" {
				defaultSetting := dto.UserSetting{}
				// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
				user.SetSetting(defaultSetting)
			}

			if err := tx.Create(user).Error; err != nil {
				return err
			}
			return claimUserExternalIdentitiesWithTx(tx, user)
		})
	}); err != nil {
		return err
	}

	user.finishInsert(inviterId)
	return nil
}

func (user *User) finishInsert(inviterId int) {
	// 用户创建成功后，根据角色初始化边栏配置
	// 需要重新获取用户以确保有正确的ID和Role
	// 必须按 site_id 过滤，否则可能取到其它子站的同名用户（用户名仅在站内唯一）。
	var createdUser User
	if err := DB.Where("username = ? AND site_id = ?", user.Username, user.SiteId).First(&createdUser).Error; err == nil {
		// 生成基于角色的默认边栏配置
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	// Referral rewards are deferred: invitee/inviter fixed bonuses and recharge commission
	// are settled on the invitee's FIRST successful real-payment top-up
	// (see model/affiliate.go SettleReferralOnTopUp). Registration only records the
	// inviter relationship via user.InviterId.
	if inviterId != 0 {
		RecordLog(user.Id, LogTypeSystem, "通过邀请码注册，邀请奖励将在首次充值后发放")
	}
}

func (user *User) FinishInsert(inviterId int) {
	user.finishInsert(inviterId)
}

// InsertWithTx inserts a new user within an existing transaction.
// This is used for OAuth registration where user creation and binding need to be atomic.
// Post-creation tasks (sidebar config, logs, inviter rewards) are handled after the transaction commits.
func (user *User) InsertWithTx(tx *gorm.DB, inviterId int) error {
	return withNormalizedEmailLock(tx, user.Email, user.SiteId, func(tx *gorm.DB) error {
		if err := user.prepareForInsert(tx); err != nil {
			return err
		}
		user.Quota = common.QuotaForNewUser
		user.AffCode = common.GetRandomString(4)
		if inviterId != 0 {
			user.InviterId = inviterId
		}

		// 初始化用户设置
		if user.Setting == "" {
			defaultSetting := dto.UserSetting{}
			user.SetSetting(defaultSetting)
		}

		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return claimUserExternalIdentitiesWithTx(tx, user)
	})
}

// FinalizeOAuthUserCreation performs post-transaction tasks for OAuth user creation.
// This should be called after the transaction commits successfully.
func (user *User) FinalizeOAuthUserCreation(inviterId int) {
	// 用户创建成功后，根据角色初始化边栏配置
	var createdUser User
	if err := DB.Where("id = ?", user.Id).First(&createdUser).Error; err == nil {
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	// Referral rewards are deferred to the invitee's first successful top-up; see Insert.
	if inviterId != 0 {
		RecordLog(user.Id, LogTypeSystem, "通过邀请码注册，邀请奖励将在首次充值后发放")
	}
}

func (user *User) Update(updatePassword bool) error {
	var previousAuthVersion int64
	if err := DB.Model(&User{}).Where("id = ?", user.Id).Select("auth_version").Find(&previousAuthVersion).Error; err != nil {
		return err
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return user.UpdateWithTx(tx, updatePassword)
	}); err != nil {
		return err
	}
	if err := updateUserCache(*user); err != nil {
		return err
	}
	if user.AuthVersion > previousAuthVersion {
		_, err := RevokeAllUserSessions(user.Id, "user_security_changed")
		return err
	}
	return nil
}

func (user *User) UpdateWithTx(tx *gorm.DB, updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	current := User{}
	if err = lockForUpdate(tx).First(&current, user.Id).Error; err != nil {
		return err
	}
	// Full User snapshots carry the authentication version that was observed by
	// the caller. Refuse to write one after a concurrent ban, role/group change,
	// or site transfer; otherwise its non-zero fields could silently restore the
	// old authorization state. Purpose-built partial updates leave AuthVersion
	// zero and only write their explicitly populated fields.
	if newUser.AuthVersion > 0 && newUser.AuthVersion != current.AuthVersion {
		return ErrStaleUserUpdate
	}
	// Updates(struct) ignores zero values. Match that behavior when deciding
	// whether this request actually changes authentication-sensitive state;
	// partial self-profile updates intentionally leave role/status/group empty.
	authChanged := (updatePassword && current.Password != newUser.Password) ||
		(newUser.Role != 0 && current.Role != newUser.Role) ||
		(newUser.Status != 0 && current.Status != newUser.Status) ||
		(newUser.Group != "" && current.Group != newUser.Group)
	if authChanged {
		newUser.AuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
	}
	if err = tx.Model(&current).Omit(
		"site_id",
		"access_token",
		"quota",
		"used_quota",
		"request_count",
		"aff_count",
		"aff_quota",
		"aff_history",
		"auth_version",
		"github_id",
		"discord_id",
		"oidc_id",
		"linux_do_id",
		"wechat_id",
		"telegram_id",
	).Updates(newUser).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

func (user *User) Edit(updatePassword bool, updateAffCommission bool, updateAffCashSettled bool) error {
	var previousAuthVersion int64
	if err := DB.Model(&User{}).Where("id = ?", user.Id).Select("auth_version").Find(&previousAuthVersion).Error; err != nil {
		return err
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return user.EditWithTx(tx, updatePassword, updateAffCommission, updateAffCashSettled)
	}); err != nil {
		return err
	}
	if err := updateUserCache(*user); err != nil {
		return err
	}
	if user.AuthVersion > previousAuthVersion {
		_, err := RevokeAllUserSessions(user.Id, "user_security_changed")
		return err
	}
	return nil
}

func (user *User) EditWithTx(tx *gorm.DB, updatePassword bool, updateAffCommission bool, updateAffCashSettled bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	updates := map[string]interface{}{
		"username":     newUser.Username,
		"display_name": newUser.DisplayName,
		"group":        newUser.Group,
		"remark":       newUser.Remark,
	}
	if updatePassword {
		updates["password"] = newUser.Password
	}
	// Only touch the per-user commission override when the caller indicates the field was present
	// in the request body: a non-nil value sets it, an explicit nil clears it back to NULL (inherit
	// the global rate). When the field is absent, the existing value is preserved so partial updates
	// from other clients do not silently wipe an admin-configured override.
	if updateAffCommission {
		updates["aff_commission_percent"] = newUser.AffCommissionPercent
	}
	// Only touch the cash-settled-promoter flag when the caller indicates the field was present in
	// the request body, so partial updates from other clients do not silently flip it back to false.
	if updateAffCashSettled {
		updates["aff_cash_settled"] = newUser.AffCashSettled
	}

	current := User{}
	if err = tx.First(&current, user.Id).Error; err != nil {
		return err
	}
	authChanged := (updatePassword && current.Password != newUser.Password) || current.Group != newUser.Group
	if authChanged {
		newUser.AuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
	}
	if err = tx.Model(&current).Updates(updates).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if provider, claimed := externalIdentityProviderForUserColumn(column); claimed {
			if err := ReleaseExternalIdentityWithTx(tx, provider, user.Id); err != nil {
				return err
			}
		}
		result := tx.Model(&User{}).Where("id = ?", user.Id).Update(column, "")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return updateUserCache(*user)
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	var nextAuthVersion int64
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		nextAuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		return tx.Delete(user).Error
	}); err != nil {
		return err
	}
	if err := publishCommittedUserAuthVersion(user.Id, nextAuthVersion); err != nil {
		return err
	}
	if _, err := RevokeAllUserSessions(user.Id, "user_deleted"); err != nil {
		return err
	}
	return invalidateUserCache(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	var tokens []Token
	var deletedAuthVersion int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		deletedAuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		if common.RedisEnabled {
			if err := tx.Unscoped().Select("id", commonKeyCol).Where("user_id = ?", user.Id).Find(&tokens).Error; err != nil {
				return err
			}
		}
		if err := deleteUserAuthenticationData(tx, user.Id); err != nil {
			return err
		}
		// Purge recorded raw request bodies alongside the user (see HardDeleteUserById).
		if err := tx.Where("user_id = ?", user.Id).Delete(&RequestBodyLog{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(user).Error
	})
	if err != nil {
		return err
	}
	if err := publishCommittedUserAuthVersion(user.Id, deletedAuthVersion); err != nil {
		common.SysError(fmt.Sprintf("failed to publish auth tombstone after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate token cache after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateUserCache(user.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate user cache after hard deleting user %d: %v", user.Id, err))
	}
	return nil
}

func deleteUserAuthenticationData(tx *gorm.DB, userId int) error {
	if err := releaseAllExternalIdentitiesWithTx(tx, userId); err != nil {
		return err
	}
	for _, authenticationData := range []any{
		&TwoFABackupCode{},
		&TwoFA{},
		&UserSession{},
		&AuthFlow{},
		&PasskeyCredential{},
		&Token{},
	} {
		if err := tx.Unscoped().Where("user_id = ?", userId).Delete(authenticationData).Error; err != nil {
			return err
		}
	}
	return deleteUserOAuthBindingsByUserId(tx, userId)
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return ErrUserEmptyCredentials
	}
	// find by username or email, scoped to the caller-provided site_id (set before
	// calling, e.g. from the request Host) so sub-sites have isolated accounts.
	err = DB.Where("(username = ? OR LOWER(email) = ?) AND site_id = ?", username, NormalizeEmail(username), user.SiteId).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if user.Password == "" {
		return ErrInvalidCredentials
	}
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return ErrInvalidCredentials
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail(siteId int) error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	// Explicit site_id condition: struct-query would drop site_id=0 (the main site).
	DB.Where("LOWER(email) = ? AND site_id = ?", NormalizeEmail(user.Email), siteId).First(user)
	return nil
}

func (user *User) FillUserByGitHubId(siteId int) error {
	return user.fillByExternalIdentity("github_id", user.GitHubId, siteId)
}

// UpdateGitHubId updates the user's GitHub ID (used for migration from login to numeric ID)
func (user *User) UpdateGitHubId(newGitHubId string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}
	if err := UpdateUserBindColumn(user.Id, "github_id", newGitHubId); err != nil {
		return err
	}
	user.GitHubId, _ = NormalizeExternalIdentitySubject(newGitHubId)
	return nil
}

func (user *User) FillUserByDiscordId(siteId int) error {
	return user.fillByExternalIdentity("discord_id", user.DiscordId, siteId)
}

func (user *User) FillUserByOidcId(siteId int) error {
	return user.fillByExternalIdentity("oidc_id", user.OidcId, siteId)
}

func (user *User) FillUserByWeChatId(siteId int) error {
	return user.fillByExternalIdentity("wechat_id", user.WeChatId, siteId)
}

func (user *User) FillUserByTelegramId(siteId int) error {
	err := user.fillByExternalIdentity("telegram_id", user.TelegramId, siteId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return err
}

func (user *User) fillByExternalIdentity(column, subject string, siteId int) error {
	if _, ok := externalIdentityProviderForUserColumn(column); !ok {
		return errors.New("invalid external identity column")
	}
	normalized, err := NormalizeExternalIdentitySubject(subject)
	if err != nil {
		return err
	}
	return DB.Where(column+" = ? AND site_id = ?", normalized, siteId).First(user).Error
}

// The Is*AlreadyTaken checks compare RowsAffected > 0 (not == 1): if duplicates
// ever exist (no unique index on these columns), the identifier must still
// report taken instead of silently allowing more duplicates.
func IsEmailAlreadyTaken(email string, siteId int) bool {
	email = NormalizeEmail(email)
	return email != "" && DB.Unscoped().Where("LOWER(email) = ? AND site_id = ?", email, siteId).Find(&User{}).RowsAffected > 0
}

// BindUserEmail sets the user's email after re-verifying same-site uniqueness
// INSIDE a transaction, so the check and the write cannot be separated by a
// concurrent bind of the same address. The duplicate probe is a locking read:
// on MySQL/InnoDB the next-key lock on the email index blocks a concurrent
// same-email bind until commit, and SQLite's single-writer model serializes the
// two transactions. (site_id, email) has no unique index, so this is the
// strongest cross-DB guard available without one; on PostgreSQL a residual
// phantom window remains. Deleted accounts still hold their address (Unscoped),
// matching IsEmailAlreadyTaken.
func BindUserEmail(userId int, email string) error {
	email = NormalizeEmail(email)
	if email == "" {
		return errors.New("邮箱地址为空！")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var self User
		if err := tx.First(&self, "id = ?", userId).Error; err != nil {
			return err
		}
		return withNormalizedEmailLock(tx, email, self.SiteId, func(tx *gorm.DB) error {
			if err := ensureEmailAvailableWithTx(tx, email, self.SiteId, userId); err != nil {
				return err
			}
			return tx.Model(&User{}).Where("id = ?", userId).Update("email", email).Error
		})
	})
	if err != nil {
		return err
	}
	return updateUserEmailCache(userId, email)
}

// BindUserEmailIfEmpty attaches an optional, externally supplied profile email
// without overwriting an email the user already chose. Duplicate same-site
// addresses are skipped rather than returned as an error so payment settlement
// callers can treat this as best-effort metadata enrichment.
func BindUserEmailIfEmpty(userId int, email string) (bool, error) {
	email = NormalizeEmail(email)
	if userId <= 0 || email == "" {
		return false, nil
	}

	updated := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var self User
		if err := tx.Select("id", "site_id", "email").Where("id = ?", userId).First(&self).Error; err != nil {
			return err
		}
		if NormalizeEmail(self.Email) != "" {
			return nil
		}

		return withNormalizedEmailLock(tx, email, self.SiteId, func(tx *gorm.DB) error {
			if err := ensureEmailAvailableWithTx(tx, email, self.SiteId, userId); err != nil {
				if errors.Is(err, ErrEmailAlreadyTaken) {
					return nil
				}
				return err
			}
			result := tx.Model(&User{}).
				Where("id = ? AND (email = '' OR email IS NULL)", userId).
				Update("email", email)
			if result.Error != nil {
				return result.Error
			}
			updated = result.RowsAffected == 1
			return nil
		})
	})
	if err != nil || !updated {
		return updated, err
	}
	return true, updateUserEmailCache(userId, email)
}

func IsWeChatIdAlreadyTaken(wechatId string, siteId int) bool {
	return isUserExternalIdentityTaken("wechat_id", wechatId, siteId)
}

func IsGitHubIdAlreadyTaken(githubId string, siteId int) bool {
	return isUserExternalIdentityTaken("github_id", githubId, siteId)
}

func IsDiscordIdAlreadyTaken(discordId string, siteId int) bool {
	return isUserExternalIdentityTaken("discord_id", discordId, siteId)
}

func IsOidcIdAlreadyTaken(oidcId string, siteId int) bool {
	return isUserExternalIdentityTaken("oidc_id", oidcId, siteId)
}

func IsTelegramIdAlreadyTaken(telegramId string, siteId int) bool {
	return isUserExternalIdentityTaken("telegram_id", telegramId, siteId)
}

func isUserExternalIdentityTaken(column, subject string, siteId int) bool {
	if _, ok := externalIdentityProviderForUserColumn(column); !ok {
		return true
	}
	normalized, err := NormalizeExternalIdentitySubject(subject)
	if err != nil {
		return false
	}
	return DB.Unscoped().Where(column+" = ? AND site_id = ?", normalized, siteId).Find(&User{}).RowsAffected > 0
}

// ResetUserPasswordByEmail resets the password for the account with the given email on
// the given sub-site. site_id MUST be matched explicitly: without it the update would
// rewrite (and the caller would return) the password of every same-email account across
// all sites — a cross-site account-takeover vector.
//
// The email must match EXACTLY ONE account on the site. (site_id, email) has no unique
// index, so races or legacy data can produce same-site duplicates; blind-updating them
// all would let whoever holds the reset token take over every duplicate at once.
func ResetUserPasswordByEmail(email string, password string, siteId int) error {
	email = NormalizeEmail(email)
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	var userId int
	if err = DB.Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, email, siteId, func(tx *gorm.DB) error {
			var ids []int
			if err := tx.Model(&User{}).
				Where("LOWER(email) = ? AND site_id = ?", email, siteId).
				Pluck("id", &ids).Error; err != nil {
				return err
			}
			switch len(ids) {
			case 0:
				return ErrEmailNotFound
			case 1:
				userId = ids[0]
			default:
				return ErrEmailAmbiguous
			}
			if _, err := IncrementUserAuthVersionWithTx(tx, userId); err != nil {
				return err
			}
			return tx.Model(&User{}).Where("id = ?", userId).Update("password", hashedPassword).Error
		})
	}); err != nil {
		return err
	}
	if err := PublishUserAuthCache(userId); err != nil {
		return err
	}
	_, err = RevokeAllUserSessions(userId, "password_reset")
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user := &User{}
	err := DB.Where("access_token = ?", token).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return user, nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	if !fromDB && common.RedisEnabled {
		return getUserQuotaCache(id)
	}
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup gets group from Redis first, falls back to DB if needed
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := RefreshUserGroupCache(id); err != nil {
					common.SysLog("failed to update user group cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		group, err := getUserGroupCache(id)
		if err == nil {
			return group, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error
	if err != nil {
		return "", err
	}

	return group, nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog("failed to update user setting cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	// can be nil setting
	var safeSetting sql.NullString
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&safeSetting).Error
	if err != nil {
		return settingMap, err
	}
	if safeSetting.Valid {
		setting = safeSetting.String
	} else {
		setting = ""
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return applyUserQuotaDelta(id, quota, db)
}

func increaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DecreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return applyUserQuotaDelta(id, -quota, db)
}

func decreaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta, false)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserLastLoginAt(id int) {
	if err := DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update user last_login_at: " + err.Error())
	}
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

// UpdateUserUsedQuota adjusts accumulated usage without changing request count.
func UpdateUserUsedQuota(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		return
	}
	if err := DB.Model(&User{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error; err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserQuotaUsedQuotaAndRequestCount(id int, quota int, usedQuota int, requestCount int) error {
	if quota == 0 && usedQuota == 0 && requestCount == 0 {
		return nil
	}

	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"quota":         gorm.Expr("quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", usedQuota),
			"request_count": gorm.Expr("request_count + ?", requestCount),
		},
	).Error
	if err != nil {
		common.SysLog("failed to batch update user quota, used quota and request count: " + err.Error())
	}
	return err
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog("failed to update user name cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string, siteId int) bool {
	return isUserExternalIdentityTaken("linux_do_id", linuxDOId, siteId)
}

func (user *User) FillUserByLinuxDOId(siteId int) error {
	return user.fillByExternalIdentity("linux_do_id", user.LinuxDOId, siteId)
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}
