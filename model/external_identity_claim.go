package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ExternalIdentityProviderGitHub           = "github"
	ExternalIdentityProviderDiscord          = "discord"
	ExternalIdentityProviderOIDC             = "oidc"
	ExternalIdentityProviderLinuxDO          = "linuxdo"
	ExternalIdentityProviderWeChat           = "wechat"
	ExternalIdentityProviderTelegram         = "telegram"
	externalIdentitySubjectMaxLength         = 256
	externalIdentitySiteSubjectIndex         = "idx_external_identity_subject_site"
	legacyExternalIdentityGlobalSubjectIndex = "idx_external_identity_subject"
)

var ErrExternalIdentityAlreadyClaimed = errors.New("external identity is already claimed")

type externalIdentityUserSource struct {
	Provider string
	Column   string
	Name     string
	Subject  func(*User) string
}

var externalIdentityUserSources = []externalIdentityUserSource{
	{ExternalIdentityProviderGitHub, "github_id", "GitHub", func(user *User) string { return user.GitHubId }},
	{ExternalIdentityProviderDiscord, "discord_id", "Discord", func(user *User) string { return user.DiscordId }},
	{ExternalIdentityProviderOIDC, "oidc_id", "OIDC", func(user *User) string { return user.OidcId }},
	{ExternalIdentityProviderLinuxDO, "linux_do_id", "LinuxDO", func(user *User) string { return user.LinuxDOId }},
	{ExternalIdentityProviderWeChat, "wechat_id", "WeChat", func(user *User) string { return user.WeChatId }},
	{ExternalIdentityProviderTelegram, "telegram_id", "Telegram", func(user *User) string { return user.TelegramId }},
}

func externalIdentityProviderForUserColumn(column string) (string, bool) {
	for _, source := range externalIdentityUserSources {
		if source.Column == column {
			return source.Provider, true
		}
	}
	return "", false
}

func customOAuthExternalIdentityProvider(providerId int) string {
	return "custom_oauth:" + strconv.Itoa(providerId)
}

// ExternalIdentityClaim is the durable ownership record for an identity issued
// by an external provider. Provider subjects are single-owner within a site,
// matching every external-login lookup in LemonHub, while each user still has
// only one subject per provider.
type ExternalIdentityClaim struct {
	Id        int64     `json:"id" gorm:"primaryKey"`
	Provider  string    `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_external_identity_subject_site,priority:1;uniqueIndex:idx_external_identity_user,priority:1"`
	SiteId    int       `json:"site_id" gorm:"type:int;not null;default:0;index;uniqueIndex:idx_external_identity_subject_site,priority:2"`
	Subject   string    `json:"subject" gorm:"type:varchar(256);not null;uniqueIndex:idx_external_identity_subject_site,priority:3"`
	UserId    int       `json:"user_id" gorm:"not null;index;uniqueIndex:idx_external_identity_user,priority:2"`
	CreatedAt time.Time `json:"created_at"`
}

func (ExternalIdentityClaim) TableName() string {
	return "external_identity_claims"
}

// NormalizeExternalIdentitySubject applies the canonical representation used
// by both legacy user columns and durable ownership claims. The 256-character
// limit preserves the pre-existing custom OAuth binding contract and covers
// OIDC subject identifiers without silently truncating multibyte values.
func NormalizeExternalIdentitySubject(subject string) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", errors.New("external identity subject is empty")
	}
	if !utf8.ValidString(subject) || utf8.RuneCountInString(subject) > externalIdentitySubjectMaxLength {
		return "", fmt.Errorf("external identity subject exceeds %d characters", externalIdentitySubjectMaxLength)
	}
	return subject, nil
}

