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
	builtInExternalIdentitySubjectMaxLength  = 191
	externalIdentitySiteSubjectIndex         = "idx_external_identity_subject_site"
	legacyExternalIdentityGlobalSubjectIndex = "idx_external_identity_subject"
	mysqlLegacyIdentityIndexMaxBytes         = 767
)

var ErrExternalIdentityAlreadyClaimed = errors.New("external identity is already claimed")

type externalIdentityUserSource struct {
	Provider string
	Column   string
	Name     string
	Subject  func(*User) string
}

type mysqlIdentityComparison struct {
	Name          string
	DataType      string `gorm:"column:data_type"`
	CharacterSet  string `gorm:"column:character_set_name"`
	Collation     string `gorm:"column:collation_name"`
	MaximumLength int64  `gorm:"column:character_maximum_length"`
	IsNullable    string `gorm:"column:is_nullable"`
	HasDefault    bool   `gorm:"column:has_default"`
	Comment       string `gorm:"column:column_comment"`
	Extra         string `gorm:"column:extra"`
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

func externalIdentityUserSourceForProvider(provider string) (externalIdentityUserSource, bool) {
	for _, source := range externalIdentityUserSources {
		if source.Provider == provider {
			return source, true
		}
	}
	return externalIdentityUserSource{}, false
}

func customOAuthExternalIdentityProvider(providerId int) string {
	return "custom_oauth:" + strconv.Itoa(providerId)
}

func validateExternalIdentityProvider(provider string) error {
	if _, builtIn := externalIdentityUserSourceForProvider(provider); builtIn {
		return nil
	}
	const customPrefix = "custom_oauth:"
	if !strings.HasPrefix(provider, customPrefix) {
		return errors.New("external identity provider is unsupported")
	}
	providerId, err := strconv.Atoi(strings.TrimPrefix(provider, customPrefix))
	if err != nil || providerId <= 0 || customOAuthExternalIdentityProvider(providerId) != provider {
		return errors.New("external identity provider is invalid")
	}
	return nil
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

// normalizeBuiltInExternalIdentitySubjectForStorage applies the stricter
// capacity of MySQL's indexed legacy users.*_id VARCHAR(191) columns. SQLite
// and PostgreSQL keep the existing 256-character contract.
func normalizeBuiltInExternalIdentitySubjectForStorage(db *gorm.DB, subject string) (string, error) {
	subject, err := NormalizeExternalIdentitySubject(subject)
	if err != nil {
		return "", err
	}
	if db != nil && db.Dialector.Name() == "mysql" &&
		utf8.RuneCountInString(subject) > builtInExternalIdentitySubjectMaxLength {
		return "", fmt.Errorf(
			"built-in external identity subject exceeds %d characters",
			builtInExternalIdentitySubjectMaxLength,
		)
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
	if err := validateExternalIdentityProvider(provider); err != nil {
		return err
	}
	var err error
	if _, builtIn := externalIdentityUserSourceForProvider(provider); builtIn {
		subject, err = normalizeBuiltInExternalIdentitySubjectForStorage(tx, subject)
	} else {
		subject, err = NormalizeExternalIdentitySubject(subject)
	}
	if err != nil {
		return err
	}
	if len(provider) > 32 {
		return fmt.Errorf("external identity claim exceeds storage limit")
	}
	if err := validateMySQLExternalIdentitySubjectStorage(tx, subject); err != nil {
		return err
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
		if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
			return nil
		}
		if !db.Migrator().HasColumn(&ExternalIdentityClaim{}, "subject") {
			return errors.New("existing external identity claims table is missing subject")
		}
		var claimCount int64
		if err := db.Table(ExternalIdentityClaim{}.TableName()).Count(&claimCount).Error; err != nil {
			return fmt.Errorf("count persisted external identity claims without users table: %w", err)
		}
		if claimCount > 0 {
			return errors.New("persisted external identity claims exist but the users table is missing")
		}
		if db.Dialector.Name() == "mysql" {
			claimTarget, err := mysqlIdentityStringColumnDefinition(
				db,
				ExternalIdentityClaim{}.TableName(),
				"subject",
				"current claim",
			)
			if err != nil {
				return err
			}
			usersTarget, err := mysqlIdentityDatabaseDefaultComparison(db, "future users table default")
			if err != nil {
				return err
			}
			if !strings.EqualFold(claimTarget.CharacterSet, usersTarget.CharacterSet) ||
				!strings.EqualFold(claimTarget.Collation, usersTarget.Collation) {
				return fmt.Errorf(
					"MySQL current claim uses %s/%s but a future users table would use %s/%s; migration stopped before identity DDL because built-in login and claim uniqueness must use identical comparison semantics",
					claimTarget.CharacterSet,
					claimTarget.Collation,
					usersTarget.CharacterSet,
					usersTarget.Collation,
				)
			}
		}
		return nil
	}
	columns := []string{"id"}
	userHasSite := db.Migrator().HasColumn(&User{}, "site_id")
	if userHasSite {
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
			subject, err := normalizeBuiltInExternalIdentitySubjectForStorage(db, rawSubject)
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
	if err := preflightExternalIdentityCollationConflicts(db, availableSources, userHasSite); err != nil {
		return err
	}
	return preflightPersistedExternalIdentityClaims(db, users, userHasSite)
}

// preflightExternalIdentityCollationConflicts asks the database to compare
// legacy subjects under both the source column and claim subject collations. A
// Go map alone is insufficient on MySQL: common *_ci collations consider values
// such as "Example" and "example" (and often accented variants) equivalent.
// Checking both semantics avoids either a later unique-index failure or silently
// splitting one legacy identity after a collation change. COALESCE mirrors the
// later NULL site_id -> 0 backfill.
func preflightExternalIdentityCollationConflicts(db *gorm.DB, sources []externalIdentityUserSource, userHasSite bool) error {
	idColumn := db.Statement.Quote(clause.Column{Name: "id"})
	siteExpression := "0"
	if userHasSite {
		siteColumn := db.Statement.Quote(clause.Column{Name: "site_id"})
		siteExpression = "COALESCE(" + siteColumn + ", 0)"
	}

	var mysqlTargets []mysqlIdentityComparison
	if db.Dialector.Name() == "mysql" {
		var err error
		mysqlTargets, err = mysqlExternalIdentityClaimComparisons(db)
		if err != nil {
			return err
		}
		availableColumns := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			availableColumns[source.Column] = struct{}{}
		}
		var usersDefault mysqlIdentityComparison
		for _, source := range externalIdentityUserSources {
			if _, exists := availableColumns[source.Column]; exists {
				continue
			}
			if usersDefault.Name == "" {
				usersDefault, err = mysqlIdentityTableDefaultComparison(
					db,
					db.NamingStrategy.TableName("User"),
					"users table default",
				)
				if err != nil {
					return err
				}
			}
			for _, target := range mysqlTargets {
				if !strings.EqualFold(usersDefault.CharacterSet, target.CharacterSet) ||
					!strings.EqualFold(usersDefault.Collation, target.Collation) {
					return fmt.Errorf(
						"MySQL future legacy %s source would inherit %s/%s but %s uses %s/%s; migration stopped before identity DDL because built-in login and claim uniqueness must use identical comparison semantics",
						source.Name,
						usersDefault.CharacterSet,
						usersDefault.Collation,
						target.Name,
						target.CharacterSet,
						target.Collation,
					)
				}
			}
		}
	}

	type identityComparison struct {
		Name       string
		Expression string
	}
	for _, source := range sources {
		subjectColumn := db.Statement.Quote(clause.Column{Name: source.Column})
		sourceComparison := source.Column
		if userHasSite {
			sourceComparison = subjectColumn
		}
		comparisons := []identityComparison{
			{Name: "source", Expression: sourceComparison},
		}
		if db.Dialector.Name() == "mysql" {
			mysqlSource, err := mysqlIdentityStringColumnDefinition(
				db,
				db.NamingStrategy.TableName("User"),
				source.Column,
				"legacy "+source.Name+" source",
			)
			if err != nil {
				return err
			}
			if !strings.EqualFold(mysqlSource.DataType, "varchar") ||
				mysqlSource.MaximumLength != builtInExternalIdentitySubjectMaxLength ||
				!strings.EqualFold(mysqlSource.IsNullable, "YES") || mysqlSource.HasDefault ||
				mysqlSource.Comment != "" || mysqlSource.Extra != "" {
				return fmt.Errorf(
					"MySQL legacy %s source must be a nullable VARCHAR(%d) without defaults, comments, or extra attributes; migration stopped before identity DDL to prevent AutoMigrate from rewriting its storage",
					source.Name,
					builtInExternalIdentitySubjectMaxLength,
				)
			}
			for _, target := range mysqlTargets {
				if err := preflightMySQLIdentityColumnConversion(
					db,
					db.NamingStrategy.TableName("User"),
					"id",
					source.Column,
					mysqlSource,
					target,
				); err != nil {
					return err
				}
				if !strings.EqualFold(mysqlSource.CharacterSet, target.CharacterSet) ||
					!strings.EqualFold(mysqlSource.Collation, target.Collation) {
					return fmt.Errorf(
						"MySQL legacy %s source uses %s/%s but %s uses %s/%s; migration stopped before identity DDL because built-in login and claim uniqueness must use identical comparison semantics",
						source.Name,
						mysqlSource.CharacterSet,
						mysqlSource.Collation,
						target.Name,
						target.CharacterSet,
						target.Collation,
					)
				}
			}
		}

		for _, comparison := range comparisons {
			groupExpression := comparison.Expression
			if userHasSite {
				groupExpression = siteExpression + ", " + comparison.Expression
			}
			var collision struct {
				SiteId         int
				FirstUserId    int
				LastUserId     int
				DuplicateCount int64
			}
			err := db.Unscoped().Model(&User{}).
				Select(fmt.Sprintf(
					"%s AS site_id, MIN(%s) AS first_user_id, MAX(%s) AS last_user_id, COUNT(*) AS duplicate_count",
					siteExpression, idColumn, idColumn,
				)).
				Where(subjectColumn+" IS NOT NULL AND "+subjectColumn+" <> ?", "").
				Group(groupExpression).
				Having("COUNT(*) > 1").
				Limit(1).
				Scan(&collision).Error
			if err != nil {
				return fmt.Errorf(
					"preflight legacy %s ownership under %s database collation: %w",
					source.Name,
					comparison.Name,
					err,
				)
			}
			if collision.DuplicateCount > 1 {
				return fmt.Errorf(
					"ambiguous legacy %s ownership in site %d under database collation (%s): users %d and %d share equivalent provider IDs",
					source.Name,
					collision.SiteId,
					comparison.Name,
					collision.FirstUserId,
					collision.LastUserId,
				)
			}
		}
	}
	return nil
}

// preflightPersistedExternalIdentityClaims validates already-materialized
// ownership rows against their owners' eventual site scope and the collation
// that AutoMigrate will apply. This matters for interrupted/rc builds where the
// claim table may contain rows that older application nodes did not maintain.
func preflightPersistedExternalIdentityClaims(db *gorm.DB, users []User, userHasSite bool) error {
	if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
		return nil
	}
	for _, column := range []string{"id", "provider", "subject", "user_id"} {
		if !db.Migrator().HasColumn(&ExternalIdentityClaim{}, column) {
			return fmt.Errorf("existing external identity claims table is missing %s", column)
		}
	}

	columns := []string{"id", "provider", "subject", "user_id"}
	var claims []ExternalIdentityClaim
	if err := db.Select(columns).Order("id").Find(&claims).Error; err != nil {
		return fmt.Errorf("preflight persisted external identity claims: %w", err)
	}

	ownerSites := make(map[int]int, len(users))
	owners := make(map[int]*User, len(users))
	for index := range users {
		user := &users[index]
		ownerSites[user.Id] = user.SiteId
		owners[user.Id] = user
	}
	type subjectKey struct {
		Provider string
		SiteId   int
		Subject  string
	}
	type userKey struct {
		Provider string
		UserId   int
	}
	subjectClaims := make(map[subjectKey]int64, len(claims))
	userClaims := make(map[userKey]int64, len(claims))
	for _, claim := range claims {
		provider := strings.TrimSpace(claim.Provider)
		if provider == "" || provider != claim.Provider || len(provider) > 32 ||
			validateExternalIdentityProvider(provider) != nil {
			return fmt.Errorf("invalid provider on persisted external identity claim %d", claim.Id)
		}
		source, builtIn := externalIdentityUserSourceForProvider(provider)
		var subject string
		var err error
		if builtIn {
			subject, err = normalizeBuiltInExternalIdentitySubjectForStorage(db, claim.Subject)
		} else {
			subject, err = NormalizeExternalIdentitySubject(claim.Subject)
		}
		if err != nil {
			return fmt.Errorf("invalid subject on persisted external identity claim %d: %w", claim.Id, err)
		}
		if subject != claim.Subject {
			return fmt.Errorf("non-canonical whitespace on persisted external identity claim %d", claim.Id)
		}
		ownerSite, exists := ownerSites[claim.UserId]
		if !exists {
			return fmt.Errorf("persisted external identity claim %d references missing user %d", claim.Id, claim.UserId)
		}
		if builtIn {
			legacySubject := source.Subject(owners[claim.UserId])
			normalizedLegacy, normalizeErr := normalizeBuiltInExternalIdentitySubjectForStorage(db, legacySubject)
			if normalizeErr != nil || normalizedLegacy != legacySubject || normalizedLegacy != subject {
				return fmt.Errorf(
					"persisted external identity claim %d conflicts with legacy %s binding for user %d",
					claim.Id,
					source.Name,
					claim.UserId,
				)
			}
		}
		sKey := subjectKey{Provider: provider, SiteId: ownerSite, Subject: subject}
		if existingClaimId, exists := subjectClaims[sKey]; exists {
			return fmt.Errorf(
				"ambiguous persisted external identity ownership in site %d: claims %d and %d share one provider subject",
				ownerSite,
				existingClaimId,
				claim.Id,
			)
		}
		uKey := userKey{Provider: provider, UserId: claim.UserId}
		if existingClaimId, exists := userClaims[uKey]; exists {
			return fmt.Errorf(
				"ambiguous persisted external identity claims %d and %d for user %d provider %s",
				existingClaimId,
				claim.Id,
				claim.UserId,
				provider,
			)
		}
		subjectClaims[sKey] = claim.Id
		userClaims[uKey] = claim.Id
	}
	if err := preflightBuiltInExternalIdentityClaimCrossOwnership(db, userHasSite); err != nil {
		return err
	}
	if len(claims) < 2 {
		return nil
	}

	claimTable := ExternalIdentityClaim{}.TableName()
	userTable := db.NamingStrategy.TableName("User")
	idColumn := db.Statement.Quote(clause.Column{Table: "c", Name: "id"})
	providerColumn := db.Statement.Quote(clause.Column{Table: "c", Name: "provider"})
	userColumn := db.Statement.Quote(clause.Column{Table: "c", Name: "user_id"})
	subjectColumn := db.Statement.Quote(clause.Column{Table: "c", Name: "subject"})
	siteExpression := "0"
	if userHasSite {
		siteColumn := db.Statement.Quote(clause.Column{Table: "u", Name: "site_id"})
		siteExpression = "COALESCE(" + siteColumn + ", 0)"
	}

	type identityComparison struct {
		Name       string
		Expression string
	}
	comparisons := []identityComparison{{Name: "current claim", Expression: subjectColumn}}
	if db.Dialector.Name() == "mysql" {
		targets, err := mysqlExternalIdentityClaimComparisons(db)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return errors.New("missing MySQL external identity claim comparison")
		}
		current := targets[0]
		for _, target := range targets[1:] {
			if strings.EqualFold(current.CharacterSet, target.CharacterSet) &&
				strings.EqualFold(current.Collation, target.Collation) {
				continue
			}
			comparisons = append(comparisons, identityComparison{
				Name: target.Name,
				Expression: fmt.Sprintf(
					"CONVERT(%s USING %s) COLLATE %s",
					subjectColumn,
					target.CharacterSet,
					target.Collation,
				),
			})
		}
	}

	joinUsers := fmt.Sprintf(
		"JOIN %s AS u ON %s = %s",
		db.Statement.Quote(clause.Table{Name: userTable}),
		db.Statement.Quote(clause.Column{Table: "u", Name: "id"}),
		userColumn,
	)
	for _, comparison := range comparisons {
		groupExpression := providerColumn + ", " + comparison.Expression
		if userHasSite {
			groupExpression = providerColumn + ", " + siteExpression + ", " + comparison.Expression
		}
		var collision struct {
			SiteId         int
			FirstClaimId   int64
			LastClaimId    int64
			DuplicateCount int64
		}
		err := db.Table(claimTable+" AS c").
			Joins(joinUsers).
			Select(fmt.Sprintf(
				"%s AS site_id, MIN(%s) AS first_claim_id, MAX(%s) AS last_claim_id, COUNT(*) AS duplicate_count",
				siteExpression,
				idColumn,
				idColumn,
			)).
			Where(subjectColumn+" IS NOT NULL AND "+subjectColumn+" <> ?", "").
			Group(groupExpression).
			Having("COUNT(*) > 1").
			Limit(1).
			Scan(&collision).Error
		if err != nil {
			return fmt.Errorf("preflight persisted external identity ownership under %s database collation: %w", comparison.Name, err)
		}
		if collision.DuplicateCount > 1 {
			return fmt.Errorf(
				"ambiguous persisted external identity ownership in site %d under database collation (%s): claims %d and %d share equivalent provider subjects",
				collision.SiteId,
				comparison.Name,
				collision.FirstClaimId,
				collision.LastClaimId,
			)
		}
	}
	return nil
}

