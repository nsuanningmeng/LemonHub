package model

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The mysqlRC25Legacy* models describe the schema that LemonHub v0.4.29
// wrote before the rc.25 synchronization. In particular, task billing
// readiness did not have
// dedicated columns, Midjourney did not persist its billing owners, custom
// OAuth subjects were globally unique, and external-identity subjects were
// limited to 128 characters under the site-scoped subject index.
type mysqlRC25LegacyUser struct {
	Id                   int            `gorm:"primaryKey"`
	SiteId               int            `gorm:"type:int;default:0;index;uniqueIndex:idx_users_site_username,priority:1;index:idx_users_site_email,priority:1"`
	Username             string         `gorm:"type:varchar(191);index;uniqueIndex:idx_users_site_username,priority:2"`
	Password             string         `gorm:"not null"`
	DisplayName          string         `gorm:"index"`
	Role                 int            `gorm:"type:int;default:1"`
	Status               int            `gorm:"type:int;default:1"`
	Email                string         `gorm:"type:varchar(191);index;index:idx_users_site_email,priority:2"`
	GitHubId             string         `gorm:"column:github_id;index"`
	DiscordId            string         `gorm:"column:discord_id;index"`
	OidcId               string         `gorm:"column:oidc_id;index"`
	WeChatId             string         `gorm:"column:wechat_id;index"`
	TelegramId           string         `gorm:"column:telegram_id;index"`
	AccessToken          *string        `gorm:"type:char(32);column:access_token;uniqueIndex"`
	Quota                int            `gorm:"type:int;default:0"`
	UsedQuota            int            `gorm:"type:int;default:0;column:used_quota"`
	RequestCount         int            `gorm:"type:int;default:0"`
	Group                string         `gorm:"type:varchar(64);default:'default'"`
	AffCode              string         `gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount             int            `gorm:"type:int;default:0;column:aff_count"`
	AffQuota             int            `gorm:"type:int;default:0;column:aff_quota"`
	AffHistoryQuota      int            `gorm:"type:int;default:0;column:aff_history"`
	InviterId            int            `gorm:"type:int;column:inviter_id;index"`
	AffCommissionPercent *float64       `gorm:"column:aff_commission_percent"`
	AffCashSettled       bool           `gorm:"column:aff_cash_settled"`
	AffCashPaid          int64          `gorm:"type:bigint;not null;default:0;column:aff_cash_paid"`
	DeletedAt            gorm.DeletedAt `gorm:"index"`
	LinuxDOId            string         `gorm:"column:linux_do_id;index"`
	Setting              string         `gorm:"type:text;column:setting"`
	Remark               string         `gorm:"type:varchar(255)"`
	StripeCustomer       string         `gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt            int64          `gorm:"autoCreateTime;column:created_at"`
	LastLoginAt          int64          `gorm:"default:0;column:last_login_at"`
	AuthVersion          int64          `gorm:"type:bigint;not null;default:1;column:auth_version"`
}

func (mysqlRC25LegacyUser) TableName() string { return "users" }

type mysqlRC25LegacyExternalIdentityClaim struct {
	Id        int64  `gorm:"primaryKey"`
	Provider  string `gorm:"type:varchar(32);not null;uniqueIndex:idx_external_identity_subject_site,priority:1;uniqueIndex:idx_external_identity_user,priority:1"`
	SiteId    int    `gorm:"type:int;not null;default:0;index;uniqueIndex:idx_external_identity_subject_site,priority:2"`
	Subject   string `gorm:"type:varchar(128);not null;uniqueIndex:idx_external_identity_subject_site,priority:3"`
	UserId    int    `gorm:"not null;index;uniqueIndex:idx_external_identity_user,priority:2"`
	CreatedAt time.Time
}

func (mysqlRC25LegacyExternalIdentityClaim) TableName() string {
	return "external_identity_claims"
}