// ClaimExternalIdentityWithTx atomically claims a provider subject for one
// user. Repeating the exact mapping is idempotent; every competing subject or
// user is rejected. Ownership is read back instead of trusting RowsAffected,
// whose duplicate-key semantics differ between supported databases.
func ClaimExternalIdentityWithTx(tx *gorm.DB, provider, subject string, userId int) error {
	provider = strings.TrimSpace(provider)
	if tx == nil || provider == "" || userId == 0 {
		return errors.New("external identity claim is invalid")
	}
	var err error
	subject, err = NormalizeExternalIdentitySubject(subject)
	if err != nil {
		return err
	}
	if len(provider) > 32 {
		return fmt.Errorf("external identity claim exceeds storage limit")
	}

	var owner User
	if err := lockForUpdate(tx.Unscoped()).Select("id", "site_id").Where("id = ?", userId).First(&owner).Error; err != nil {
		return err
	}

	claim := ExternalIdentityClaim{
		Provider: provider,
		SiteId:   owner.SiteId,
		Subject:  subject,
		UserId:   userId,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claim)
	if result.Error != nil {
		return result.Error
	}
	var subjectOwner ExternalIdentityClaim
	if err := tx.Where("provider = ? AND site_id = ? AND subject = ?", provider, owner.SiteId, subject).
		First(&subjectOwner).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrExternalIdentityAlreadyClaimed
		}
		return err
	}
	if subjectOwner.UserId != userId {
		return ErrExternalIdentityAlreadyClaimed
	}

	var userClaim ExternalIdentityClaim
	if err := tx.Where("provider = ? AND user_id = ?", provider, userId).First(&userClaim).Error; err != nil {
		return err
	}
	if userClaim.SiteId != owner.SiteId || userClaim.Subject != subject {
		return ErrExternalIdentityAlreadyClaimed
	}
	return nil
}

func ReleaseExternalIdentityWithTx(tx *gorm.DB, provider string, userId int) error {
	provider = strings.TrimSpace(provider)
	if tx == nil || provider == "" || userId == 0 {
		return errors.New("external identity release is invalid")
	}
	return tx.Where("provider = ? AND user_id = ?", provider, userId).
		Delete(&ExternalIdentityClaim{}).Error
}

func releaseAllExternalIdentitiesWithTx(tx *gorm.DB, userId int) error {
	if tx == nil || userId == 0 {
		return errors.New("external identity release is invalid")
	}
	return tx.Where("user_id = ?", userId).Delete(&ExternalIdentityClaim{}).Error
}

// InitializeExternalIdentityClaims imports every legacy built-in and custom
// OAuth binding after the claim/binding tables are migrated. The same provider
// subject may belong to one account on each site; duplicate ownership inside a
// site remains ambiguous and rolls back the whole backfill.
func InitializeExternalIdentityClaims() error {
	var users []User
	userColumns := []string{"id", "site_id"}
	for _, source := range externalIdentityUserSources {
		userColumns = append(userColumns, source.Column)
	}
	if err := DB.Unscoped().Select(userColumns).Find(&users).Error; err != nil {
		return err
	}
	var bindings []UserOAuthBinding
	if err := DB.Select("id", "user_id", "provider_id", "site_id", "provider_user_id").Find(&bindings).Error; err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, user := range users {
			for _, source := range externalIdentityUserSources {
				subject := strings.TrimSpace(source.Subject(&user))
				if subject == "" {
					continue
				}
				if err := ClaimExternalIdentityWithTx(tx, source.Provider, subject, user.Id); err != nil {
					return fmt.Errorf("backfill %s identity for user %d: %w", source.Name, user.Id, err)
				}
			}
		}
		for _, binding := range bindings {
			if err := ClaimExternalIdentityWithTx(tx,
				customOAuthExternalIdentityProvider(binding.ProviderId), binding.ProviderUserId, binding.UserId); err != nil {
				return fmt.Errorf("backfill custom OAuth provider %d identity for user %d: %w",
					binding.ProviderId, binding.UserId, err)
			}
		}
		return nil
	})
}