func preflightBuiltInExternalIdentityClaimCrossOwnership(db *gorm.DB, userHasSite bool) error {
	if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
		return nil
	}
	claimTable := ExternalIdentityClaim{}.TableName()
	userTable := db.NamingStrategy.TableName("User")
	claimProvider := db.Statement.Quote(clause.Column{Table: "c", Name: "provider"})
	claimSubject := db.Statement.Quote(clause.Column{Table: "c", Name: "subject"})
	claimUser := db.Statement.Quote(clause.Column{Table: "c", Name: "user_id"})
	legacyUser := db.Statement.Quote(clause.Column{Table: "u", Name: "id"})
	claimOwner := db.Statement.Quote(clause.Column{Table: "cu", Name: "id"})
	legacySite := "0"
	claimSite := "0"
	if userHasSite {
		legacySite = "COALESCE(" + db.Statement.Quote(clause.Column{Table: "u", Name: "site_id"}) + ", 0)"
		claimSite = "COALESCE(" + db.Statement.Quote(clause.Column{Table: "cu", Name: "site_id"}) + ", 0)"
	}
	joinClaimOwner := fmt.Sprintf(
		"JOIN %s AS cu ON %s = %s",
		db.Statement.Quote(clause.Table{Name: userTable}),
		claimOwner,
		claimUser,
	)
	for _, source := range externalIdentityUserSources {
		if !db.Migrator().HasColumn(&User{}, source.Column) {
			continue
		}
		legacySubject := db.Statement.Quote(clause.Column{Table: "u", Name: source.Column})
		joinClaim := fmt.Sprintf(
			"JOIN %s AS c ON %s = ? AND (%s = %s OR %s = %s)",
			db.Statement.Quote(clause.Table{Name: claimTable}),
			claimProvider,
			claimSubject,
			legacySubject,
			legacySubject,
			claimSubject,
		)
		var conflict struct {
			LegacyUserId int   `gorm:"column:legacy_user_id"`
			ClaimId      int64 `gorm:"column:claim_id"`
			ClaimUserId  int   `gorm:"column:claim_user_id"`
		}
		result := db.Table(userTable+" AS u").
			Joins(joinClaim, source.Provider).
			Joins(joinClaimOwner).
			Select(fmt.Sprintf(
				"%s AS legacy_user_id, %s AS claim_id, %s AS claim_user_id",
				legacyUser,
				db.Statement.Quote(clause.Column{Table: "c", Name: "id"}),
				claimUser,
			)).
			Where(legacySubject+" IS NOT NULL AND "+legacySubject+" <> ?", "").
			Where(claimUser + " <> " + legacyUser).
			Where(legacySite + " = " + claimSite).
			Limit(1).
			Scan(&conflict)
		if result.Error != nil {
			return fmt.Errorf("preflight legacy %s ownership against persisted claims: %w", source.Name, result.Error)
		}
		if result.RowsAffected > 0 {
			return fmt.Errorf(
				"persisted external identity claim %d for user %d conflicts with legacy %s ownership for user %d",
				conflict.ClaimId,
				conflict.ClaimUserId,
				source.Name,
				conflict.LegacyUserId,
			)
		}
	}
	return nil
}