type mysqlRC25LegacyCustomOAuthProvider struct {
	Id                    int    `gorm:"primaryKey"`
	Name                  string `gorm:"type:varchar(64);not null"`
	Slug                  string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Icon                  string `gorm:"type:varchar(128);default:''"`
	Enabled               bool   `gorm:"default:false"`
	ClientId              string `gorm:"type:varchar(256)"`
	ClientSecret          string `gorm:"type:varchar(512)"`
	AuthorizationEndpoint string `gorm:"type:varchar(512)"`
	TokenEndpoint         string `gorm:"type:varchar(512)"`
	UserInfoEndpoint      string `gorm:"type:varchar(512)"`
	Scopes                string `gorm:"type:varchar(256);default:'openid profile email'"`
	UserIdField           string `gorm:"type:varchar(128);default:'sub'"`
	UsernameField         string `gorm:"type:varchar(128);default:'preferred_username'"`
	DisplayNameField      string `gorm:"type:varchar(128);default:'name'"`
	EmailField            string `gorm:"type:varchar(128);default:'email'"`
	WellKnown             string `gorm:"type:varchar(512)"`
	AuthStyle             int    `gorm:"default:0"`
	AccessPolicy          string `gorm:"type:text"`
	AccessDeniedMessage   string `gorm:"type:varchar(512)"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (mysqlRC25LegacyCustomOAuthProvider) TableName() string {
	return "custom_oauth_providers"
}

type mysqlRC25LegacyUserOAuthBinding struct {
	Id             int    `gorm:"primaryKey"`
	UserId         int    `gorm:"not null;uniqueIndex:ux_user_provider,priority:1"`
	ProviderId     int    `gorm:"not null;uniqueIndex:ux_user_provider,priority:2;uniqueIndex:ux_provider_userid,priority:1"`
	ProviderUserId string `gorm:"type:varchar(256);not null;uniqueIndex:ux_provider_userid,priority:2"`
	CreatedAt      time.Time
}

func (mysqlRC25LegacyUserOAuthBinding) TableName() string {
	return "user_oauth_bindings"
}

type mysqlRC25LegacyTask struct {
	ID          int64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt   int64 `gorm:"index"`
	UpdatedAt   int64
	TaskID      string                `gorm:"type:varchar(191);index"`
	Platform    constant.TaskPlatform `gorm:"type:varchar(30);index"`
	UserId      int                   `gorm:"index"`
	Group       string                `gorm:"type:varchar(50)"`
	ChannelId   int                   `gorm:"index"`
	Quota       int
	Action      string     `gorm:"type:varchar(40);index"`
	Status      TaskStatus `gorm:"type:varchar(20);index"`
	FailReason  string
	SubmitTime  int64           `gorm:"index"`
	StartTime   int64           `gorm:"index"`
	FinishTime  int64           `gorm:"index"`
	Progress    string          `gorm:"type:varchar(20);index"`
	Properties  Properties      `gorm:"type:json"`
	PrivateData TaskPrivateData `gorm:"column:private_data;type:json"`
	Data        json.RawMessage `gorm:"type:json"`
}

func (mysqlRC25LegacyTask) TableName() string { return "tasks" }

type mysqlRC25LegacyMidjourney struct {
	Id          int `gorm:"primaryKey"`
	Code        int
	UserId      int    `gorm:"index"`
	Action      string `gorm:"type:varchar(40);index"`
	MjId        string `gorm:"index"`
	Prompt      string
	PromptEn    string
	Description string
	State       string
	SubmitTime  int64 `gorm:"index"`
	StartTime   int64 `gorm:"index"`
	FinishTime  int64 `gorm:"index"`
	ImageUrl    string
	VideoUrl    string
	VideoUrls   string
	Status      string `gorm:"type:varchar(20);index"`
	Progress    string `gorm:"type:varchar(30);index"`
	FailReason  string
	ChannelId   int
	Quota       int
	Buttons     string
	Properties  string
}

func (mysqlRC25LegacyMidjourney) TableName() string { return "midjourneys" }

type mysqlRC25LegacyTopUp struct {
	Id              int `gorm:"primaryKey"`
	SiteId          int `gorm:"type:int;default:0;index"`
	UserId          int `gorm:"index"`
	Amount          int64
	Money           float64
	TradeNo         string `gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `gorm:"type:varchar(50)"`
	PaymentProvider string `gorm:"type:varchar(50);default:'';index:idx_topup_provider_status_time,priority:1"`
	PaymentIntent   string `gorm:"type:varchar(255);index;default:''"`
	ClawedBackQuota int64  `gorm:"default:0"`
	CreateTime      int64  `gorm:"index:idx_topup_provider_status_time,priority:3"`
	CompleteTime    int64
	Status          string `gorm:"type:varchar(32);index:idx_topup_provider_status_time,priority:2"`
}

func (mysqlRC25LegacyTopUp) TableName() string { return "top_ups" }

