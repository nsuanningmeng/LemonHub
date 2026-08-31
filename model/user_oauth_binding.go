package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	userOAuthBindingSiteSubjectIndex         = "ux_provider_site_userid"
	legacyUserOAuthBindingGlobalSubjectIndex = "ux_provider_userid"
)

// UserOAuthBinding stores the binding relationship between users and custom OAuth providers
type UserOAuthBinding struct {
	Id             int       `json:"id" gorm:"primaryKey"`
	UserId         int       `json:"user_id" gorm:"not null;uniqueIndex:ux_user_provider,priority:1"` // User ID - one binding per user per provider
	ProviderId     int       `json:"provider_id" gorm:"not null;uniqueIndex:ux_user_provider,priority:2;uniqueIndex:ux_provider_site_userid,priority:1"`
	SiteId         int       `json:"site_id" gorm:"type:int;not null;default:0;index;uniqueIndex:ux_provider_site_userid,priority:2"`
	ProviderUserId string    `json:"provider_user_id" gorm:"type:varchar(256);not null;uniqueIndex:ux_provider_site_userid,priority:3"`
	CreatedAt      time.Time `json:"created_at"`
}

func (UserOAuthBinding) TableName() string {
	return "user_oauth_bindings"
}

// GetUserOAuthBindingsByUserId returns all OAuth bindings for a user
func GetUserOAuthBindingsByUserId(userId int) ([]*UserOAuthBinding, error) {
	var bindings []*UserOAuthBinding
	err := DB.Where("user_id = ?", userId).Find(&bindings).Error
	return bindings, err
}

// GetUserOAuthBinding returns a specific binding for a user and provider
func GetUserOAuthBinding(userId, providerId int) (*UserOAuthBinding, error) {
	var binding UserOAuthBinding
	err := DB.Where("user_id = ? AND provider_id = ?", userId, providerId).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

// GetUserByOAuthBinding finds a user by provider ID and provider user ID
func GetUserByOAuthBinding(providerId int, providerUserId string, siteId int) (*User, error) {
	providerUserId, err := NormalizeExternalIdentitySubject(providerUserId)
	if err != nil {
		return nil, err
	}
	if err := validateMySQLIdentitySubjectStorage(
		DB,
		&UserOAuthBinding{},
		UserOAuthBinding{}.TableName(),
		"provider_user_id",
		"custom OAuth binding subject",
		providerUserId,
	); err != nil {
		return nil, err
	}
	var binding UserOAuthBinding
	err = DB.Where("provider_id = ? AND site_id = ? AND provider_user_id = ?", providerId, siteId, providerUserId).First(&binding).Error
	if err != nil {
		return nil, err
	}

	var user User
	err = DB.First(&user, binding.UserId).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// IsProviderUserIdTaken checks if a provider user ID is already bound to any user
func IsProviderUserIdTaken(providerId int, providerUserId string, siteId int) bool {
	providerUserId, err := NormalizeExternalIdentitySubject(providerUserId)
	if err != nil {
		return false
	}
	if err := validateMySQLIdentitySubjectStorage(
		DB,
		&UserOAuthBinding{},
		UserOAuthBinding{}.TableName(),
		"provider_user_id",
		"custom OAuth binding subject",
		providerUserId,
	); err != nil {
		return false
	}
	var count int64
	DB.Model(&UserOAuthBinding{}).
		Where("provider_id = ? AND site_id = ? AND provider_user_id = ?", providerId, siteId, providerUserId).
		Count(&count)
	return count > 0
}

// CreateUserOAuthBinding creates a new OAuth binding
func CreateUserOAuthBinding(binding *UserOAuthBinding) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return CreateUserOAuthBindingWithTx(tx, binding)
	})
}