// preflightExternalIdentityClaims rejects ambiguous legacy built-in mappings
// before any external-identity DDL is attempted. Cross-site reuse is valid;
// duplicate ownership inside one site is not.
func preflightExternalIdentityClaims(db *gorm.DB) error {
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}
	columns := []string{"id"}
	if db.Migrator().HasColumn(&User{}, "site_id") {
		columns = append(columns, "site_id")
	}
	availableSources := make([]externalIdentityUserSource, 0, len(externalIdentityUserSources))
	for _, source := range externalIdentityUserSources {
		if db.Migrator().HasColumn(&User{}, source.Column) {
			columns = append(columns, source.Column)
			availableSources = append(availableSources, source)
		}
	}
	var users []User
	if err := db.Unscoped().Select(columns).Order("id").Find(&users).Error; err != nil {
		return fmt.Errorf("preflight legacy external identity ownership: %w", err)
	}
	type ownershipKey struct {
		Provider string
		SiteId   int
		Subject  string
	}
	owners := make(map[ownershipKey]int)
	for _, user := range users {
		for _, source := range availableSources {
			rawSubject := source.Subject(&user)
			if rawSubject == "" {
				continue
			}
			subject, err := NormalizeExternalIdentitySubject(rawSubject)
			if err != nil {
				return fmt.Errorf("invalid legacy %s ownership for user %d in site %d: %w", source.Name, user.Id, user.SiteId, err)
			}
			if rawSubject != subject {
				return fmt.Errorf("non-canonical whitespace in legacy %s ownership for user %d in site %d", source.Name, user.Id, user.SiteId)
			}
			key := ownershipKey{Provider: source.Provider, SiteId: user.SiteId, Subject: subject}
			if existingUserId, exists := owners[key]; exists {
				return fmt.Errorf(
					"ambiguous legacy %s ownership in site %d: users %d and %d share one provider ID",
					source.Name,
					user.SiteId,
					existingUserId,
					user.Id,
				)
			}
			owners[key] = user.Id
		}
	}
	return nil
}

// prepareExternalIdentityClaimsSiteScope upgrades the claim rows before
// AutoMigrate creates the replacement site-scoped unique index. The legacy
// global index remains in place throughout this phase, so an interrupted
// startup never leaves the table without subject-ownership enforcement.
func prepareExternalIdentityClaimsSiteScope(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
		return nil
	}

	var claims []ExternalIdentityClaim
	if err := db.Select("id", "user_id").Find(&claims).Error; err != nil {
		return fmt.Errorf("load external identity claims for site backfill: %w", err)
	}
	// A fast-migration process can be interrupted after creating the claim
	// table but before the concurrent User AutoMigrate adds site_id. Treat those
	// legacy owners as main-site users so the next startup can finish instead of
	// failing before User AutoMigrate gets another chance to run.
	userHasSite := db.Migrator().HasColumn(&User{}, "site_id")
	ownerSites := make(map[int64]int, len(claims))
	for _, claim := range claims {
		var owner User
		ownerColumns := []string{"id"}
		if userHasSite {
			ownerColumns = append(ownerColumns, "site_id")
		}
		if err := db.Unscoped().Select(ownerColumns).Where("id = ?", claim.UserId).First(&owner).Error; err != nil {
			return fmt.Errorf("resolve site for external identity claim %d: %w", claim.Id, err)
		}
		ownerSites[claim.Id] = owner.SiteId
	}

	if !db.Migrator().HasColumn(&ExternalIdentityClaim{}, "site_id") {
		if err := db.Migrator().AddColumn(&ExternalIdentityClaim{}, "SiteId"); err != nil {
			return fmt.Errorf("add external identity site scope: %w", err)
		}
	}

	if err := db.Select("id", "user_id", "site_id").Find(&claims).Error; err != nil {
		return fmt.Errorf("load external identity claims for site backfill: %w", err)
	}
	for _, claim := range claims {
		ownerSite, resolved := ownerSites[claim.Id]
		if !resolved {
			var owner User
			ownerColumns := []string{"id"}
			if userHasSite {
				ownerColumns = append(ownerColumns, "site_id")
			}
			if err := db.Unscoped().Select(ownerColumns).Where("id = ?", claim.UserId).First(&owner).Error; err != nil {
				return fmt.Errorf("resolve site for concurrent external identity claim %d: %w", claim.Id, err)
			}
			ownerSite = owner.SiteId
		}
		if claim.SiteId == ownerSite {
			continue
		}
		if err := db.Model(&ExternalIdentityClaim{}).Where("id = ?", claim.Id).
			Update("site_id", ownerSite).Error; err != nil {
			return fmt.Errorf("backfill site for external identity claim %d: %w", claim.Id, err)
		}
	}
	if db.Dialector.Name() == "sqlite" {
		return replaceExternalIdentitySubjectIndexSQLite(db)
	}
	return nil
}