// mysqlRC25LegacySubscriptionOrder is the subscription order schema written by
// LemonHub v0.4.30, before authoritative Epay reconciliation added nullable query
// leases and immutable checkout snapshots. Keeping a populated legacy table in the
// full MySQL migration test protects existing payment records from accidental table
// recreation, truncation, or backfill during upgrades.
type mysqlRC25LegacySubscriptionOrder struct {
	Id              int `gorm:"primaryKey"`
	UserId          int `gorm:"index"`
	PlanId          int `gorm:"index"`
	Money           float64
	TradeNo         string `gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `gorm:"type:varchar(50)"`
	PaymentProvider string `gorm:"type:varchar(50);default:''"`
	Status          string
	CreateTime      int64
	CompleteTime    int64
	ProviderPayload string `gorm:"type:text"`
}

func (mysqlRC25LegacySubscriptionOrder) TableName() string { return "subscription_orders" }

type mysqlRC25LegacySubscriptionPlan struct {
	Id                      int     `gorm:"primaryKey"`
	Title                   string  `gorm:"type:varchar(128);not null"`
	Subtitle                string  `gorm:"type:varchar(255);default:''"`
	PriceAmount             float64 `gorm:"type:decimal(10,6);not null;default:0"`
	Currency                string  `gorm:"type:varchar(8);not null;default:'USD'"`
	DurationUnit            string  `gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue           int     `gorm:"type:int;not null;default:1"`
	CustomSeconds           int64   `gorm:"type:bigint;not null;default:0"`
	Enabled                 bool    `gorm:"default:true"`
	SortOrder               int     `gorm:"type:int;default:0"`
	AllowBalancePay         *bool
	AllowWalletOverflow     *bool
	StripePriceId           string `gorm:"type:varchar(128);default:''"`
	CreemProductId          string `gorm:"type:varchar(128);default:''"`
	WaffoPancakeProductId   string `gorm:"type:varchar(128);default:''"`
	MaxPurchasePerUser      int    `gorm:"type:int;default:0"`
	UpgradeGroup            string `gorm:"type:varchar(64);default:''"`
	DowngradeGroup          string `gorm:"type:varchar(64);default:''"`
	TotalAmount             int64  `gorm:"type:bigint;not null;default:0"`
	QuotaResetPeriod        string `gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `gorm:"type:bigint;default:0"`
	CreatedAt               int64  `gorm:"bigint"`
	UpdatedAt               int64  `gorm:"bigint"`
}

func (mysqlRC25LegacySubscriptionPlan) TableName() string {
	return "subscription_plans"
}

type mysqlRC25LegacyToken struct {
	Id                 int    `gorm:"primaryKey"`
	SiteId             int    `gorm:"type:int;default:0;index"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"type:varchar(128);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int    `gorm:"default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int     `gorm:"default:0"`
	Group              string  `gorm:"default:''"`
	CrossGroupRetry    bool
	AutoGroups         string         `gorm:"type:text"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (mysqlRC25LegacyToken) TableName() string { return "tokens" }

type mysqlRC25LegacyUserSession struct {
	SID                 string `gorm:"column:sid;type:varchar(64);primaryKey"`
	UserID              int    `gorm:"column:user_id;not null;index:idx_user_sessions_user_status_expiry,priority:1;index:idx_user_sessions_user_created,priority:1"`
	Version             int64  `gorm:"type:bigint;not null;default:1"`
	UserAuthVersion     int64  `gorm:"type:bigint;not null"`
	Status              string `gorm:"type:varchar(16);not null;index:idx_user_sessions_user_status_expiry,priority:2;index:idx_user_sessions_status_revoked,priority:1"`
	RefreshHash         string `gorm:"type:char(64);not null"`
	PreviousRefreshHash string `gorm:"type:varchar(64)"`
	PreviousValidUntil  int64  `gorm:"type:bigint;not null;default:0"`
	LoginMethod         string `gorm:"type:varchar(32);not null"`
	IP                  string `gorm:"type:varchar(64)"`
	UserAgent           string `gorm:"type:text"`
	CreatedAt           int64  `gorm:"autoCreateTime;column:created_at;index:idx_user_sessions_user_created,priority:2"`
	LastActiveAt        int64  `gorm:"type:bigint;not null;column:last_active_at"`
	ExpiresAt           int64  `gorm:"type:bigint;not null;column:expires_at;index:idx_user_sessions_user_status_expiry,priority:3;index:idx_user_sessions_expires_at"`
	RevokedAt           int64  `gorm:"type:bigint;not null;default:0;column:revoked_at;index:idx_user_sessions_status_revoked,priority:2"`
	RevokedReason       string `gorm:"type:varchar(64);column:revoked_reason"`
}

func (mysqlRC25LegacyUserSession) TableName() string { return "user_sessions" }

var mysqlRC25UpgradeDatabaseName = regexp.MustCompile(`^lemonhub_rc25_upgrade_test_[a-z0-9_]+$`)

func TestMySQLRC25UpgradePreservesLegacyData(t *testing.T) {
	rawDSN := os.Getenv("TEST_MYSQL_RC25_UPGRADE_DSN")
	if rawDSN == "" {
		t.Skip("set TEST_MYSQL_RC25_UPGRADE_DSN to a dedicated disposable MySQL database named lemonhub_rc25_upgrade_test_*")
	}

	dsnConfig, err := mysqldriver.ParseDSN(rawDSN)
	require.NoError(t, err)
	require.Regexpf(t, mysqlRC25UpgradeDatabaseName, strings.ToLower(dsnConfig.DBName),
		"refusing destructive migration test against database %q: its name must match lemonhub_rc25_upgrade_test_*", dsnConfig.DBName)
	dsnConfig.ParseTime = true

	mdb, err := gorm.Open(gormmysql.Open(dsnConfig.FormatDSN()), newGormConfig(true))
	require.NoError(t, err)
	sqlDB, err := mdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)

	var selectedDatabase string
	require.NoError(t, mdb.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Regexpf(t, mysqlRC25UpgradeDatabaseName, strings.ToLower(selectedDatabase),
		"refusing destructive migration test against selected database %q: its name must match lemonhub_rc25_upgrade_test_*", selectedDatabase)

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = mdb, mdb
	common.SetDatabaseTypes(common.DatabaseTypeMySQL, common.DatabaseTypeMySQL)
	initCol()

	t.Cleanup(func() {
		rc25DropAllMySQLTablesBestEffort(mdb)
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
		_ = sqlDB.Close()
	})
	rc25DropAllMySQLTables(t, mdb)

	legacyModels := []interface{}{
		&mysqlRC25LegacyUser{},
		&mysqlRC25LegacyExternalIdentityClaim{},
		&mysqlRC25LegacyCustomOAuthProvider{},
		&mysqlRC25LegacyUserOAuthBinding{},
		&mysqlRC25LegacyTask{},
		&mysqlRC25LegacyMidjourney{},
		&mysqlRC25LegacyTopUp{},
		&mysqlRC25LegacySubscriptionOrder{},
		&mysqlRC25LegacySubscriptionPlan{},
		&mysqlRC25LegacyToken{},
		&mysqlRC25LegacyUserSession{},
	}
	require.NoError(t, mdb.AutoMigrate(legacyModels...))
	require.NoError(t, mdb.Exec("ALTER TABLE `external_identity_claims` ROW_FORMAT=COMPACT").Error)
	require.Equal(t, "compact", rc25MySQLRowFormat(t, mdb, "external_identity_claims"))

	legacySubject := strings.Repeat("t", 120)
	bindingSubject := "rc25-custom-subject-" + strings.Repeat("b", 180)
	accessToken := "0123456789abcdef0123456789abcdef"
	affiliatePercent := 12.5
	allowIPs := "127.0.0.1\n10.0.0.0/8"
	allowBalancePay := false
	allowWalletOverflow := true
	createdAt := time.Unix(1_700_000_000, 0).UTC()

	legacyUser := mysqlRC25LegacyUser{
		Id:                   101,
		SiteId:               7,
		Username:             "rc25-sentinel",
		Password:             "legacy-password-hash",
		DisplayName:          "RC25 Sentinel",
		Role:                 2,
		Status:               1,
		Email:                "rc25-sentinel@example.test",
		GitHubId:             "rc25-github-sentinel",
		TelegramId:           legacySubject,
		AccessToken:          &accessToken,
		Quota:                987654,
		UsedQuota:            123456,
		RequestCount:         321,
		Group:                "legacy-premium",
		AffCode:              "RC25AFF",
		AffCount:             8,
		AffQuota:             7654,
		AffHistoryQuota:      8765,
		InviterId:            55,
		AffCommissionPercent: &affiliatePercent,
		AffCashSettled:       true,
		AffCashPaid:          4567,
		LinuxDOId:            "",
		Setting:              `{"legacy":true}`,
		Remark:               "must survive rc25 migration",
		StripeCustomer:       "cus_rc25_sentinel",
		CreatedAt:            1_700_000_000,
		LastLoginAt:          1_700_001_000,
		AuthVersion:          9,
	}
	provider := mysqlRC25LegacyCustomOAuthProvider{
		Id:                    301,
		Name:                  "RC25 OAuth",
		Slug:                  "rc25-oauth",
		Icon:                  "KeyRound",
		Enabled:               true,
		ClientId:              "rc25-client",
		ClientSecret:          "rc25-secret",
		AuthorizationEndpoint: "https://id.example.test/authorize",
		TokenEndpoint:         "https://id.example.test/token",
		UserInfoEndpoint:      "https://id.example.test/userinfo",
		Scopes:                "openid profile email",
		UserIdField:           "sub",
		UsernameField:         "preferred_username",
		DisplayNameField:      "name",
		EmailField:            "email",
		WellKnown:             "https://id.example.test/.well-known/openid-configuration",
		AuthStyle:             2,
		AccessPolicy:          `{"logic":"and","conditions":[]}`,
		AccessDeniedMessage:   "legacy policy denial",
		CreatedAt:             createdAt,
		UpdatedAt:             createdAt.Add(time.Hour),
	}
	claim := mysqlRC25LegacyExternalIdentityClaim{
		Id: 201, Provider: ExternalIdentityProviderTelegram, SiteId: 7,
		Subject: legacySubject, UserId: legacyUser.Id, CreatedAt: createdAt,
	}
	binding := mysqlRC25LegacyUserOAuthBinding{
		Id: 302, UserId: legacyUser.Id, ProviderId: provider.Id,
		ProviderUserId: bindingSubject, CreatedAt: createdAt.Add(2 * time.Hour),
	}

	taskDataBytes, err := common.Marshal(map[string]interface{}{
		"output": "legacy-result",
		"nested": map[string]interface{}{"kept": true},
	})
	require.NoError(t, err)
	legacyTask := mysqlRC25LegacyTask{
		ID: 501, CreatedAt: 1_700_010_000, UpdatedAt: 1_700_010_100,
		TaskID: "task_rc25_sentinel", Platform: constant.TaskPlatform("kling"),
		UserId: legacyUser.Id, Group: "legacy-premium", ChannelId: 41, Quota: 24680,
		Action: "video", Status: TaskStatusSuccess, FailReason: "", SubmitTime: 1_700_010_010,
		StartTime: 1_700_010_020, FinishTime: 1_700_010_120, Progress: "100%",
		Properties: Properties{Input: "legacy prompt", UpstreamModelName: "legacy-upstream", OriginModelName: "legacy-origin"},
		PrivateData: TaskPrivateData{
			Key: "legacy-private-key", UpstreamTaskID: "upstream-rc25-501", ResultURL: "https://result.example.test/501",
			BillingSource: "wallet", SubscriptionId: 17, TokenId: 401, NodeName: "legacy-node",
			BillingContext: &TaskBillingContext{
				ModelPrice: 2.5, GroupRatio: 1.2, ModelRatio: 0.8,
				OtherRatios: map[string]float64{"duration": 3}, OriginModelName: "legacy-origin",
			},
		},
		Data: json.RawMessage(taskDataBytes),
	}
	legacyMidjourney := mysqlRC25LegacyMidjourney{
		Id: 502, Code: 1, UserId: legacyUser.Id, Action: "IMAGINE", MjId: "mj_rc25_sentinel",
		Prompt: "legacy mj prompt", PromptEn: "legacy translated prompt", Description: "legacy description",
		State: "legacy-state", SubmitTime: 1_700_020_000, StartTime: 1_700_020_010, FinishTime: 1_700_020_100,
		ImageUrl: "https://result.example.test/mj.png", VideoUrl: "https://result.example.test/mj.mp4",
		VideoUrls: `["https://result.example.test/mj.mp4"]`, Status: "SUCCESS", Progress: "100%",
		ChannelId: 42, Quota: 13579, Buttons: `[{"customId":"u1"}]`, Properties: `{"legacy":true}`,
	}
	legacyTopUp := mysqlRC25LegacyTopUp{
		Id: 601, SiteId: 7, UserId: legacyUser.Id, Amount: 88, Money: 19.99,
		TradeNo: "RC25-TOPUP-SENTINEL", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		PaymentIntent: "pi_rc25_sentinel", ClawedBackQuota: 1234, CreateTime: 1_700_030_000,
		CompleteTime: 1_700_030_100, Status: common.TopUpStatusSuccess,
	}
	legacySubscriptionOrder := mysqlRC25LegacySubscriptionOrder{
		Id: 602, UserId: legacyUser.Id, PlanId: 701, Money: 19.875,
		TradeNo: "RC25-SUBSCRIPTION-ORDER-SENTINEL", PaymentMethod: "alipay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
		CreateTime: 1_700_035_000, CompleteTime: 0,
		ProviderPayload: `{"legacy_callback":"must survive migration"}`,
	}
	legacyPlan := mysqlRC25LegacySubscriptionPlan{
		Id: 701, Title: "RC25 Plan", Subtitle: "legacy plan sentinel", PriceAmount: 19.875,
		Currency: "USD", DurationUnit: "month", DurationValue: 3, CustomSeconds: 0,
		Enabled: true, SortOrder: 17, AllowBalancePay: &allowBalancePay, AllowWalletOverflow: &allowWalletOverflow,
		StripePriceId: "price_rc25", CreemProductId: "creem_rc25", WaffoPancakeProductId: "waffo_rc25",
		MaxPurchasePerUser: 4, UpgradeGroup: "legacy-premium", DowngradeGroup: "default",
		TotalAmount: 5_000_000, QuotaResetPeriod: "month", QuotaResetCustomSeconds: 0,
		CreatedAt: 1_700_040_000, UpdatedAt: 1_700_040_100,
	}
	legacyToken := mysqlRC25LegacyToken{
		Id: 401, SiteId: 7, UserId: legacyUser.Id, Key: "sk-rc25-token-sentinel", Status: 1,
		Name: "RC25 token", CreatedTime: 1_700_050_000, AccessedTime: 1_700_050_100, ExpiredTime: -1,
		RemainQuota: 444444, UnlimitedQuota: false, ModelLimitsEnabled: true,
		ModelLimits: "gpt-4o,gpt-4.1,claude-sonnet-4-20250514", AllowIps: &allowIPs,
		UsedQuota: 55555, Group: "legacy-premium", CrossGroupRetry: true, AutoGroups: `["legacy-premium","default"]`,
	}
	legacySession := mysqlRC25LegacyUserSession{
		SID: "sid-rc25-sentinel", UserID: legacyUser.Id, Version: 4, UserAuthVersion: legacyUser.AuthVersion,
		Status: UserSessionStatusActive, RefreshHash: strings.Repeat("a", 64), PreviousRefreshHash: strings.Repeat("b", 64),
		PreviousValidUntil: 1_700_060_050, LoginMethod: "custom_oauth", IP: "203.0.113.9",
		UserAgent: "RC25 integration sentinel", CreatedAt: 1_700_060_000, LastActiveAt: 1_700_060_100,
		ExpiresAt: 1_800_000_000, RevokedAt: 0, RevokedReason: "",
	}

	for _, row := range []interface{}{
		&legacyUser, &provider, &claim, &binding, &legacyTask, &legacyMidjourney,
		&legacyTopUp, &legacySubscriptionOrder, &legacyPlan, &legacyToken, &legacySession,
	} {
		require.NoError(t, mdb.Create(row).Error)
	}

	require.Equal(t, "varchar(128)", strings.ToLower(rc25MySQLColumnType(t, mdb, "external_identity_claims", "subject")))
	require.Equal(t, []string{"provider", "site_id", "subject"}, rc25MySQLIndexColumns(t, mdb, "external_identity_claims", "idx_external_identity_subject_site"))
	require.Equal(t, []string{"provider_id", "provider_user_id"}, rc25MySQLIndexColumns(t, mdb, "user_oauth_bindings", "ux_provider_userid"))
	require.False(t, mdb.Migrator().HasColumn(&mysqlRC25LegacyTask{}, "billing_status"))
	require.False(t, mdb.Migrator().HasColumn(&mysqlRC25LegacyTask{}, "token_charged"))
	require.False(t, mdb.Migrator().HasColumn(&mysqlRC25LegacyMidjourney{}, "token_id"))
	require.False(t, mdb.Migrator().HasColumn(&mysqlRC25LegacyMidjourney{}, "billing_channel_id"))

	require.NoError(t, migrateDB(), "production rc25 migration sequence must upgrade the seeded legacy schema")
	rc25AssertUpgradedSentinels(t, mdb, legacySubject, bindingSubject)

	targetTables := []string{
		"users", "external_identity_claims", "custom_oauth_providers", "user_oauth_bindings",
		"tasks", "task_billing_ledgers", "midjourneys", "top_ups", "subscription_orders", "subscription_plans", "tokens", "user_sessions",
	}
	beforeSecondMigrationDDL := rc25MySQLTableDefinitions(t, mdb, targetTables)
	beforeSecondMigrationCounts := rc25MySQLTableCounts(t, mdb, targetTables)

	require.NoError(t, migrateDB(), "production migration sequence must be safe to run twice")

	afterSecondMigrationDDL := rc25MySQLTableDefinitions(t, mdb, targetTables)
	afterSecondMigrationCounts := rc25MySQLTableCounts(t, mdb, targetTables)
	assert.Equal(t, beforeSecondMigrationDDL, afterSecondMigrationDDL, "second migration must leave target schemas unchanged")
	assert.Equal(t, beforeSecondMigrationCounts, afterSecondMigrationCounts, "second migration must not add, remove, or merge persisted rows")
	rc25AssertUpgradedSentinels(t, mdb, legacySubject, bindingSubject)
}

func rc25AssertUpgradedSentinels(t *testing.T, db *gorm.DB, legacySubject, bindingSubject string) {
	t.Helper()

	var user User
	require.NoError(t, db.Unscoped().First(&user, 101).Error)
	assert.Equal(t, 7, user.SiteId)
	assert.Equal(t, "rc25-sentinel", user.Username)
	assert.Equal(t, "rc25-sentinel@example.test", user.Email)
	assert.Equal(t, "rc25-github-sentinel", user.GitHubId)
	assert.Equal(t, legacySubject, user.TelegramId)
	assert.Equal(t, 987654, user.Quota)
	assert.Equal(t, 123456, user.UsedQuota)
	assert.Equal(t, 321, user.RequestCount)
	assert.Equal(t, "legacy-premium", user.Group)
	assert.Equal(t, int64(9), user.AuthVersion)
	assert.Equal(t, int64(4567), user.AffCashPaid)

	var provider CustomOAuthProvider
	require.NoError(t, db.First(&provider, 301).Error)
	assert.Equal(t, "rc25-oauth", provider.Slug)
	assert.True(t, provider.Enabled)
	assert.Equal(t, "rc25-client", provider.ClientId)
	assert.Equal(t, `{"logic":"and","conditions":[]}`, provider.AccessPolicy)

	var legacyClaim ExternalIdentityClaim
	require.NoError(t, db.First(&legacyClaim, 201).Error)
	assert.Equal(t, ExternalIdentityProviderTelegram, legacyClaim.Provider)
	assert.Equal(t, 7, legacyClaim.SiteId)
	assert.Equal(t, legacySubject, legacyClaim.Subject)
	assert.Equal(t, 101, legacyClaim.UserId)

	var githubClaim ExternalIdentityClaim
	require.NoError(t, db.Where(
		"provider = ? AND site_id = ? AND subject = ?",
		ExternalIdentityProviderGitHub,
		7,
		"rc25-github-sentinel",
	).First(&githubClaim).Error)
	assert.Equal(t, 101, githubClaim.UserId)

	var binding UserOAuthBinding
	require.NoError(t, db.First(&binding, 302).Error)
	assert.Equal(t, 101, binding.UserId)
	assert.Equal(t, 301, binding.ProviderId)
	assert.Equal(t, 7, binding.SiteId)
	assert.Equal(t, bindingSubject, binding.ProviderUserId)

	boundUser, err := GetUserByOAuthBinding(301, bindingSubject, 7)
	require.NoError(t, err)
	assert.Equal(t, 101, boundUser.Id)

	var customClaim ExternalIdentityClaim
	require.NoError(t, db.Where("provider = ? AND site_id = ? AND subject = ?", customOAuthExternalIdentityProvider(301), 7, bindingSubject).First(&customClaim).Error)
	assert.Equal(t, 101, customClaim.UserId)

	assert.Equal(t, "varchar(256)", strings.ToLower(rc25MySQLColumnType(t, db, "external_identity_claims", "subject")))
	assert.Equal(t, "dynamic", rc25MySQLRowFormat(t, db, "external_identity_claims"))
	assert.False(t, rc25MySQLIndexExists(t, db, "external_identity_claims", "idx_external_identity_subject"))
	assert.True(t, rc25MySQLIndexUnique(t, db, "external_identity_claims", "idx_external_identity_subject_site"))
	assert.Equal(t, []string{"provider", "site_id", "subject"}, rc25MySQLIndexColumns(t, db, "external_identity_claims", "idx_external_identity_subject_site"))
	assert.False(t, rc25MySQLIndexExists(t, db, "user_oauth_bindings", "ux_provider_userid"))
	assert.True(t, rc25MySQLIndexUnique(t, db, "user_oauth_bindings", "ux_provider_site_userid"))
	assert.Equal(t, []string{"provider_id", "site_id", "provider_user_id"}, rc25MySQLIndexColumns(t, db, "user_oauth_bindings", "ux_provider_site_userid"))

	var task Task
	require.NoError(t, db.First(&task, 501).Error)
	assert.Equal(t, "task_rc25_sentinel", task.TaskID)
	assert.Equal(t, 101, task.UserId)
	assert.Equal(t, 24680, task.Quota)
	assert.Equal(t, "legacy prompt", task.Properties.Input)
	assert.Equal(t, "legacy-private-key", task.PrivateData.Key)
	assert.Equal(t, "upstream-rc25-501", task.PrivateData.UpstreamTaskID)
	assert.Equal(t, "wallet", task.PrivateData.BillingSource)
	assert.Equal(t, 17, task.PrivateData.SubscriptionId)
	assert.Equal(t, 401, task.PrivateData.TokenId)
	assert.Equal(t, "legacy-node", task.PrivateData.NodeName)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, 2.5, task.PrivateData.BillingContext.ModelPrice)
	assert.Equal(t, 3.0, task.PrivateData.BillingContext.OtherRatios["duration"])
	assert.Nil(t, task.PrivateData.SubmissionBilling)
	assert.Nil(t, task.PrivateData.BillingOperation)
	assert.Empty(t, task.PrivateData.AggregateUsageState)
	assert.Empty(t, task.BillingStatus)
	assert.Nil(t, task.TokenCharged)
	assert.True(t, task.BillingReady(), "a migrated legacy task must remain visible to the billing-ready query gate")
	assert.True(t, task.TokenBillingEnabled(), "nil token_charged must retain the legacy private_data.token_id meaning")
	var taskNulls struct {
		BillingStatusNull int `gorm:"column:billing_status_null"`
		TokenChargedNull  int `gorm:"column:token_charged_null"`
	}
	require.NoError(t, db.Raw(
		"SELECT billing_status IS NULL AS billing_status_null, token_charged IS NULL AS token_charged_null FROM tasks WHERE id = ?", 501,
	).Scan(&taskNulls).Error)
	assert.Equal(t, 1, taskNulls.BillingStatusNull)
	assert.Equal(t, 1, taskNulls.TokenChargedNull)
	var readyTaskCount int64
	require.NoError(t, taskBillingReadyScope(db.Model(&Task{})).Where("id = ?", 501).Count(&readyTaskCount).Error)
	assert.Equal(t, int64(1), readyTaskCount)
	var taskData map[string]interface{}
	require.NoError(t, common.Unmarshal(task.Data, &taskData))
	assert.Equal(t, "legacy-result", taskData["output"])

	var ledgerCount int64
	require.True(t, db.Migrator().HasTable(&TaskBillingLedger{}))
	require.NoError(t, db.Model(&TaskBillingLedger{}).Count(&ledgerCount).Error)
	assert.Zero(t, ledgerCount, "legacy tasks must not gain fabricated billing journal rows")
	assert.True(t, rc25MySQLIndexUnique(t, db, "task_billing_ledgers", "idx_task_billing_stage"))
	assert.Equal(t, []string{"task_type", "task_record_id", "operation", "stage"}, rc25MySQLIndexColumns(t, db, "task_billing_ledgers", "idx_task_billing_stage"))

	var midjourney Midjourney
	require.NoError(t, db.First(&midjourney, 502).Error)
	assert.Equal(t, "mj_rc25_sentinel", midjourney.MjId)
	assert.Equal(t, 13579, midjourney.Quota)
	assert.Equal(t, 42, midjourney.ChannelId)
	assert.Zero(t, midjourney.TokenId)
	assert.Zero(t, midjourney.BillingChannelId)

	var topUp TopUp
	require.NoError(t, db.First(&topUp, 601).Error)
	assert.Equal(t, 7, topUp.SiteId)
	assert.Equal(t, int64(88), topUp.Amount)
	assert.InDelta(t, 19.99, topUp.Money, 0.000001)
	assert.Equal(t, "RC25-TOPUP-SENTINEL", topUp.TradeNo)
	assert.Equal(t, "pi_rc25_sentinel", topUp.PaymentIntent)
	assert.Equal(t, int64(1234), topUp.ClawedBackQuota)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Nil(t, topUp.EpayQueryTime)
	assert.Nil(t, topUp.EpayCallbackQueryTime)

	var subscriptionOrder SubscriptionOrder
	require.NoError(t, db.First(&subscriptionOrder, 602).Error)
	assert.Equal(t, 101, subscriptionOrder.UserId)
	assert.Equal(t, 701, subscriptionOrder.PlanId)
	assert.InDelta(t, 19.875, subscriptionOrder.Money, 0.0000001)
	assert.Equal(t, "RC25-SUBSCRIPTION-ORDER-SENTINEL", subscriptionOrder.TradeNo)
	assert.Equal(t, "alipay", subscriptionOrder.PaymentMethod)
	assert.Equal(t, PaymentProviderEpay, subscriptionOrder.PaymentProvider)
	assert.Equal(t, common.TopUpStatusPending, subscriptionOrder.Status)
	assert.Equal(t, int64(1_700_035_000), subscriptionOrder.CreateTime)
	assert.Zero(t, subscriptionOrder.CompleteTime)
	assert.Equal(t, `{"legacy_callback":"must survive migration"}`, subscriptionOrder.ProviderPayload)
	assert.Nil(t, subscriptionOrder.EpayQueryTime)
	assert.Nil(t, subscriptionOrder.EpayCallbackQueryTime)
	assert.Empty(t, subscriptionOrder.PlanSnapshot)
	assert.False(t, subscriptionOrder.PurchaseLimitReserved)
	assert.True(t, rc25MySQLIndexExists(t, db, "subscription_orders", "idx_subscription_order_provider_time"))
	assert.Equal(t, []string{"payment_provider", "create_time"},
		rc25MySQLIndexColumns(t, db, "subscription_orders", "idx_subscription_order_provider_time"))

	var plan SubscriptionPlan
	require.NoError(t, db.First(&plan, 701).Error)
	assert.Equal(t, "RC25 Plan", plan.Title)
	assert.InDelta(t, 19.875, plan.PriceAmount, 0.0000001)
	assert.Equal(t, int64(5_000_000), plan.TotalAmount)
	require.NotNil(t, plan.AllowBalancePay)
	assert.False(t, *plan.AllowBalancePay)
	require.NotNil(t, plan.AllowWalletOverflow)
	assert.True(t, *plan.AllowWalletOverflow)
	assert.Equal(t, "decimal(10,6)", strings.ToLower(rc25MySQLColumnType(t, db, "subscription_plans", "price_amount")))

	var token Token
	require.NoError(t, db.First(&token, 401).Error)
	assert.Equal(t, 7, token.SiteId)
	assert.Equal(t, 101, token.UserId)
	assert.Equal(t, "sk-rc25-token-sentinel", token.Key)
	assert.Equal(t, 444444, token.RemainQuota)
	assert.Equal(t, 55555, token.UsedQuota)
	assert.Equal(t, "legacy-premium", token.Group)
	assert.Equal(t, "gpt-4o,gpt-4.1,claude-sonnet-4-20250514", token.ModelLimits)
	assert.Equal(t, "text", strings.ToLower(rc25MySQLColumnType(t, db, "tokens", "model_limits")))

	var session UserSession
	require.NoError(t, db.Where("sid = ?", "sid-rc25-sentinel").First(&session).Error)
	assert.Equal(t, 101, session.UserID)
	assert.Equal(t, int64(4), session.Version)
	assert.Equal(t, int64(9), session.UserAuthVersion)
	assert.Equal(t, UserSessionStatusActive, session.Status)
	assert.Equal(t, strings.Repeat("a", 64), session.RefreshHash)
	assert.Equal(t, strings.Repeat("b", 64), session.PreviousRefreshHash)
	assert.Equal(t, "custom_oauth", session.LoginMethod)
	assert.Equal(t, int64(1_800_000_000), session.ExpiresAt)
}

func rc25DropAllMySQLTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Connection(func(conn *gorm.DB) (err error) {
		var tables []string
		if err = conn.Raw(
			"SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'",
		).Scan(&tables).Error; err != nil {
			return err
		}
		if err = conn.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
			return err
		}
		defer func() {
			restoreErr := conn.Exec("SET FOREIGN_KEY_CHECKS = 1").Error
			if err == nil {
				err = restoreErr
			}
		}()
		for _, table := range tables {
			if err = conn.Exec("DROP TABLE IF EXISTS `" + strings.ReplaceAll(table, "`", "``") + "`").Error; err != nil {
				return err
			}
		}
		return nil
	}))

	var remaining int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'",
	).Scan(&remaining).Error)
	require.Zero(t, remaining, "the integration test must start from an empty disposable database")
}

