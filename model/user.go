package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const UserNameMaxLength = 20

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
	AffCashSettled       bool           `json:"aff_cash_settled" gorm:"column:aff_cash_settled"`
	// AffCashPaid is the authoritative running total (quota units) of off-platform cash already
	// settled to this inviter. It is only ever advanced by RecordAffiliateCashPayout via a capped
	// conditional UPDATE (so concurrent settlements cannot over-pay without SELECT ... FOR UPDATE,
	// which SQLite rejects); the AffiliateCashPayout rows are the human-readable settlement history.
	AffCashPaid          int64          `json:"aff_cash_paid" gorm:"type:bigint;not null;default:0;column:aff_cash_paid"`
	DeletedAt            gorm.DeletedAt `gorm:"index"`
	LinuxDOId            string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting              string         `json:"setting" gorm:"type:text;column:setting"`
	Remark               string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer       string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt            int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt          int64          `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	// AdminPermissions is a transient view of the fine-grained admin authz matrix
	// (module -> action -> allowed), populated from the casbin policy on read and
	// consumed by updateAdminPermissionsForUserInTx on write. Never persisted here.
	AdminPermissions map[string]map[string]bool `json:"admin_permissions,omitempty" gorm:"-:all"`
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:       user.Id,
		Group:    user.Group,
		Quota:    user.Quota,
		Status:   user.Status,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
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
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ? AND site_id = ?", username, siteId).Error
	} else {
		err = DB.Unscoped().First(&user, "(username = ? OR email = ?) AND site_id = ?", username, email, siteId).Error
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

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

// GetAllUsers returns a page of users. When siteScope != SiteScopeAll the result is
// restricted to that site_id (sub-site admins); SiteScopeAll keeps the global view
// (main-site admins / root). site_id is filtered with an explicit condition so the
// main site (site_id=0) is matched correctly.
func GetAllUsers(pageInfo *common.PageInfo, siteScope int) (users []*User, total int64, err error) {
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
	dataQuery := tx.Unscoped().Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password", "access_token")
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
func SearchUsers(keyword string, group string, role *int, status *int, startIdx int, num int, siteScope int) ([]*User, int64, error) {
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
	err = query.Omit("password", "access_token").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
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
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := deleteUserOAuthBindingsByUserId(tx, id); err != nil {
			return err
		}
		return tx.Unscoped().Delete(&User{}, "id = ?", id).Error
	})
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(int(common.QuotaPerUnit)))
	}

	// Single conditional UPDATE: the aff_quota >= ? guard makes concurrent
	// transfers safe, and touching only the two balance columns can never
	// clobber a concurrent consumption update to quota/used_quota/request_count
	// (a stale full-row Save here previously could restore pre-consumption
	// values and erase usage).
	result := DB.Model(&User{}).
		Where("id = ? AND aff_quota >= ?", user.Id, quota).
		Updates(map[string]interface{}{
			"aff_quota": gorm.Expr("aff_quota - ?", quota),
			"quota":     gorm.Expr("quota + ?", quota),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("邀请额度不足！")
	}
	return nil
}

func (user *User) Insert(inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	//user.SetAccessToken(common.GetUUID())
	user.AffCode = common.GetRandomString(4)
	// Record the inviter relationship so referral rewards can be settled on the user's
	// first successful top-up (rewards are deferred, not granted at registration).
	if inviterId != 0 {
		user.InviterId = inviterId
	}

	// 初始化用户设置，包括默认的边栏配置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
		user.SetSetting(defaultSetting)
	}

	result := DB.Create(user)
	if result.Error != nil {
		return result.Error
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
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	user.AffCode = common.GetRandomString(4)
	// Record the inviter relationship so referral rewards can be settled on the user's
	// first successful top-up (rewards are deferred, not granted at registration).
	if inviterId != 0 {
		user.InviterId = inviterId
	}

	// 初始化用户设置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		user.SetSetting(defaultSetting)
	}

	result := tx.Create(user)
	if result.Error != nil {
		return result.Error
	}

	return nil
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
	if err := user.UpdateWithTx(DB, updatePassword); err != nil {
		return err
	}
	return updateUserCache(*user)
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
	if err = tx.First(&current, user.Id).Error; err != nil {
		return err
	}
	if err = tx.Model(&current).Omit("quota", "used_quota", "request_count").Updates(newUser).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

func (user *User) Edit(updatePassword bool, updateAffCommission bool, updateAffCashSettled bool) error {
	if err := user.EditWithTx(DB, updatePassword, updateAffCommission, updateAffCashSettled); err != nil {
		return err
	}
	return updateUserCache(*user)
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

	if err := DB.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
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
	if err := DB.Delete(user).Error; err != nil {
		return err
	}

	// 清除缓存
	return invalidateUserCache(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := deleteUserOAuthBindingsByUserId(tx, user.Id); err != nil {
			return err
		}
		return tx.Unscoped().Delete(user).Error
	})
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
	err = DB.Where("(username = ? OR email = ?) AND site_id = ?", username, username, user.SiteId).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
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
	DB.Where("email = ? AND site_id = ?", user.Email, siteId).First(user)
	return nil
}

func (user *User) FillUserByGitHubId(siteId int) error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where("github_id = ? AND site_id = ?", user.GitHubId, siteId).First(user)
	return nil
}

// UpdateGitHubId updates the user's GitHub ID (used for migration from login to numeric ID)
func (user *User) UpdateGitHubId(newGitHubId string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}
	return DB.Model(user).Update("github_id", newGitHubId).Error
}

func (user *User) FillUserByDiscordId(siteId int) error {
	if user.DiscordId == "" {
		return errors.New("discord id 为空！")
	}
	DB.Where("discord_id = ? AND site_id = ?", user.DiscordId, siteId).First(user)
	return nil
}

func (user *User) FillUserByOidcId(siteId int) error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where("oidc_id = ? AND site_id = ?", user.OidcId, siteId).First(user)
	return nil
}

func (user *User) FillUserByWeChatId(siteId int) error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where("wechat_id = ? AND site_id = ?", user.WeChatId, siteId).First(user)
	return nil
}

func (user *User) FillUserByTelegramId(siteId int) error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where("telegram_id = ? AND site_id = ?", user.TelegramId, siteId).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return nil
}

// The Is*AlreadyTaken checks compare RowsAffected > 0 (not == 1): if duplicates
// ever exist (no unique index on these columns), the identifier must still
// report taken instead of silently allowing more duplicates.
func IsEmailAlreadyTaken(email string, siteId int) bool {
	return DB.Unscoped().Where("email = ? AND site_id = ?", email, siteId).Find(&User{}).RowsAffected > 0
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
	if email == "" {
		return errors.New("邮箱地址为空！")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var self User
		if err := tx.First(&self, "id = ?", userId).Error; err != nil {
			return err
		}
		var holders []User
		if err := lockForUpdate(tx).Unscoped().
			Where("email = ? AND site_id = ? AND id <> ?", email, self.SiteId, userId).
			Find(&holders).Error; err != nil {
			return err
		}
		if len(holders) > 0 {
			return errors.New("邮箱地址已被占用")
		}
		return tx.Model(&User{}).Where("id = ?", userId).Update("email", email).Error
	})
	if err != nil {
		return err
	}
	return InvalidateUserCache(userId)
}

func IsWeChatIdAlreadyTaken(wechatId string, siteId int) bool {
	return DB.Unscoped().Where("wechat_id = ? AND site_id = ?", wechatId, siteId).Find(&User{}).RowsAffected > 0
}

func IsGitHubIdAlreadyTaken(githubId string, siteId int) bool {
	return DB.Unscoped().Where("github_id = ? AND site_id = ?", githubId, siteId).Find(&User{}).RowsAffected > 0
}

func IsDiscordIdAlreadyTaken(discordId string, siteId int) bool {
	return DB.Unscoped().Where("discord_id = ? AND site_id = ?", discordId, siteId).Find(&User{}).RowsAffected > 0
}

func IsOidcIdAlreadyTaken(oidcId string, siteId int) bool {
	return DB.Where("oidc_id = ? AND site_id = ?", oidcId, siteId).Find(&User{}).RowsAffected > 0
}

func IsTelegramIdAlreadyTaken(telegramId string, siteId int) bool {
	return DB.Unscoped().Where("telegram_id = ? AND site_id = ?", telegramId, siteId).Find(&User{}).RowsAffected > 0
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
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var ids []int
		if err := tx.Model(&User{}).Where("email = ? AND site_id = ?", email, siteId).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return errors.New("该邮箱未绑定任何账户")
		}
		if len(ids) > 1 {
			return errors.New("该邮箱匹配到多个账户，请联系管理员处理")
		}
		return tx.Model(&User{}).Where("id = ?", ids[0]).Update("password", hashedPassword).Error
	})
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
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserQuotaCache(id, quota); err != nil {
					common.SysLog("failed to update user quota cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			return quota, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
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
				if err := updateUserGroupCache(id, group); err != nil {
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
	gopool.Go(func() {
		err := cacheIncrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to increase user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, quota)
		return nil
	}
	return increaseUserQuota(id, quota)
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
	gopool.Go(func() {
		err := cacheDecrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to decrease user quota: " + err.Error())
		}
	})
	if !db && common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, -quota)
		return nil
	}
	return decreaseUserQuota(id, quota)
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

func updateUserQuotaUsedQuotaAndRequestCount(id int, quota int, usedQuota int, requestCount int) {
	if quota == 0 && usedQuota == 0 && requestCount == 0 {
		return
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
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
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
	var user User
	err := DB.Unscoped().Where("linux_do_id = ? AND site_id = ?", linuxDOId, siteId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId(siteId int) error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ? AND site_id = ?", user.LinuxDOId, siteId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}