func mysqlExternalIdentityClaimComparisons(db *gorm.DB) ([]mysqlIdentityComparison, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil, errors.New("MySQL external identity comparison requires a MySQL database")
	}

	var targets []mysqlIdentityComparison
	if db.Migrator().HasTable(&ExternalIdentityClaim{}) &&
		db.Migrator().HasColumn(&ExternalIdentityClaim{}, "subject") {
		existingTarget, err := mysqlIdentityStringColumnDefinition(
			db,
			ExternalIdentityClaim{}.TableName(),
			"subject",
			"current claim",
		)
		if err != nil {
			return nil, err
		}
		targets = append(targets, existingTarget)
	} else {
		newTarget, err := mysqlNewExternalIdentityClaimComparison(db)
		if err != nil {
			return nil, err
		}
		newTarget.DataType = "varchar"
		newTarget.MaximumLength = externalIdentitySubjectMaxLength
		newTarget.IsNullable = "NO"
		targets = append(targets, newTarget)
	}

	for _, target := range targets {
		if err := validateMySQLIdentityComparison(target); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func mysqlNewExternalIdentityClaimComparison(db *gorm.DB) (mysqlIdentityComparison, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return mysqlIdentityComparison{}, errors.New("new MySQL identity comparison requires a MySQL database")
	}
	if !db.Migrator().HasTable(&User{}) {
		return mysqlIdentityDatabaseDefaultComparison(db, "claim target")
	}

	var target mysqlIdentityComparison
	var usersDefault mysqlIdentityComparison
	for _, source := range externalIdentityUserSources {
		var comparison mysqlIdentityComparison
		var err error
		if db.Migrator().HasColumn(&User{}, source.Column) {
			comparison, err = mysqlIdentityStringColumnDefinition(
				db,
				db.NamingStrategy.TableName("User"),
				source.Column,
				"legacy "+source.Name+" source",
			)
		} else {
			if usersDefault.Name == "" {
				usersDefault, err = mysqlIdentityTableDefaultComparison(
					db,
					db.NamingStrategy.TableName("User"),
					"users table default",
				)
			}
			comparison = usersDefault
		}
		if err != nil {
			return mysqlIdentityComparison{}, err
		}
		if target.Name == "" {
			target = comparison
			continue
		}
		if !strings.EqualFold(target.CharacterSet, comparison.CharacterSet) ||
			!strings.EqualFold(target.Collation, comparison.Collation) {
			return mysqlIdentityComparison{}, fmt.Errorf(
				"MySQL built-in identity sources do not share one comparison: %s uses %s/%s while %s would use %s/%s; migration stopped before identity DDL",
				target.Name,
				target.CharacterSet,
				target.Collation,
				source.Name,
				comparison.CharacterSet,
				comparison.Collation,
			)
		}
	}
	target.Name = "claim target"
	return target, nil
}