func rc25DropAllMySQLTablesBestEffort(db *gorm.DB) {
	_ = db.Connection(func(conn *gorm.DB) error {
		var tables []string
		if err := conn.Raw(
			"SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'",
		).Scan(&tables).Error; err != nil {
			return err
		}
		if err := conn.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
			return err
		}
		defer func() { _ = conn.Exec("SET FOREIGN_KEY_CHECKS = 1").Error }()
		for _, table := range tables {
			_ = conn.Exec("DROP TABLE IF EXISTS `" + strings.ReplaceAll(table, "`", "``") + "`").Error
		}
		return nil
	})
}

func rc25MySQLColumnType(t *testing.T, db *gorm.DB, table, column string) string {
	t.Helper()
	var columnType string
	require.NoError(t, db.Raw(
		"SELECT COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?",
		table, column,
	).Scan(&columnType).Error)
	require.NotEmpty(t, columnType, "column %s.%s must exist", table, column)
	return columnType
}

func rc25MySQLRowFormat(t *testing.T, db *gorm.DB, table string) string {
	t.Helper()
	var rowFormat string
	require.NoError(t, db.Raw(
		"SELECT ROW_FORMAT FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		table,
	).Scan(&rowFormat).Error)
	return strings.ToLower(rowFormat)
}

