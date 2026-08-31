package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
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
		return nil
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
	return nil
}

func prepareUserOAuthBindingsSiteScope(db *gorm.DB) error {
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
		}
		if tx.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex) {
			if err := tx.Migrator().DropIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex); err != nil {
				return fmt.Errorf("drop global custom OAuth binding index: %w", err)
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
	if db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex) {
		if err := db.Migrator().DropIndex(&UserOAuthBinding{}, legacyUserOAuthBindingGlobalSubjectIndex); err != nil {
			return fmt.Errorf("drop global custom OAuth binding index: %w", err)
		}
	}
	return nil
}

func userOAuthBindingSubjectIndexIsSiteScoped(db *gorm.DB, name string) (bool, error) {
	if db.Dialector.Name() == "sqlite" {
		var sql string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?",
			UserOAuthBinding{}.TableName(), name).Scan(&sql).Error; err != nil {
			return false, err
		}
		normalized := strings.ToLower(sql)
		columnsAt := strings.LastIndex(normalized, "(")
		if columnsAt < 0 || !strings.Contains(normalized, "create unique index") {
			return false, nil
		}
		parts := strings.Split(strings.TrimSpace(strings.TrimSuffix(normalized[columnsAt+1:], ")")), ",")
		if len(parts) != 3 {
			return false, nil
		}
		for i := range parts {
			parts[i] = strings.Trim(strings.TrimSpace(parts[i]), "`\"[]")
		}
		return parts[0] == "provider_id" && parts[1] == "site_id" && parts[2] == "provider_user_id", nil
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
		found := map[string]bool{}
		for _, column := range index.Columns() {
			found[strings.ToLower(column)] = true
		}
		return len(index.Columns()) == 3 && found["provider_id"] && found["site_id"] && found["provider_user_id"], nil
	}
	return false, nil
}