func mysqlIdentityDatabaseDefaultComparison(db *gorm.DB, comparisonName string) (mysqlIdentityComparison, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return mysqlIdentityComparison{}, errors.New("MySQL identity database comparison requires a MySQL database")
	}
	var comparison mysqlIdentityComparison
	if err := db.Raw(
		`SELECT DEFAULT_CHARACTER_SET_NAME AS character_set_name,
       DEFAULT_COLLATION_NAME AS collation_name
FROM information_schema.SCHEMATA
WHERE SCHEMA_NAME = DATABASE()`,
	).Scan(&comparison).Error; err != nil {
		return mysqlIdentityComparison{}, fmt.Errorf(
			"resolve MySQL %s comparison: %w",
			comparisonName,
			err,
		)
	}
	comparison.Name = comparisonName
	if err := validateMySQLIdentityComparison(comparison); err != nil {
		return mysqlIdentityComparison{}, err
	}
	return comparison, nil
}

func mysqlIdentityStringColumnDefinition(
	db *gorm.DB,
	tableName string,
	columnName string,
	comparisonName string,
) (mysqlIdentityComparison, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return mysqlIdentityComparison{}, errors.New("MySQL identity column definition requires a MySQL database")
	}
	var definition mysqlIdentityComparison
	if err := db.Raw(
		`SELECT c.DATA_TYPE AS data_type,
       c.CHARACTER_SET_NAME AS character_set_name,
       c.COLLATION_NAME AS collation_name,
       c.CHARACTER_MAXIMUM_LENGTH AS character_maximum_length,
       c.IS_NULLABLE AS is_nullable,
       (c.COLUMN_DEFAULT IS NOT NULL) AS has_default,
       c.COLUMN_COMMENT AS column_comment,
       c.EXTRA AS extra
FROM information_schema.COLUMNS c
WHERE c.TABLE_SCHEMA = DATABASE() AND c.TABLE_NAME = ? AND c.COLUMN_NAME = ?`,
		tableName,
		columnName,
	).Scan(&definition).Error; err != nil {
		return mysqlIdentityComparison{}, fmt.Errorf(
			"resolve MySQL %s column definition: %w",
			comparisonName,
			err,
		)
	}
	definition.Name = comparisonName
	if definition.MaximumLength <= 0 {
		return mysqlIdentityComparison{}, fmt.Errorf("missing MySQL %s column definition", comparisonName)
	}
	if err := validateMySQLIdentityComparison(definition); err != nil {
		return mysqlIdentityComparison{}, err
	}
	return definition, nil
}

func mysqlIdentityTableDefaultComparison(
	db *gorm.DB,
	tableName string,
	comparisonName string,
) (mysqlIdentityComparison, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return mysqlIdentityComparison{}, errors.New("MySQL identity table comparison requires a MySQL database")
	}
	var comparison mysqlIdentityComparison
	if err := db.Raw(
		`SELECT co.CHARACTER_SET_NAME AS character_set_name,
       t.TABLE_COLLATION AS collation_name
FROM information_schema.TABLES t
JOIN information_schema.COLLATIONS co ON co.COLLATION_NAME = t.TABLE_COLLATION
WHERE t.TABLE_SCHEMA = DATABASE() AND t.TABLE_NAME = ?`,
		tableName,
	).Scan(&comparison).Error; err != nil {
		return mysqlIdentityComparison{}, fmt.Errorf(
			"resolve MySQL %s comparison: %w",
			comparisonName,
			err,
		)
	}
	comparison.Name = comparisonName
	if err := validateMySQLIdentityComparison(comparison); err != nil {
		return mysqlIdentityComparison{}, err
	}
	return comparison, nil
}

func validateMySQLIdentityComparison(target mysqlIdentityComparison) error {
	identifiers := []struct {
		Name  string
		Value string
	}{
		{Name: "character set", Value: target.CharacterSet},
		{Name: "collation", Value: target.Collation},
	}
	for _, identifier := range identifiers {
		invalid := identifier.Value == "" || strings.ContainsFunc(identifier.Value, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '_')
		})
		if invalid {
			return fmt.Errorf(
				"invalid MySQL external identity %s %q for %s",
				identifier.Name,
				identifier.Value,
				target.Name,
			)
		}
	}
	return nil
}