func rc25MySQLIndexExists(t *testing.T, db *gorm.DB, table, index string) bool {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table, index,
	).Scan(&count).Error)
	return count > 0
}

func rc25MySQLIndexColumns(t *testing.T, db *gorm.DB, table, index string) []string {
	t.Helper()
	var columns []string
	require.NoError(t, db.Raw(
		"SELECT COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ? ORDER BY SEQ_IN_INDEX",
		table, index,
	).Scan(&columns).Error)
	return columns
}

func rc25MySQLIndexUnique(t *testing.T, db *gorm.DB, table, index string) bool {
	t.Helper()
	var nonUnique []int
	require.NoError(t, db.Raw(
		"SELECT DISTINCT NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table, index,
	).Scan(&nonUnique).Error)
	return len(nonUnique) == 1 && nonUnique[0] == 0
}

var rc25MySQLAutoIncrement = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)

func rc25MySQLTableDefinitions(t *testing.T, db *gorm.DB, tables []string) map[string]string {
	t.Helper()
	definitions := make(map[string]string, len(tables))
	for _, table := range tables {
		var name, definition string
		require.NoError(t, db.Raw("SHOW CREATE TABLE `"+strings.ReplaceAll(table, "`", "``")+"`").Row().Scan(&name, &definition))
		definitions[table] = rc25MySQLAutoIncrement.ReplaceAllString(definition, "")
	}
	return definitions
}

func rc25MySQLTableCounts(t *testing.T, db *gorm.DB, tables []string) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		counts[table] = count
	}
	return counts
}