// CreateUserOAuthBindingWithTx creates a new OAuth binding within a transaction
func CreateUserOAuthBindingWithTx(tx *gorm.DB, binding *UserOAuthBinding) error {
	if tx == nil || binding == nil || binding.UserId == 0 {
		return errors.New("user ID is required")
	}
	if binding.ProviderId <= 0 {
		return errors.New("provider ID is required")
	}
	var err error
	binding.ProviderUserId, err = NormalizeExternalIdentitySubject(binding.ProviderUserId)
	if err != nil {
		return err
	}
	if err := validateMySQLIdentitySubjectStorage(
		tx,
		&UserOAuthBinding{},
		UserOAuthBinding{}.TableName(),
		"provider_user_id",
		"custom OAuth binding subject",
		binding.ProviderUserId,
	); err != nil {
		return err
	}
	if err := lockCustomOAuthProviderForBinding(tx, binding.ProviderId); err != nil {
		return err
	}
	var owner User
	if err := lockForUpdate(tx).Select("id", "site_id").Where("id = ?", binding.UserId).First(&owner).Error; err != nil {
		return err
	}
	binding.SiteId = owner.SiteId
	if err := ClaimExternalIdentityWithTx(tx, customOAuthExternalIdentityProvider(binding.ProviderId), binding.ProviderUserId, binding.UserId); err != nil {
		return err
	}

	binding.CreatedAt = time.Now()
	return tx.Create(binding).Error
}

// UpdateUserOAuthBinding updates an existing OAuth binding (e.g., rebind to different OAuth account)
func UpdateUserOAuthBinding(userId, providerId int, newProviderUserId string) error {
	var err error
	newProviderUserId, err = NormalizeExternalIdentitySubject(newProviderUserId)
	if userId <= 0 || providerId <= 0 || err != nil {
		return errors.New("invalid OAuth binding")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := validateMySQLIdentitySubjectStorage(
			tx,
			&UserOAuthBinding{},
			UserOAuthBinding{}.TableName(),
			"provider_user_id",
			"custom OAuth binding subject",
			newProviderUserId,
		); err != nil {
			return err
		}
		if err := lockCustomOAuthProviderForBinding(tx, providerId); err != nil {
			return err
		}
		var owner User
		if err := lockForUpdate(tx).Select("id", "site_id").Where("id = ?", userId).First(&owner).Error; err != nil {
			return err
		}
		var binding UserOAuthBinding
		err := tx.Where("user_id = ? AND provider_id = ?", userId, providerId).First(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CreateUserOAuthBindingWithTx(tx, &UserOAuthBinding{
				UserId: userId, ProviderId: providerId, ProviderUserId: newProviderUserId,
			})
		}
		if err != nil {
			return err
		}
		if binding.ProviderUserId == newProviderUserId && binding.SiteId == owner.SiteId {
			return nil
		}
		provider := customOAuthExternalIdentityProvider(providerId)
		if err := ReleaseExternalIdentityWithTx(tx, provider, userId); err != nil {
			return err
		}
		if err := ClaimExternalIdentityWithTx(tx, provider, newProviderUserId, userId); err != nil {
			return err
		}
		result := tx.Model(&UserOAuthBinding{}).Where("id = ?", binding.Id).
			Updates(map[string]interface{}{"site_id": owner.SiteId, "provider_user_id": newProviderUserId})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// DeleteUserOAuthBinding deletes an OAuth binding
func DeleteUserOAuthBinding(userId, providerId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := ReleaseExternalIdentityWithTx(tx, customOAuthExternalIdentityProvider(providerId), userId); err != nil {
			return err
		}
		return tx.Where("user_id = ? AND provider_id = ?", userId, providerId).Delete(&UserOAuthBinding{}).Error
	})
}

func deleteUserOAuthBindingsByUserId(tx *gorm.DB, userId int) error {
	return tx.Where("user_id = ?", userId).Delete(&UserOAuthBinding{}).Error
}

// GetBindingCountByProviderId returns the number of bindings for a provider
func GetBindingCountByProviderId(providerId int) (int64, error) {
	var count int64
	err := DB.Model(&UserOAuthBinding{}).Where("provider_id = ?", providerId).Count(&count).Error
	return count, err
}

func lockCustomOAuthProviderForBinding(tx *gorm.DB, providerId int) error {
	if tx == nil || providerId <= 0 {
		return errors.New("invalid custom OAuth provider")
	}
	var provider CustomOAuthProvider
	return lockForUpdate(tx).Select("id").Where("id = ?", providerId).First(&provider).Error
}