func preflightMySQLIdentityColumnConversion(
	db *gorm.DB,
	tableName string,
	idColumnName string,
	subjectColumnName string,
	source mysqlIdentityComparison,
	target mysqlIdentityComparison,
) error {
	if db == nil || db.Dialector.Name() != "mysql" ||
		strings.EqualFold(source.CharacterSet, target.CharacterSet) {
		return nil
	}
	if err := validateMySQLIdentityComparison(source); err != nil {
		return err
	}
	if err := validateMySQLIdentityComparison(target); err != nil {
		return err
	}
	quotedID := db.Statement.Quote(clause.Column{Name: idColumnName})
	quotedSubject := db.Statement.Quote(clause.Column{Name: subjectColumnName})
	roundTrip := fmt.Sprintf(
		"CONVERT(CONVERT(%s USING %s) USING %s)",
		quotedSubject,
		target.CharacterSet,
		source.CharacterSet,
	)
	var unsafeRow struct {
		Id int64 `gorm:"column:id"`
	}
	result := db.Table(tableName).
		Select(quotedID+" AS id").
		Where(quotedSubject+" IS NOT NULL AND "+quotedSubject+" <> ?", "").
		Where("HEX(" + quotedSubject + ") <> HEX(" + roundTrip + ")").
		Order(quotedID).
		Limit(1).
		Scan(&unsafeRow)
	if result.Error != nil {
		return fmt.Errorf(
			"preflight MySQL %s conversion to %s: %w",
			source.Name,
			target.Name,
			result.Error,
		)
	}
	if result.RowsAffected > 0 {
		return fmt.Errorf(
			"MySQL %s row %d cannot be represented by %s character set %s; migration stopped before identity DDL",
			source.Name,
			unsafeRow.Id,
			target.Name,
			target.CharacterSet,
		)
	}
	return nil
}

func validateMySQLExternalIdentitySubjectStorage(db *gorm.DB, subject string) error {
	return validateMySQLIdentitySubjectStorage(
		db,
		&ExternalIdentityClaim{},
		ExternalIdentityClaim{}.TableName(),
		"subject",
		"external identity claim subject",
		subject,
	)
}

func validateMySQLIdentitySubjectStorage(
	db *gorm.DB,
	value any,
	tableName string,
	columnName string,
	comparisonName string,
	subject string,
) error {
	if db == nil || db.Dialector.Name() != "mysql" ||
		!db.Migrator().HasTable(value) ||
		!db.Migrator().HasColumn(value, columnName) {
		return nil
	}
	target, err := mysqlIdentityStringColumnDefinition(
		db,
		tableName,
		columnName,
		comparisonName,
	)
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(subject) > int(target.MaximumLength) {
		return fmt.Errorf(
			"%s exceeds configured MySQL column capacity of %d characters",
			comparisonName,
			target.MaximumLength,
		)
	}
	if strings.EqualFold(target.CharacterSet, "utf8mb4") {
		return nil
	}
	var representable bool
	expression := fmt.Sprintf(
		`SELECT HEX(CONVERT(CAST(? AS BINARY) USING utf8mb4)) =
HEX(CONVERT(CONVERT(CONVERT(CAST(? AS BINARY) USING utf8mb4) USING %s) USING utf8mb4))`,
		target.CharacterSet,
	)
	if err := db.Raw(expression, subject, subject).Scan(&representable).Error; err != nil {
		return fmt.Errorf("validate MySQL %s storage: %w", comparisonName, err)
	}
	if !representable {
		return fmt.Errorf(
			"%s cannot be represented by configured MySQL character set %s",
			comparisonName,
			target.CharacterSet,
		)
	}
	return nil
}

// ensureMySQLIdentityIndexStorage makes the two identity tables capable of
// holding their full, non-prefix unique keys. Wide character sets require
// large prefixes and DYNAMIC/COMPRESSED storage; narrower character sets that
// fit the legacy 767-byte limit remain compatible with older row formats.
func ensureMySQLIdentityIndexStorage(db *gorm.DB, value any, tableName string) error {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil
	}
	type mysqlVariable struct {
		Name  string `gorm:"column:Variable_name"`
		Value string `gorm:"column:Value"`
	}
	var variables []mysqlVariable
	if err := db.Raw(
		"SHOW VARIABLES WHERE Variable_name IN ('innodb_page_size', 'innodb_large_prefix')",
	).Scan(&variables).Error; err != nil {
		return fmt.Errorf("inspect MySQL identity index capacity: %w", err)
	}
	pageSize := 0
	largePrefixSupported := true
	for _, variable := range variables {
		switch strings.ToLower(variable.Name) {
		case "innodb_page_size":
			var err error
			pageSize, err = strconv.Atoi(variable.Value)
			if err != nil {
				return fmt.Errorf("invalid MySQL innodb_page_size %q: %w", variable.Value, err)
			}
		case "innodb_large_prefix":
			largePrefixSupported = strings.EqualFold(variable.Value, "ON") || variable.Value == "1"
		}
	}
	if pageSize == 0 {
		return errors.New("missing MySQL innodb_page_size while preparing identity indexes")
	}
	requiredKeyBytes, err := mysqlIdentityIndexRequiredBytes(db, tableName)
	if err != nil {
		return err
	}
	maxKeyBytes := pageSize * 3 / 16
	capacityInsufficient := requiredKeyBytes > mysqlLegacyIdentityIndexMaxBytes &&
		(!largePrefixSupported || maxKeyBytes < requiredKeyBytes)
	capacityError := func() error {
		return fmt.Errorf(
			"MySQL identity indexes require large prefixes and at least %d key bytes (innodb_page_size=%d, capacity=%d); migration stopped before identity DDL",
			requiredKeyBytes,
			pageSize,
			maxKeyBytes,
		)
	}

	if !db.Migrator().HasTable(value) {
		if capacityInsufficient {
			return capacityError()
		}
		tableOptions := "ENGINE=InnoDB"
		if requiredKeyBytes > mysqlLegacyIdentityIndexMaxBytes {
			tableOptions += " ROW_FORMAT=DYNAMIC"
		}
		if tableName == (ExternalIdentityClaim{}).TableName() {
			targets, err := mysqlExternalIdentityClaimComparisons(db)
			if err != nil {
				return err
			}
			if len(targets) != 1 {
				return errors.New("missing MySQL external identity claim target")
			}
			tableOptions += fmt.Sprintf(
				" DEFAULT CHARACTER SET %s COLLATE %s",
				targets[0].CharacterSet,
				targets[0].Collation,
			)
		}
		if err := db.Set("gorm:table_options", tableOptions).AutoMigrate(value); err != nil {
			return fmt.Errorf("create MySQL identity table %s with DYNAMIC row format: %w", tableName, err)
		}
	}
	type mysqlTableStorage struct {
		Engine    string `gorm:"column:engine"`
		RowFormat string `gorm:"column:row_format"`
	}
	loadStorage := func() (mysqlTableStorage, error) {
		var storage mysqlTableStorage
		err := db.Raw(
			`SELECT ENGINE AS engine, ROW_FORMAT AS row_format
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`,
			tableName,
		).Scan(&storage).Error
		return storage, err
	}
	storage, err := loadStorage()
	if err != nil {
		return fmt.Errorf("inspect MySQL identity table %s storage: %w", tableName, err)
	}
	if !strings.EqualFold(storage.Engine, "InnoDB") {
		return fmt.Errorf("MySQL identity table %s must use InnoDB, found %q", tableName, storage.Engine)
	}
	if requiredKeyBytes <= mysqlLegacyIdentityIndexMaxBytes {
		return nil
	}
	if strings.EqualFold(storage.RowFormat, "Dynamic") || strings.EqualFold(storage.RowFormat, "Compressed") {
		if !capacityInsufficient {
			return nil
		}
		complete, err := mysqlIdentityIndexStorageAlreadyPrepared(db, tableName)
		if err != nil {
			return err
		}
		if complete {
			// A later MySQL configuration change must not make an already valid,
			// fully indexed schema fail every startup. No wide-key DDL remains.
			return nil
		}
		return capacityError()
	}
	if capacityInsufficient {
		return capacityError()
	}
	quotedTable := db.Statement.Quote(clause.Table{Name: tableName})
	if err := db.Exec("ALTER TABLE " + quotedTable + " ROW_FORMAT=DYNAMIC").Error; err != nil {
		return fmt.Errorf("upgrade MySQL identity table %s to DYNAMIC row format: %w", tableName, err)
	}
	storage, err = loadStorage()
	if err != nil {
		return fmt.Errorf("verify MySQL identity table %s storage: %w", tableName, err)
	}
	if !strings.EqualFold(storage.RowFormat, "Dynamic") {
		return fmt.Errorf("MySQL identity table %s remained in unsupported row format %q", tableName, storage.RowFormat)
	}
	return nil
}

