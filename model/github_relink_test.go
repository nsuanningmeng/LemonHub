package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGitHubRelinkFailureRestoresLegacyBindingAndClaim(t *testing.T) {
	truncateTables(t)
	owner := User{Username: "relink-owner", Password: "password", AffCode: "relink-owner", GitHubId: "old-login", SiteId: 7}
	other := User{Username: "numeric-owner", Password: "password", AffCode: "numeric-owner", GitHubId: "987654", SiteId: 7}
	require.NoError(t, DB.Create(&owner).Error)
	require.NoError(t, DB.Create(&other).Error)
	require.NoError(t, UpdateUserBindColumn(owner.Id, "github_id", owner.GitHubId))
	require.NoError(t, UpdateUserBindColumn(other.Id, "github_id", other.GitHubId))
	rollbackErr := errors.New("abort after identity rewrite")

	for _, test := range []struct {
		name    string
		subject string
		after   error
		wantErr error
	}{
		{name: "claimed numeric ID", subject: other.GitHubId, wantErr: ErrExternalIdentityAlreadyClaimed},
		{name: "transaction rollback after rewrite", subject: "123456", after: rollbackErr, wantErr: rollbackErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := DB.Transaction(func(tx *gorm.DB) error {
				if err := BindUserExternalIdentityWithTx(tx, owner.Id, "github_id", test.subject); err != nil {
					return err
				}
				return test.after
			})
			assert.ErrorIs(t, err, test.wantErr)
			var unchanged User
			require.NoError(t, DB.First(&unchanged, owner.Id).Error)
			assert.Equal(t, "old-login", unchanged.GitHubId)
			var claim ExternalIdentityClaim
			require.NoError(t, DB.Where("provider = ? AND user_id = ?", ExternalIdentityProviderGitHub, owner.Id).First(&claim).Error)
			assert.Equal(t, "old-login", claim.Subject)
			assert.Equal(t, 7, claim.SiteId)
			var claims int64
			require.NoError(t, DB.Model(&ExternalIdentityClaim{}).Count(&claims).Error)
			assert.EqualValues(t, 2, claims)
		})
	}
}

func TestCompetingGitHubRelinksPreserveLosingLegacyBinding(t *testing.T) {
	truncateTables(t)
	owners := []User{
		{Username: "first-relink", Password: "password", AffCode: "first-relink", GitHubId: "first-old-login", SiteId: 7},
		{Username: "second-relink", Password: "password", AffCode: "second-relink", GitHubId: "second-old-login", SiteId: 7},
	}
	for index := range owners {
		require.NoError(t, DB.Create(&owners[index]).Error)
		require.NoError(t, UpdateUserBindColumn(owners[index].Id, "github_id", owners[index].GitHubId))
	}
	start := make(chan struct{})
	results := make(chan error, len(owners))
	var ready sync.WaitGroup
	ready.Add(len(owners))
	for _, owner := range owners {
		go func(id int) {
			ready.Done()
			<-start
			results <- UpdateUserBindColumn(id, "github_id", "987654")
		}(owner.Id)
	}
	ready.Wait()
	close(start)
	successes, conflicts := 0, 0
	for range owners {
		err := <-results
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)
			conflicts++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	var winner ExternalIdentityClaim
	require.NoError(t, DB.Where("provider = ? AND site_id = ? AND subject = ?", ExternalIdentityProviderGitHub, 7, "987654").First(&winner).Error)
	for _, owner := range owners {
		wantSubject := owner.GitHubId
		if owner.Id == winner.UserId {
			wantSubject = "987654"
		}
		var actual User
		require.NoError(t, DB.First(&actual, owner.Id).Error)
		assert.Equal(t, wantSubject, actual.GitHubId)
		var claim ExternalIdentityClaim
		require.NoError(t, DB.Where("provider = ? AND user_id = ?", ExternalIdentityProviderGitHub, owner.Id).First(&claim).Error)
		assert.Equal(t, wantSubject, claim.Subject)
		assert.Equal(t, 7, claim.SiteId)
	}
}
