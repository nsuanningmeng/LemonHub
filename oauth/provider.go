package oauth

import (
	"context"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Provider defines the interface for OAuth providers
type Provider interface {
	// GetName returns the display name of the provider (e.g., "GitHub", "Discord")
	GetName() string

	// IsEnabled returns whether this OAuth provider is enabled
	IsEnabled() bool

	// ExchangeToken exchanges the authorization code for an access token
	// The gin.Context is passed for providers that need request info (e.g., for redirect_uri)
	ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error)

	// GetUserInfo retrieves user information using the access token
	GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error)

	// IsUserIDTaken checks if the provider user ID is already associated with an account
	// on the given sub-site (siteId, 0 = main site). Identity is isolated per site.
	IsUserIDTaken(providerUserID string, siteId int) bool

	// FillUserByProviderID fills the user model by provider user ID, scoped to siteId.
	FillUserByProviderID(user *model.User, providerUserID string, siteId int) error

	// SetProviderUserID sets the provider user ID on the user model
	SetProviderUserID(user *model.User, providerUserID string)

	// GetProviderPrefix returns the prefix for auto-generated usernames (e.g., "github_")
	GetProviderPrefix() string
}
