package model

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Setup struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(50);not null"`
	InitializedAt int64  `json:"initialized_at" gorm:"type:bigint;not null"`
}

var ErrSetupAlreadyInitialized = errors.New("system is already initialized")

type setupStorageError struct{ cause error }

func (e *setupStorageError) Error() string { return "failed to access setup storage" }
func (e *setupStorageError) Unwrap() error { return e.cause }

func GetSetup() (*Setup, error) { return getSetupWithDB(DB) }

func getSetupWithDB(db *gorm.DB) (*Setup, error) {
	var setup Setup
	err := db.First(&setup).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, &setupStorageError{cause: err}
	}
	return &setup, nil
}

func rootUserExistsWithDB(db *gorm.DB) (bool, error) {
	// Preserve the legacy definition: an undeleted root in any site, regardless
	// of status. A disabled root must not reopen anonymous initialization.
	var user User
	err := db.Where("role = ?", common.RoleRootUser).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, &setupStorageError{cause: err}
	}
	return true, nil
}

// InitializeSetup commits the root account, mode options and setup marker
// together. The existing marker primary key arbitrates empty-database races
// across processes; locking a nonexistent row alone would not do so.
func InitializeSetup(username, passwordHash string, selfUse, demo bool) error {
	optionWriterMu.Lock()
	defer optionWriterMu.Unlock()
	return initializeSetup(username, passwordHash, selfUse, demo)
}

// initializeSetup performs the complete setup transaction and publication.
// The public entry point serializes local option writers; the database claim
// inside this primitive also arbitrates independent application processes.
func initializeSetup(username, passwordHash string, selfUse, demo bool) error {
	values := []Option{{Key: "SelfUseModeEnabled", Value: strconv.FormatBool(selfUse)}, {Key: "DemoSiteEnabled", Value: strconv.FormatBool(demo)}}
	err := DB.Transaction(func(tx *gorm.DB) error {
		setup, err := getSetupWithDB(tx)
		if err != nil {
			return err
		}
		if setup != nil {
			return ErrSetupAlreadyInitialized
		}
		rootExists, err := rootUserExistsWithDB(tx)
		if err != nil {
			return err
		}
		if rootExists {
			return ErrSetupAlreadyInitialized
		}
		// A losing concurrent insert fails the entire transaction. Never ignore
		// this conflict and continue to create another root or overwrite modes.
		marker := Setup{ID: 1, Version: common.Version, InitializedAt: time.Now().Unix()}
		if err := tx.Create(&marker).Error; err != nil {
			return err
		}
		root := User{Username: username, Password: passwordHash, Role: common.RoleRootUser, Status: common.UserStatusEnabled, DisplayName: "Root User", Quota: 100000000}
		if err := tx.Create(&root).Error; err != nil {
			return err
		}
		for _, value := range values {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&value).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrSetupAlreadyInitialized) {
			return ErrSetupAlreadyInitialized
		}
		return &setupStorageError{cause: err}
	}
	// Both values are canonical bool strings and their setters cannot fail.
	for _, value := range values {
		if err := updateOptionMap(value.Key, value.Value); err != nil {
			return err
		}
	}
	constant.Setup = true
	return nil
}