// preflightUserOAuthBindingsSiteScope resolves every binding through its owner
// before DDL. Ambiguous same-site legacy ownership or orphaned rows stop startup
// explicitly; no account is selected by row order and no data is auto-deduped.
func preflightUserOAuthBindingsSiteScope(db *gorm.DB) error {
	if !db.Migrator().HasTable(&UserOAuthBinding{}) {
		return preflightCustomOAuthClaimConsistency(db, nil, false)
	}
	var bindings []UserOAuthBinding
	if err := db.Select("id", "user_id", "provider_id", "provider_user_id").Order("id").Find(&bindings).Error; err != nil {
		return fmt.Errorf("preflight custom OAuth bindings: %w", err)
	}
	type subjectKey struct {
		ProviderId int
		SiteId     int
		Subject    string
	}
	type userKey struct {
		UserId     int
		ProviderId int
	}
	subjectOwners := make(map[subjectKey]int, len(bindings))
	userBindings := make(map[userKey]int, len(bindings))
	providers := make(map[int]struct{})
	userHasSite := db.Migrator().HasColumn(&User{}, "site_id")
	for _, binding := range bindings {
		if binding.UserId <= 0 || binding.ProviderId <= 0 {
			return fmt.Errorf("invalid custom OAuth binding %d", binding.Id)
		}
		subject, err := NormalizeExternalIdentitySubject(binding.ProviderUserId)
		if err != nil {
			return fmt.Errorf("invalid provider user ID on custom OAuth binding %d: %w", binding.Id, err)
		}
		if binding.ProviderUserId != subject {
			return fmt.Errorf("non-canonical whitespace in provider user ID on custom OAuth binding %d", binding.Id)
		}
		if _, checked := providers[binding.ProviderId]; !checked {
			if !db.Migrator().HasTable(&CustomOAuthProvider{}) {
				return fmt.Errorf("custom OAuth binding %d references missing provider %d", binding.Id, binding.ProviderId)
			}
			var provider CustomOAuthProvider
			if err := db.Select("id").Where("id = ?", binding.ProviderId).First(&provider).Error; err != nil {
				return fmt.Errorf("resolve provider for custom OAuth binding %d: %w", binding.Id, err)
			}
			providers[binding.ProviderId] = struct{}{}
		}
		columns := []string{"id"}
		if userHasSite {
			columns = append(columns, "site_id")
		}
		var owner User
		if err := db.Unscoped().Select(columns).Where("id = ?", binding.UserId).First(&owner).Error; err != nil {
			return fmt.Errorf("resolve owner for custom OAuth binding %d: %w", binding.Id, err)
		}
		sKey := subjectKey{ProviderId: binding.ProviderId, SiteId: owner.SiteId, Subject: subject}
		if existingUserId, exists := subjectOwners[sKey]; exists && existingUserId != binding.UserId {
			return fmt.Errorf("ambiguous custom OAuth provider %d ownership in site %d: users %d and %d share one provider ID",
				binding.ProviderId, owner.SiteId, existingUserId, binding.UserId)
		}
		uKey := userKey{UserId: binding.UserId, ProviderId: binding.ProviderId}
		if existingBindingId, exists := userBindings[uKey]; exists {
			return fmt.Errorf("ambiguous custom OAuth bindings %d and %d for user %d provider %d",
				existingBindingId, binding.Id, binding.UserId, binding.ProviderId)
		}
		subjectOwners[sKey] = binding.UserId
		userBindings[uKey] = binding.Id
	}
	if err := preflightUserOAuthBindingCollationConflicts(db, userHasSite, len(bindings)); err != nil {
		return err
	}
	return preflightCustomOAuthClaimConsistency(db, bindings, userHasSite)
}