// SQLite supports transactional DDL. Swap the subject indexes in one
// transaction before the broader AutoMigrate so the old or new uniqueness
// constraint is always present at commit boundaries. MySQL/PostgreSQL keep the
// old index until AutoMigrate creates the new one and finalize removes it.
func replaceExternalIdentitySubjectIndexSQLite(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		siteScoped, err := externalIdentitySubjectIndexIsSiteScoped(tx, externalIdentitySiteSubjectIndex)
		if err != nil {
			return err
		}
		if !siteScoped {
			if tx.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex) {
				return fmt.Errorf("external identity index %s exists with an unexpected definition", externalIdentitySiteSubjectIndex)
			}
			if err := tx.Migrator().CreateIndex(&ExternalIdentityClaim{}, externalIdentitySiteSubjectIndex); err != nil {
				return fmt.Errorf("create site-scoped external identity index: %w", err)
			}
			siteScoped, err = externalIdentitySubjectIndexIsSiteScoped(tx, externalIdentitySiteSubjectIndex)
			if err != nil {
				return err
			}
			if !siteScoped {
				return fmt.Errorf("site-scoped external identity index %s was not created", externalIdentitySiteSubjectIndex)
			}
		}

		legacyIndexes, err := legacyGlobalExternalIdentitySubjectIndexes(tx)
		if err != nil {
			return err
		}
		for _, name := range legacyIndexes {
			if err := tx.Migrator().DropIndex(&ExternalIdentityClaim{}, name); err != nil {
				return fmt.Errorf("drop global external identity index %s: %w", name, err)
			}
		}
		obsoleteScoped, err := externalIdentitySubjectIndexIsSiteScoped(tx, legacyExternalIdentityGlobalSubjectIndex)
		if err != nil {
			return err
		}
		if obsoleteScoped {
			if err := tx.Migrator().DropIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex); err != nil {
				return fmt.Errorf("drop obsolete external identity index: %w", err)
			}
		}
		return nil
	})
}

// finalizeExternalIdentityClaimsSiteScope runs after AutoMigrate. It first
// verifies that the new site-scoped unique index exists, then removes the old
// stricter global index. This ordering preserves a unique ownership constraint
// across interrupted upgrades. Mixed-version application nodes must not write
// during migration because older versions do not maintain the claim table.
func finalizeExternalIdentityClaimsSiteScope(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
		return nil
	}
	siteScoped, err := externalIdentitySubjectIndexIsSiteScoped(db, externalIdentitySiteSubjectIndex)
	if err != nil {
		return fmt.Errorf("verify site-scoped external identity index: %w", err)
	}
	if !siteScoped {
		return fmt.Errorf("site-scoped external identity subject index %s was not created", externalIdentitySiteSubjectIndex)
	}

	legacyIndexes, err := legacyGlobalExternalIdentitySubjectIndexes(db)
	if err != nil {
		return fmt.Errorf("get external identity indexes: %w", err)
	}
	for _, name := range legacyIndexes {
		common.SysLog("dropping legacy global external-identity subject index: " + name)
		if err := db.Migrator().DropIndex(&ExternalIdentityClaim{}, name); err != nil {
			return fmt.Errorf("drop global external identity index %s: %w", name, err)
		}
	}

	// A build from the interrupted release candidate may already have created
	// the site-scoped index under the old canonical name. The new index is now
	// verified, so removing that duplicate is safe and keeps the schema stable.
	obsoleteScoped, err := externalIdentitySubjectIndexIsSiteScoped(db, legacyExternalIdentityGlobalSubjectIndex)
	if err != nil {
		return fmt.Errorf("inspect obsolete external identity index: %w", err)
	}
	if obsoleteScoped {
		common.SysLog("dropping obsolete external-identity subject index: " + legacyExternalIdentityGlobalSubjectIndex)
		if err := db.Migrator().DropIndex(&ExternalIdentityClaim{}, legacyExternalIdentityGlobalSubjectIndex); err != nil {
			return fmt.Errorf("drop obsolete external identity index: %w", err)
		}
	}
	return nil
}