func mysqlIdentityIndexStorageAlreadyPrepared(db *gorm.DB, tableName string) (bool, error) {
	type expectedColumn struct {
		DataType          string
		MaximumLength     int64
		Nullable          bool
		HasDefault        bool
		DefaultValue      string
		DatetimePrecision int64
		AutoIncrement     bool
		StringColumn      bool
	}
	expectedColumns := map[string]expectedColumn{
		"id":         {DataType: "bigint", AutoIncrement: true},
		"provider":   {DataType: "varchar", MaximumLength: 32, StringColumn: true},
		"site_id":    {DataType: "bigint", HasDefault: true, DefaultValue: "0"},
		"subject":    {DataType: "varchar", MaximumLength: externalIdentitySubjectMaxLength, StringColumn: true},
		"user_id":    {DataType: "bigint"},
		"created_at": {DataType: "datetime", Nullable: true, DatetimePrecision: 3},
	}
	type expectedIndex struct {
		Name    string
		Columns []string
		Unique  bool
	}
	expectedIndexes := []expectedIndex{
		{Name: "PRIMARY", Columns: []string{"id"}, Unique: true},
		{Name: externalIdentitySiteSubjectIndex, Columns: []string{"provider", "site_id", "subject"}, Unique: true},
		{Name: "idx_external_identity_user", Columns: []string{"provider", "user_id"}, Unique: true},
		{Name: "idx_external_identity_claims_site_id", Columns: []string{"site_id"}},
		{Name: "idx_external_identity_claims_user_id", Columns: []string{"user_id"}},
	}
	if tableName == (UserOAuthBinding{}).TableName() {
		expectedColumns = map[string]expectedColumn{
			"id":               {DataType: "bigint", AutoIncrement: true},
			"user_id":          {DataType: "bigint"},
			"provider_id":      {DataType: "bigint"},
			"site_id":          {DataType: "bigint", HasDefault: true, DefaultValue: "0"},
			"provider_user_id": {DataType: "varchar", MaximumLength: externalIdentitySubjectMaxLength, StringColumn: true},
			"created_at":       {DataType: "datetime", Nullable: true, DatetimePrecision: 3},
		}
		expectedIndexes = []expectedIndex{
			{Name: "PRIMARY", Columns: []string{"id"}, Unique: true},
			{Name: "ux_user_provider", Columns: []string{"user_id", "provider_id"}, Unique: true},
			{Name: userOAuthBindingSiteSubjectIndex, Columns: []string{"provider_id", "site_id", "provider_user_id"}, Unique: true},
			{Name: "idx_user_oauth_bindings_site_id", Columns: []string{"site_id"}},
		}
	} else if tableName != (ExternalIdentityClaim{}).TableName() {
		return false, fmt.Errorf("unsupported MySQL identity table %q", tableName)
	}

	type mysqlColumnShape struct {
		Name              string `gorm:"column:column_name"`
		DataType          string `gorm:"column:data_type"`
		ColumnType        string `gorm:"column:column_type"`
		MaximumLength     int64  `gorm:"column:character_maximum_length"`
		IsNullable        string `gorm:"column:is_nullable"`
		HasDefault        bool   `gorm:"column:has_default"`
		DefaultValue      string `gorm:"column:default_value"`
		DatetimePrecision int64  `gorm:"column:datetime_precision"`
		CharacterSet      string `gorm:"column:character_set_name"`
		Collation         string `gorm:"column:collation_name"`
		Comment           string `gorm:"column:column_comment"`
		Extra             string `gorm:"column:extra"`
	}
	var columns []mysqlColumnShape
	if err := db.Raw(
		`SELECT COLUMN_NAME AS column_name, DATA_TYPE AS data_type, COLUMN_TYPE AS column_type,
COALESCE(CHARACTER_MAXIMUM_LENGTH, 0) AS character_maximum_length,
IS_NULLABLE AS is_nullable, COLUMN_DEFAULT IS NOT NULL AS has_default,
COALESCE(COLUMN_DEFAULT, '') AS default_value,
COALESCE(DATETIME_PRECISION, 0) AS datetime_precision,
COALESCE(CHARACTER_SET_NAME, '') AS character_set_name,
COALESCE(COLLATION_NAME, '') AS collation_name,
COLUMN_COMMENT AS column_comment, EXTRA AS extra
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`,
		tableName,
	).Scan(&columns).Error; err != nil {
		return false, fmt.Errorf("inspect completed MySQL identity table %s: %w", tableName, err)
	}
	foundColumns := make(map[string]mysqlColumnShape, len(columns))
	for _, column := range columns {
		foundColumns[strings.ToLower(column.Name)] = column
	}
	for name, expected := range expectedColumns {
		column, found := foundColumns[name]
		if !found || !strings.EqualFold(column.DataType, expected.DataType) ||
			strings.Contains(strings.ToLower(column.ColumnType), "unsigned") ||
			column.MaximumLength != expected.MaximumLength ||
			strings.EqualFold(column.IsNullable, "YES") != expected.Nullable ||
			column.HasDefault != expected.HasDefault ||
			(expected.HasDefault && column.DefaultValue != expected.DefaultValue) ||
			column.DatetimePrecision != expected.DatetimePrecision ||
			column.Comment != "" {
			return false, nil
		}
		hasAutoIncrement := strings.Contains(strings.ToLower(column.Extra), "auto_increment")
		if hasAutoIncrement != expected.AutoIncrement {
			return false, nil
		}
		if !expected.AutoIncrement && column.Extra != "" {
			return false, nil
		}
		if expected.StringColumn && (column.CharacterSet == "" || column.Collation == "") {
			return false, nil
		}
	}
	expectedIndexNames := make(map[string]struct{}, len(expectedIndexes))
	for _, expected := range expectedIndexes {
		expectedIndexNames[strings.ToLower(expected.Name)] = struct{}{}
		matches, err := mysqlIdentityIndexHasExactDefinition(
			db,
			tableName,
			expected.Name,
			expected.Columns,
			expected.Unique,
		)
		if err != nil {
			return false, err
		}
		if !matches {
			return false, nil
		}
	}
	indexColumns, err := mysqlIdentityIndexColumns(db, tableName, "")
	if err != nil {
		return false, err
	}
	groupedIndexes := make(map[string][]mysqlIdentityIndexColumn)
	for _, column := range indexColumns {
		groupedIndexes[strings.ToLower(column.IndexName)] = append(
			groupedIndexes[strings.ToLower(column.IndexName)],
			column,
		)
	}
	for name, columns := range groupedIndexes {
		if _, expected := expectedIndexNames[name]; expected || len(columns) != 1 ||
			columns[0].NonUnique != 0 || columns[0].ColumnName == nil {
			continue
		}
		if _, modelColumn := expectedColumns[strings.ToLower(*columns[0].ColumnName)]; modelColumn {
			// A stray single-column UNIQUE makes GORM report the field itself as
			// unique and can trigger a MODIFY on the next AutoMigrate. With large
			// prefixes disabled that rebuild would invalidate the existing wide key.
			return false, nil
		}
	}
	return true, nil
}