func preflightCustomOAuthClaimConsistency(db *gorm.DB, bindings []UserOAuthBinding, userHasSite bool) error {
	if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
		return nil
	}
	type bindingKey struct {
		ProviderId int
		UserId     int
	}
	bindingByUser := make(map[bindingKey]UserOAuthBinding, len(bindings))
	providers := make(map[int]struct{})
	for _, binding := range bindings {
		bindingByUser[bindingKey{ProviderId: binding.ProviderId, UserId: binding.UserId}] = binding
		providers[binding.ProviderId] = struct{}{}
	}
	var claims []ExternalIdentityClaim
	const customProviderPrefix = "custom_oauth:"
	if err := db.Select("id", "provider", "subject", "user_id").
		Where("substr(provider, 1, ?) = ?", len(customProviderPrefix), customProviderPrefix).
		Order("id").
		Find(&claims).Error; err != nil {
		return fmt.Errorf("preflight persisted custom OAuth claims: %w", err)
	}
	for _, claim := range claims {
		providerText := strings.TrimPrefix(claim.Provider, customProviderPrefix)
		providerId, err := strconv.Atoi(providerText)
		if err != nil || providerId <= 0 || customOAuthExternalIdentityProvider(providerId) != claim.Provider {
			return fmt.Errorf("invalid custom OAuth provider on persisted external identity claim %d", claim.Id)
		}
		subject, err := NormalizeExternalIdentitySubject(claim.Subject)
		if err != nil || subject != claim.Subject {
			return fmt.Errorf("invalid subject on persisted custom OAuth claim %d", claim.Id)
		}
		binding, exists := bindingByUser[bindingKey{ProviderId: providerId, UserId: claim.UserId}]
		if !exists || binding.ProviderUserId != subject {
			return fmt.Errorf(
				"persisted custom OAuth claim %d conflicts with provider %d binding for user %d",
				claim.Id,
				providerId,
				claim.UserId,
			)
		}
		providers[providerId] = struct{}{}
	}
	if len(bindings) == 0 || len(claims) == 0 {
		return nil
	}

	bindingTable := UserOAuthBinding{}.TableName()
	claimTable := ExternalIdentityClaim{}.TableName()
	userTable := db.NamingStrategy.TableName("User")
	bindingSubject := db.Statement.Quote(clause.Column{Table: "b", Name: "provider_user_id"})
	bindingUser := db.Statement.Quote(clause.Column{Table: "b", Name: "user_id"})
	claimSubject := db.Statement.Quote(clause.Column{Table: "c", Name: "subject"})
	claimUser := db.Statement.Quote(clause.Column{Table: "c", Name: "user_id"})
	bindingSite := "0"
	claimSite := "0"
	subjectEquality := fmt.Sprintf(
		"(%s = %s OR %s = %s)",
		claimSubject,
		bindingSubject,
		bindingSubject,
		claimSubject,
	)
	if db.Dialector.Name() == "mysql" {
		targets, err := mysqlExternalIdentityClaimComparisons(db)
		if err != nil {
			return err
		}
		if len(targets) != 1 {
			return errors.New("missing MySQL external identity claim comparison")
		}
		subjectEquality = fmt.Sprintf(
			"CONVERT(%s USING %s) COLLATE %s = %s",
			bindingSubject,
			targets[0].CharacterSet,
			targets[0].Collation,
			claimSubject,
		)
	}
	if userHasSite {
		bindingSite = "COALESCE(" + db.Statement.Quote(clause.Column{Table: "bu", Name: "site_id"}) + ", 0)"
		claimSite = "COALESCE(" + db.Statement.Quote(clause.Column{Table: "cu", Name: "site_id"}) + ", 0)"
	}
	joinBindingOwner := fmt.Sprintf(
		"JOIN %s AS bu ON %s = %s",
		db.Statement.Quote(clause.Table{Name: userTable}),
		db.Statement.Quote(clause.Column{Table: "bu", Name: "id"}),
		bindingUser,
	)
	joinClaimOwner := fmt.Sprintf(
		"JOIN %s AS cu ON %s = %s",
		db.Statement.Quote(clause.Table{Name: userTable}),
		db.Statement.Quote(clause.Column{Table: "cu", Name: "id"}),
		claimUser,
	)
	for providerId := range providers {
		joinClaim := fmt.Sprintf(
			"JOIN %s AS c ON %s = ? AND %s",
			db.Statement.Quote(clause.Table{Name: claimTable}),
			db.Statement.Quote(clause.Column{Table: "c", Name: "provider"}),
			subjectEquality,
		)
		var conflict struct {
			BindingId   int   `gorm:"column:binding_id"`
			BindingUser int   `gorm:"column:binding_user_id"`
			ClaimId     int64 `gorm:"column:claim_id"`
			ClaimUser   int   `gorm:"column:claim_user_id"`
		}
		result := db.Table(bindingTable+" AS b").
			Joins(joinBindingOwner).
			Joins(joinClaim, customOAuthExternalIdentityProvider(providerId)).
			Joins(joinClaimOwner).
			Select(fmt.Sprintf(
				"%s AS binding_id, %s AS binding_user_id, %s AS claim_id, %s AS claim_user_id",
				db.Statement.Quote(clause.Column{Table: "b", Name: "id"}),
				bindingUser,
				db.Statement.Quote(clause.Column{Table: "c", Name: "id"}),
				claimUser,
			)).
			Where(db.Statement.Quote(clause.Column{Table: "b", Name: "provider_id"})+" = ?", providerId).
			Where(claimUser + " <> " + bindingUser).
			Where(bindingSite + " = " + claimSite).
			Limit(1).
			Scan(&conflict)
		if result.Error != nil {
			return fmt.Errorf("preflight custom OAuth provider %d bindings against persisted claims: %w", providerId, result.Error)
		}
		if result.RowsAffected > 0 {
			return fmt.Errorf(
				"persisted custom OAuth claim %d for user %d conflicts with binding %d for user %d",
				conflict.ClaimId,
				conflict.ClaimUser,
				conflict.BindingId,
				conflict.BindingUser,
			)
		}
	}
	return nil
}