func externalIdentitySubjectIndexIsSiteScoped(db *gorm.DB, name string) (bool, error) {
	if db.Dialector.Name() == "sqlite" {
		var sql string
		if err := db.Raw(
			"SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?",
			ExternalIdentityClaim{}.TableName(),
			name,
		).Scan(&sql).Error; err != nil {
			return false, err
		}
		columns, unique := sqliteIndexColumns(sql)
		if !unique || len(columns) != 3 {
			return false, nil
		}
		return columns[0] == "provider" && columns[1] == "site_id" && columns[2] == "subject", nil
	}

	indexes, err := db.Migrator().GetIndexes(&ExternalIdentityClaim{})
	if err != nil {
		return false, err
	}
	for _, index := range indexes {
		if index.Name() != name {
			continue
		}
		unique, known := index.Unique()
		if !known || !unique {
			return false, nil
		}
		columns := index.Columns()
		if len(columns) != 3 {
			return false, nil
		}
		found := make(map[string]bool, len(columns))
		for _, column := range columns {
			found[strings.ToLower(column)] = true
		}
		return found["provider"] && found["site_id"] && found["subject"], nil
	}
	return false, nil
}

func legacyGlobalExternalIdentitySubjectIndexes(db *gorm.DB) ([]string, error) {
	if db.Dialector.Name() == "sqlite" {
		var indexes []struct {
			Name string `gorm:"column:name"`
			SQL  string `gorm:"column:sql"`
		}
		if err := db.Raw(
			"SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL",
			ExternalIdentityClaim{}.TableName(),
		).Scan(&indexes).Error; err != nil {
			return nil, err
		}
		var names []string
		for _, index := range indexes {
			columns, unique := sqliteIndexColumns(index.SQL)
			if unique && len(columns) == 2 &&
				((columns[0] == "provider" && columns[1] == "subject") ||
					(columns[0] == "subject" && columns[1] == "provider")) {
				names = append(names, index.Name)
			}
		}
		return names, nil
	}

	indexes, err := db.Migrator().GetIndexes(&ExternalIdentityClaim{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, index := range indexes {
		columns := index.Columns()
		if len(columns) != 2 {
			continue
		}
		hasProvider := false
		hasSubject := false
		hasSiteId := false
		for _, column := range columns {
			switch strings.ToLower(column) {
			case "provider":
				hasProvider = true
			case "subject":
				hasSubject = true
			case "site_id":
				hasSiteId = true
			}
		}
		if !hasProvider || !hasSubject || hasSiteId {
			continue
		}
		unique, known := index.Unique()
		if (known && unique) || (!known && index.Name() == legacyExternalIdentityGlobalSubjectIndex) {
			names = append(names, index.Name())
		}
	}
	return names, nil
}

func sqliteIndexColumns(sql string) ([]string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(sql))
	if !strings.HasPrefix(normalized, "create unique index") {
		return nil, false
	}
	columnsAt := strings.Index(normalized, "(")
	columnsEnd := strings.LastIndex(normalized, ")")
	if columnsAt < 0 || columnsEnd <= columnsAt {
		return nil, false
	}
	parts := strings.Split(normalized[columnsAt+1:columnsEnd], ",")
	for i := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(parts[i]), "`\"[]")
	}
	return parts, true
}