func mysqlIdentityIndexRequiredBytes(db *gorm.DB, tableName string) (int, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return 0, errors.New("MySQL identity index sizing requires a MySQL database")
	}
	charsetMaxBytes := func(characterSet string) (int, error) {
		var maxBytes int
		if err := db.Raw(
			`SELECT MAXLEN FROM information_schema.CHARACTER_SETS WHERE CHARACTER_SET_NAME = ?`,
			characterSet,
		).Scan(&maxBytes).Error; err != nil {
			return 0, fmt.Errorf("resolve MySQL character-set width for %s: %w", characterSet, err)
		}
		if maxBytes <= 0 {
			return 0, fmt.Errorf("missing MySQL character-set width for %s", characterSet)
		}
		return maxBytes, nil
	}

	subjectCharset := ""
	if db.Migrator().HasTable(tableName) {
		columnName := "subject"
		if tableName == (UserOAuthBinding{}).TableName() {
			columnName = "provider_user_id"
		}
		subject, err := mysqlIdentityStringColumnDefinition(
			db,
			tableName,
			columnName,
			"identity index subject",
		)
		if err != nil {
			return 0, err
		}
		subjectCharset = subject.CharacterSet
	} else if tableName == (ExternalIdentityClaim{}).TableName() {
		target, err := mysqlNewExternalIdentityClaimComparison(db)
		if err != nil {
			return 0, err
		}
		subjectCharset = target.CharacterSet
	} else {
		target, err := mysqlIdentityDatabaseDefaultComparison(db, "new custom OAuth binding target")
		if err != nil {
			return 0, err
		}
		subjectCharset = target.CharacterSet
	}
	subjectMaxBytes, err := charsetMaxBytes(subjectCharset)
	if err != nil {
		return 0, err
	}
	requiredBytes := externalIdentitySubjectMaxLength*subjectMaxBytes + 16
	if tableName == (ExternalIdentityClaim{}).TableName() {
		providerCharset := subjectCharset
		if db.Migrator().HasTable(tableName) {
			provider, err := mysqlIdentityStringColumnDefinition(
				db,
				tableName,
				"provider",
				"identity index provider",
			)
			if err != nil {
				return 0, err
			}
			providerCharset = provider.CharacterSet
		}
		providerMaxBytes, err := charsetMaxBytes(providerCharset)
		if err != nil {
			return 0, err
		}
		requiredBytes += 32 * providerMaxBytes
	}
	return requiredBytes, nil
}

// widenMySQLIdentitySubjectColumn preserves the existing character set and
// collation explicitly. Letting AutoMigrate issue a bare MODIFY can reset an
// explicitly collated utf8mb4 column to a narrower table default and mutate
// valid claim or binding subjects (for example, emoji) on non-STRICT servers.
func widenMySQLIdentitySubjectColumn(
	db *gorm.DB,
	value any,
	tableName string,
	columnName string,
	comparisonName string,
) error {
	if db == nil || db.Dialector.Name() != "mysql" || !db.Migrator().HasTable(value) ||
		!db.Migrator().HasColumn(value, columnName) {
		return nil
	}
	current, err := mysqlIdentityStringColumnDefinition(db, tableName, columnName, comparisonName)
	if err != nil {
		return err
	}
	if strings.EqualFold(current.DataType, "varchar") &&
		current.MaximumLength == externalIdentitySubjectMaxLength &&
		strings.EqualFold(current.IsNullable, "NO") &&
		!current.HasDefault && current.Comment == "" && current.Extra == "" {
		return nil
	}
	if !strings.EqualFold(current.DataType, "varchar") {
		return fmt.Errorf("MySQL %s must use VARCHAR storage, found %q", comparisonName, current.DataType)
	}
	if current.HasDefault || current.Comment != "" || current.Extra != "" {
		return fmt.Errorf(
			"MySQL %s has unsupported default, comment, or extra attributes; migration stopped before resizing",
			comparisonName,
		)
	}
	quotedTable := db.Statement.Quote(clause.Table{Name: tableName})
	quotedSubject := db.Statement.Quote(clause.Column{Name: columnName})
	var unsafeRows int64
	if err := db.Table(tableName).
		Where(quotedSubject+" IS NULL OR CHAR_LENGTH("+quotedSubject+") > ?", externalIdentitySubjectMaxLength).
		Count(&unsafeRows).Error; err != nil {
		return fmt.Errorf("validate MySQL %s before resizing: %w", comparisonName, err)
	}
	if unsafeRows > 0 {
		return fmt.Errorf(
			"MySQL %s contains %d null or oversized rows; migration stopped before resizing",
			comparisonName,
			unsafeRows,
		)
	}
	statement := fmt.Sprintf(
		"ALTER TABLE %s MODIFY COLUMN %s VARCHAR(%d) CHARACTER SET %s COLLATE %s NOT NULL",
		quotedTable,
		quotedSubject,
		externalIdentitySubjectMaxLength,
		current.CharacterSet,
		current.Collation,
	)
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("widen MySQL %s without changing collation: %w", comparisonName, err)
	}
	verified, err := mysqlIdentityStringColumnDefinition(db, tableName, columnName, comparisonName)
	if err != nil {
		return err
	}
	if !strings.EqualFold(verified.DataType, "varchar") ||
		verified.MaximumLength != externalIdentitySubjectMaxLength ||
		!strings.EqualFold(verified.IsNullable, "NO") || verified.HasDefault ||
		verified.Comment != "" || verified.Extra != "" ||
		!strings.EqualFold(verified.CharacterSet, current.CharacterSet) ||
		!strings.EqualFold(verified.Collation, current.Collation) {
		return fmt.Errorf("MySQL %s definition changed unexpectedly during widening", comparisonName)
	}
	return nil
}