func preflightUserOAuthBindingCollationConflicts(db *gorm.DB, userHasSite bool, bindingCount int) error {
	bindingTable := UserOAuthBinding{}.TableName()
	userTable := db.NamingStrategy.TableName("User")
	providerColumn := db.Statement.Quote(clause.Column{Table: "b", Name: "provider_id"})
	userColumn := db.Statement.Quote(clause.Column{Table: "b", Name: "user_id"})
	subjectColumn := db.Statement.Quote(clause.Column{Table: "b", Name: "provider_user_id"})
	siteExpression := "0"
	groupBySite := ""
	if userHasSite {
		siteColumn := db.Statement.Quote(clause.Column{Table: "u", Name: "site_id"})
		siteExpression = "COALESCE(" + siteColumn + ", 0)"
		groupBySite = siteExpression + ", "
	}

	type identityComparison struct {
		Name       string
		Expression string
	}
	comparisons := []identityComparison{{Name: "source", Expression: subjectColumn}}
	if db.Dialector.Name() == "mysql" {
		source, err := mysqlIdentityStringColumnDefinition(
			db,
			bindingTable,
			"provider_user_id",
			"custom OAuth binding source",
		)
		if err != nil {
			return err
		}
		targets, err := mysqlExternalIdentityClaimComparisons(db)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if err := preflightMySQLIdentityColumnConversion(
				db,
				bindingTable,
				"id",
				"provider_user_id",
				source,
				target,
			); err != nil {
				return err
			}
			if strings.EqualFold(source.CharacterSet, target.CharacterSet) &&
				strings.EqualFold(source.Collation, target.Collation) {
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
	if bindingCount < 2 {
		return nil
	}

	joinUsers := fmt.Sprintf(
		"JOIN %s AS u ON %s = %s",
		db.Statement.Quote(clause.Table{Name: userTable}),
		db.Statement.Quote(clause.Column{Table: "u", Name: "id"}),
		userColumn,
	)
	for _, comparison := range comparisons {
		var collision struct {
			ProviderId     int
			SiteId         int
			FirstUserId    int
			LastUserId     int
			DuplicateCount int64
		}
		err := db.Table(bindingTable+" AS b").
			Joins(joinUsers).
			Select(fmt.Sprintf(
				"%s AS provider_id, %s AS site_id, MIN(%s) AS first_user_id, MAX(%s) AS last_user_id, COUNT(*) AS duplicate_count",
				providerColumn,
				siteExpression,
				userColumn,
				userColumn,
			)).
			Where(subjectColumn+" IS NOT NULL AND "+subjectColumn+" <> ?", "").
			Group(providerColumn + ", " + groupBySite + comparison.Expression).
			Having("COUNT(*) > 1").
			Limit(1).
			Scan(&collision).Error
		if err != nil {
			return fmt.Errorf("preflight custom OAuth binding ownership under %s database collation: %w", comparison.Name, err)
		}
		if collision.DuplicateCount > 1 {
			return fmt.Errorf(
				"ambiguous custom OAuth provider %d ownership in site %d under database collation (%s): users %d and %d share equivalent provider IDs",
				collision.ProviderId,
				collision.SiteId,
				comparison.Name,
				collision.FirstUserId,
				collision.LastUserId,
			)
		}
	}
	return nil
}

func prepareUserOAuthBindingsSiteScope(db *gorm.DB) error {
	if db.Dialector.Name() == "mysql" {
		if err := ensureMySQLIdentityIndexStorage(db, &UserOAuthBinding{}, UserOAuthBinding{}.TableName()); err != nil {
			return err
		}
		if err := widenMySQLIdentitySubjectColumn(
			db,
			&UserOAuthBinding{},
			UserOAuthBinding{}.TableName(),
			"provider_user_id",
			"custom OAuth binding subject",
		); err != nil {
			return err
		}
	}
	if !db.Migrator().HasTable(&UserOAuthBinding{}) {
		return nil
	}
	prepare := func(tx *gorm.DB) error {
		var bindings []UserOAuthBinding
		if err := tx.Select("id", "user_id").Find(&bindings).Error; err != nil {
			return fmt.Errorf("load custom OAuth bindings for site backfill: %w", err)
		}
		// migrateDBFast creates User and binding columns concurrently. If a
		// process stopped after the binding table was created but before users
		// gained site_id, the next run must still be able to resume; such legacy
		// users belong to the main site.
		userHasSite := tx.Migrator().HasColumn(&User{}, "site_id")
		ownerSites := make(map[int]int, len(bindings))
		for _, binding := range bindings {
			var owner User
			ownerColumns := []string{"id"}
			if userHasSite {
				ownerColumns = append(ownerColumns, "site_id")
			}
			if err := tx.Unscoped().Select(ownerColumns).Where("id = ?", binding.UserId).First(&owner).Error; err != nil {
				return fmt.Errorf("resolve site for custom OAuth binding %d: %w", binding.Id, err)
			}
			ownerSites[binding.Id] = owner.SiteId
		}
		if !tx.Migrator().HasColumn(&UserOAuthBinding{}, "site_id") {
			if err := tx.Migrator().AddColumn(&UserOAuthBinding{}, "SiteId"); err != nil {
				return fmt.Errorf("add custom OAuth binding site scope: %w", err)
			}
		}
		for id, siteId := range ownerSites {
			if err := tx.Model(&UserOAuthBinding{}).Where("id = ?", id).Update("site_id", siteId).Error; err != nil {
				return fmt.Errorf("backfill site for custom OAuth binding %d: %w", id, err)
			}
		}
		siteScoped, err := userOAuthBindingSubjectIndexIsSiteScoped(tx, userOAuthBindingSiteSubjectIndex)
		if err != nil {
			return err
		}
		if !siteScoped {
			if tx.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex) {
				return fmt.Errorf("custom OAuth binding index %s has an unexpected definition", userOAuthBindingSiteSubjectIndex)
			}
			if err := tx.Migrator().CreateIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex); err != nil {
				return fmt.Errorf("create site-scoped custom OAuth binding index: %w", err)
			}
			siteScoped, err = userOAuthBindingSubjectIndexIsSiteScoped(tx, userOAuthBindingSiteSubjectIndex)
			if err != nil {
				return fmt.Errorf("verify site-scoped custom OAuth binding index: %w", err)
			}
			if !siteScoped {
				return fmt.Errorf("custom OAuth binding index %s was not created with full unique columns", userOAuthBindingSiteSubjectIndex)
			}
		}
		legacyIndexes, err := legacyGlobalUserOAuthBindingSubjectIndexes(tx)
		if err != nil {
			return fmt.Errorf("inspect global custom OAuth binding indexes: %w", err)
		}
		canonicalLegacyFound := false
		for _, name := range legacyIndexes {
			if name == legacyUserOAuthBindingGlobalSubjectIndex {
				canonicalLegacyFound = true
			}
		}
		if tx.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex) &&
			!canonicalLegacyFound {
			return fmt.Errorf("custom OAuth binding index %s has an unexpected definition", legacyUserOAuthBindingGlobalSubjectIndex)
		}
		for _, name := range legacyIndexes {
			if err := tx.Migrator().DropIndex(&UserOAuthBinding{}, name); err != nil {
				return fmt.Errorf("drop global custom OAuth binding index %s: %w", name, err)
			}
		}
		return nil
	}
	if db.Dialector.Name() == "sqlite" {
		return db.Transaction(prepare)
	}
	return prepare(db)
}

