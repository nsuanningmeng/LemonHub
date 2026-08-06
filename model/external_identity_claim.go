package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ExternalIdentityProviderTelegram         = "telegram"
	externalIdentitySiteSubjectIndex         = "idx_external_identity_subject_site"
	legacyExternalIdentityGlobalSubjectIndex = "idx_external_identity_subject"
)

var ErrExternalIdentityAlreadyClaimed = errors.New("external identity is already claimed")

// ExternalIdentityClaim is the durable ownership record for an identity issued
// by an external provider. Provider subjects are single-owner within a site,
// matching every external-login lookup in LemonHub, while each user still has
// only one subject per provider.
type ExternalIdentityClaim struct {
	Id        int64     `json:"id" gorm:"primaryKey"`
	Provider  string    `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_external_identity_subject_site,priority:1;uniqueIndex:idx_external_identity_user,priority:1"`
	SiteId    int       `json:"site_id" gorm:"type:int;not null;default:0;index;uniqueIndex:idx_external_identity_subject_site,priority:2"`
	Subject   string    `json:"subject" gorm:"type:varchar(128);not null;uniqueIndex:idx_external_identity_subject_site,priority:3"`
	UserId    int       `json:"user_id" gorm:"not null;index;uniqueIndex:idx_external_identity_user,priority:2"`
	CreatedAt time.Time `json:"created_at"`
}

func (ExternalIdentityClaim) TableName() string {
	return "external_identity_claims"
}

// ClaimExternalIdentityWithTx atomically claims a provider subject for one
// user. Repeating the exact mapping is idempotent; every competing subject or
// user is rejected. Ownership is read back instead of trusting RowsAffected,
// whose duplicate-key semantics differ between supported databases.
func ClaimExternalIdentityWithTx(tx *gorm.DB, provider, subject string, userId int) error {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if tx == nil || provider == "" || subject == "" || userId == 0 {
		return errors.New("external identity claim is invalid")
	}

	var owner User
	if err := tx.Unscoped().Select("id", "site_id").Where("id = ?", userId).First(&owner).Error; err != nil {
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

// InitializeExternalIdentityClaims imports legacy Telegram bindings after the
// claim table is migrated. Telegram login is scoped by the request host, so the
// same Telegram subject may legitimately belong to one account on each site.
// Duplicate ownership within a site remains ambiguous and fails closed.
func InitializeExternalIdentityClaims() error {
	var users []User
	if err := DB.Unscoped().Select("id", "telegram_id").
		Where("telegram_id <> ?", "").Find(&users).Error; err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, user := range users {
			if err := ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.TelegramId, user.Id); err != nil {
				return fmt.Errorf("backfill Telegram identity for user %d: %w", user.Id, err)
			}
		}
		return nil
	})
}

// preflightExternalIdentityClaims rejects an ambiguous legacy Telegram mapping
// before any external-identity DDL is attempted. Cross-site reuse is valid;
// duplicate ownership inside one site is not.
func preflightExternalIdentityClaims(db *gorm.DB) error {
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}
	columns := []string{"id", "telegram_id"}
	if db.Migrator().HasColumn(&User{}, "site_id") {
		columns = append(columns, "site_id")
	}
	rows, err := db.Unscoped().Model(&User{}).
		Select(columns).
		Where("telegram_id IS NOT NULL AND telegram_id <> ?", "").
		Order("id").
		Rows()
	if err != nil {
		return fmt.Errorf("preflight legacy Telegram ownership: %w", err)
	}
	defer rows.Close()
	type ownershipKey struct {
		SiteId  int
		Subject string
	}
	owners := make(map[ownershipKey]int)
	for rows.Next() {
		var user struct {
			Id         int
			SiteId     int
			TelegramId string
		}
		if err := db.ScanRows(rows, &user); err != nil {
			return fmt.Errorf("scan legacy Telegram ownership: %w", err)
		}
		subject := strings.TrimSpace(user.TelegramId)
		if subject == "" {
			return fmt.Errorf("invalid blank legacy Telegram ownership for user %d in site %d", user.Id, user.SiteId)
		}
		key := ownershipKey{SiteId: user.SiteId, Subject: subject}
		if existingUserId, exists := owners[key]; exists {
			return fmt.Errorf(
				"ambiguous legacy Telegram ownership in site %d: users %d and %d share one Telegram ID",
				user.SiteId,
				existingUserId,
				user.Id,
			)
		}
		owners[key] = user.Id
	}
	return rows.Err()
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
	ownerSites := make(map[int64]int, len(claims))
	for _, claim := range claims {
		var owner User
		if err := db.Unscoped().Select("id", "site_id").Where("id = ?", claim.UserId).First(&owner).Error; err != nil {
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
			if err := db.Unscoped().Select("id", "site_id").Where("id = ?", claim.UserId).First(&owner).Error; err != nil {
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
		normalized := strings.ToLower(sql)
		columnsAt := strings.LastIndex(normalized, "(")
		if columnsAt < 0 {
			return false, nil
		}
		columns := strings.TrimSpace(strings.TrimSuffix(normalized[columnsAt+1:], ")"))
		parts := strings.Split(columns, ",")
		if len(parts) != 3 || !strings.Contains(normalized, "create unique index") {
			return false, nil
		}
		for i := range parts {
			parts[i] = strings.Trim(strings.TrimSpace(parts[i]), "`\"[]")
		}
		return parts[0] == "provider" && parts[1] == "site_id" && parts[2] == "subject", nil
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
			sql := strings.ToLower(index.SQL)
			if strings.Contains(sql, "create unique index") &&
				strings.Contains(sql, "provider") &&
				strings.Contains(sql, "subject") &&
				!strings.Contains(sql, "site_id") {
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