// prepareExternalIdentityClaimsSiteScope upgrades the claim rows before
// AutoMigrate creates the replacement site-scoped unique index. The legacy
// global index remains in place throughout this phase, so an interrupted
// startup never leaves the table without subject-ownership enforcement.
func prepareExternalIdentityClaimsSiteScope(db *gorm.DB) error {
	if db.Dialector.Name() == "mysql" {
		if err := ensureMySQLIdentityIndexStorage(db, &ExternalIdentityClaim{}, ExternalIdentityClaim{}.TableName()); err != nil {
			return err
		}
		if err := widenMySQLIdentitySubjectColumn(
			db,
			&ExternalIdentityClaim{},
			ExternalIdentityClaim{}.TableName(),
			"subject",
			"external identity claim subject",
		); err != nil {
			return err
		}
	}
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

	if err := db.Select("id", "user_id").Find(&claims).Error; err != nil {
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
// across interrupted upgrades. Deployments must use a maintenance window that
// quiesces every old-version writer before migration, because older versions do
// not maintain the claim table or the site scope on custom OAuth bindings.
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
	if db.Dialector.Name() == "mysql" {
		return mysqlIdentityIndexIsFullUnique(
			db,
			ExternalIdentityClaim{}.TableName(),
			name,
			[]string{"provider", "site_id", "subject"},
		)
	}
	if db.Dialector.Name() == "postgres" {
		unconditional, err := postgresIdentityIndexIsUnconditional(
			db,
			ExternalIdentityClaim{}.TableName(),
			name,
		)
		if err != nil || !unconditional {
			return false, err
		}
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
		expected := []string{"provider", "site_id", "subject"}
		for position, column := range columns {
			if !strings.EqualFold(column, expected[position]) {
				return false, nil
			}
		}
		return true, nil
	}
	return false, nil
}

func mysqlIdentityIndexIsFullUnique(
	db *gorm.DB,
	tableName string,
	indexName string,
	expectedColumns []string,
) (bool, error) {
	return mysqlIdentityIndexHasExactDefinition(db, tableName, indexName, expectedColumns, true)
}

func mysqlIdentityIndexHasExactDefinition(
	db *gorm.DB,
	tableName string,
	indexName string,
	expectedColumns []string,
	expectedUnique bool,
) (bool, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return false, errors.New("MySQL identity index inspection requires a MySQL database")
	}
	columns, err := mysqlIdentityIndexColumns(db, tableName, indexName)
	if err != nil {
		return false, err
	}
	if len(columns) != len(expectedColumns) {
		return false, nil
	}
	for position, column := range columns {
		if (column.NonUnique == 0) != expectedUnique || column.SubPart != nil ||
			column.ColumnName == nil || !strings.EqualFold(*column.ColumnName, expectedColumns[position]) {
			return false, nil
		}
	}
	return true, nil
}

type mysqlIdentityIndexColumn struct {
	IndexName  string  `gorm:"column:index_name"`
	ColumnName *string `gorm:"column:column_name"`
	SubPart    *int64  `gorm:"column:sub_part"`
	NonUnique  int     `gorm:"column:non_unique"`
}

func mysqlIdentityIndexColumns(db *gorm.DB, tableName string, indexName string) ([]mysqlIdentityIndexColumn, error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil, errors.New("MySQL identity index inspection requires a MySQL database")
	}
	query := `SELECT INDEX_NAME AS index_name, COLUMN_NAME AS column_name,
       SUB_PART AS sub_part, NON_UNIQUE AS non_unique
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`
	args := []any{tableName}
	if indexName != "" {
		query += " AND INDEX_NAME = ?"
		args = append(args, indexName)
	}
	query += " ORDER BY INDEX_NAME, SEQ_IN_INDEX"
	var columns []mysqlIdentityIndexColumn
	if err := db.Raw(query, args...).Scan(&columns).Error; err != nil {
		return nil, err
	}
	return columns, nil
}

func postgresIdentityIndexIsUnconditional(db *gorm.DB, tableName string, indexName string) (bool, error) {
	if db == nil || db.Dialector.Name() != "postgres" {
		return false, errors.New("PostgreSQL identity index inspection requires a PostgreSQL database")
	}
	type postgresIndexShape struct {
		Unconditional bool `gorm:"column:unconditional"`
		PlainColumns  bool `gorm:"column:plain_columns"`
	}
	var shape postgresIndexShape
	result := db.Raw(
		`SELECT ix.indpred IS NULL AS unconditional, ix.indexprs IS NULL AS plain_columns
FROM pg_catalog.pg_index ix
JOIN pg_catalog.pg_class relation ON relation.oid = ix.indrelid
JOIN pg_catalog.pg_class index_relation ON index_relation.oid = ix.indexrelid
WHERE relation.oid = pg_catalog.to_regclass(CAST(? AS text)) AND index_relation.relname = ?`,
		tableName,
		indexName,
	).Scan(&shape)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1 && shape.Unconditional && shape.PlainColumns, nil
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
	if db.Dialector.Name() == "mysql" {
		columns, err := mysqlIdentityIndexColumns(db, ExternalIdentityClaim{}.TableName(), "")
		if err != nil {
			return nil, err
		}
		grouped := make(map[string][]mysqlIdentityIndexColumn)
		var order []string
		for _, column := range columns {
			if _, exists := grouped[column.IndexName]; !exists {
				order = append(order, column.IndexName)
			}
			grouped[column.IndexName] = append(grouped[column.IndexName], column)
		}
		var names []string
		for _, name := range order {
			indexColumns := grouped[name]
			if len(indexColumns) != 2 || indexColumns[0].NonUnique != 0 ||
				indexColumns[1].NonUnique != 0 || indexColumns[0].ColumnName == nil ||
				indexColumns[1].ColumnName == nil {
				continue
			}
			first := strings.ToLower(*indexColumns[0].ColumnName)
			second := strings.ToLower(*indexColumns[1].ColumnName)
			if (first == "provider" && second == "subject") ||
				(first == "subject" && second == "provider") {
				names = append(names, name)
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
		if !((known && unique) || (!known && index.Name() == legacyExternalIdentityGlobalSubjectIndex)) {
			continue
		}
		if db.Dialector.Name() == "postgres" {
			unconditional, err := postgresIdentityIndexIsUnconditional(
				db,
				ExternalIdentityClaim{}.TableName(),
				index.Name(),
			)
			if err != nil {
				return nil, err
			}
			if !unconditional {
				continue
			}
		}
		names = append(names, index.Name())
	}
	return names, nil
}

func sqliteIndexColumns(sql string) ([]string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(sql))
	normalized = strings.TrimSpace(strings.TrimSuffix(normalized, ";"))
	if !strings.HasPrefix(normalized, "create unique index") {
		return nil, false
	}
	columnsAt := strings.Index(normalized, "(")
	if columnsAt < 0 {
		return nil, false
	}
	columnsEnd := -1
	depth := 0
	for position := columnsAt; position < len(normalized); position++ {
		switch normalized[position] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				columnsEnd = position
			}
		}
		if columnsEnd >= 0 {
			break
		}
	}
	if columnsEnd <= columnsAt || strings.TrimSpace(normalized[columnsEnd+1:]) != "" {
		// Partial indexes and any other trailing clause do not provide the
		// unconditional ownership constraint required by the migration.
		return nil, false
	}
	parts := strings.Split(normalized[columnsAt+1:columnsEnd], ",")
	for i := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(parts[i]), "`\"[]")
		if parts[i] == "" || strings.ContainsAny(parts[i], " ()") {
			return nil, false
		}
	}
	return parts, true
}