func finalizeUserOAuthBindingsSiteScope(db *gorm.DB) error {
	if !db.Migrator().HasTable(&UserOAuthBinding{}) {
		return nil
	}
	siteScoped, err := userOAuthBindingSubjectIndexIsSiteScoped(db, userOAuthBindingSiteSubjectIndex)
	if err != nil {
		return err
	}
	if !siteScoped {
		return fmt.Errorf("site-scoped custom OAuth binding index %s was not created", userOAuthBindingSiteSubjectIndex)
	}
	legacyIndexes, err := legacyGlobalUserOAuthBindingSubjectIndexes(db)
	if err != nil {
		return fmt.Errorf("inspect global custom OAuth binding indexes: %w", err)
	}
	canonicalLegacyFound := false
	for _, name := range legacyIndexes {
		if name == legacyUserOAuthBindingGlobalSubjectIndex {
			canonicalLegacyFound = true
		}
	}
	if db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex) &&
		!canonicalLegacyFound {
		return fmt.Errorf("custom OAuth binding index %s has an unexpected definition", legacyUserOAuthBindingGlobalSubjectIndex)
	}
	for _, name := range legacyIndexes {
		if err := db.Migrator().DropIndex(&UserOAuthBinding{}, name); err != nil {
			return fmt.Errorf("drop global custom OAuth binding index %s: %w", name, err)
		}
	}
	return nil
}

func legacyGlobalUserOAuthBindingSubjectIndexes(db *gorm.DB) ([]string, error) {
	tableName := UserOAuthBinding{}.TableName()
	if db.Dialector.Name() == "sqlite" {
		var indexes []struct {
			Name string `gorm:"column:name"`
			SQL  string `gorm:"column:sql"`
		}
		if err := db.Raw(
			"SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL",
			tableName,
		).Scan(&indexes).Error; err != nil {
			return nil, err
		}
		var names []string
		for _, index := range indexes {
			columns, unique := sqliteIndexColumns(index.SQL)
			if unique && len(columns) == 2 &&
				((columns[0] == "provider_id" && columns[1] == "provider_user_id") ||
					(columns[0] == "provider_user_id" && columns[1] == "provider_id")) {
				names = append(names, index.Name)
			}
		}
		return names, nil
	}
	if db.Dialector.Name() == "mysql" {
		columns, err := mysqlIdentityIndexColumns(db, tableName, "")
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
			if (first == "provider_id" && second == "provider_user_id") ||
				(first == "provider_user_id" && second == "provider_id") {
				names = append(names, name)
			}
		}
		return names, nil
	}

	indexes, err := db.Migrator().GetIndexes(&UserOAuthBinding{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, index := range indexes {
		unique, known := index.Unique()
		columns := index.Columns()
		if !known || !unique || len(columns) != 2 {
			continue
		}
		first := strings.ToLower(columns[0])
		second := strings.ToLower(columns[1])
		if (first == "provider_id" && second == "provider_user_id") ||
			(first == "provider_user_id" && second == "provider_id") {
			if db.Dialector.Name() == "postgres" {
				unconditional, err := postgresIdentityIndexIsUnconditional(db, tableName, index.Name())
				if err != nil {
					return nil, err
				}
				if !unconditional {
					continue
				}
			}
			names = append(names, index.Name())
		}
	}
	return names, nil
}

func userOAuthBindingSubjectIndexIsSiteScoped(db *gorm.DB, name string) (bool, error) {
	if db.Dialector.Name() == "sqlite" {
		var sql string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?",
			UserOAuthBinding{}.TableName(), name).Scan(&sql).Error; err != nil {
			return false, err
		}
		parts, unique := sqliteIndexColumns(sql)
		if !unique {
			return false, nil
		}
		if len(parts) != 3 {
			return false, nil
		}
		return parts[0] == "provider_id" && parts[1] == "site_id" && parts[2] == "provider_user_id", nil
	}
	if db.Dialector.Name() == "mysql" {
		return mysqlIdentityIndexIsFullUnique(
			db,
			UserOAuthBinding{}.TableName(),
			name,
			[]string{"provider_id", "site_id", "provider_user_id"},
		)
	}
	if db.Dialector.Name() == "postgres" {
		unconditional, err := postgresIdentityIndexIsUnconditional(
			db,
			UserOAuthBinding{}.TableName(),
			name,
		)
		if err != nil || !unconditional {
			return false, err
		}
	}
	indexes, err := db.Migrator().GetIndexes(&UserOAuthBinding{})
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
		expected := []string{"provider_id", "site_id", "provider_user_id"}
		for position, column := range columns {
			if !strings.EqualFold(column, expected[position]) {
				return false, nil
			}
		}
		return true, nil
	}
	return false, nil
}
