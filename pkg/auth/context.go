// Package auth provides user context utilities for request-scoped authentication data.
package auth

import (
	"context"
	"errors"
	"strings"
)

// contextKey is a custom type to avoid collisions with other context values.
type contextKey string

const (
	// UserContextKey is the key used to store user context in Go context.
	UserContextKey contextKey = "user"
)

// UserContext represents the authenticated user's context information.
// This is derived from JWT claims and stored in the request context.
type UserContext struct {
	APIKeyID    string   `json:"api_key_id,omitempty"`
	AuthVersion int64    `json:"auth_version"`
	UserID      int64    `json:"user_id"`
	Email       string   `json:"email"`
	Username    string   `json:"username,omitempty"`
	TenantID    string   `json:"tenant_id,omitempty"`
	Schemas     []string `json:"schemas,omitempty"` // Empty denies schema access for non-admins.
	Roles       []string `json:"roles,omitempty"`
	OrgID       *int64   `json:"org_id,omitempty"`
	ProjectID   *int64   `json:"project_id,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

// GetUserContext extracts the user context from the Go context.
// Returns an error if no user context is found.
func GetUserContext(ctx context.Context) (*UserContext, error) {
	user, ok := ctx.Value(UserContextKey).(*UserContext)
	if !ok || user == nil {
		return nil, errors.New("user context not found")
	}
	return user, nil
}

// Deprecated: MustGetUserContext panics on missing context. Use GetUserContext instead,
// which returns an error that callers can handle gracefully.
func MustGetUserContext(ctx context.Context) *UserContext {
	user, err := GetUserContext(ctx)
	if err != nil {
		panic("user context not found: " + err.Error())
	}
	return user
}

// GetUserID extracts the user ID from the context.
func GetUserID(ctx context.Context) (int64, error) {
	user, err := GetUserContext(ctx)
	if err != nil {
		return 0, err
	}
	return user.UserID, nil
}

// GetUserSchemas extracts the user's accessible schemas from the context.
// Empty grants deny schema access unless the user is an administrator.
func GetUserSchemas(ctx context.Context) ([]string, error) {
	user, err := GetUserContext(ctx)
	if err != nil {
		return nil, err
	}
	return user.Schemas, nil
}

// GetUserRoles extracts the user's roles from the context.
func GetUserRoles(ctx context.Context) ([]string, error) {
	user, err := GetUserContext(ctx)
	if err != nil {
		return nil, err
	}
	return user.Roles, nil
}

// GetUserTenantID extracts the user's tenant ID from the context.
func GetUserTenantID(ctx context.Context) (string, error) {
	user, err := GetUserContext(ctx)
	if err != nil {
		return "", err
	}
	return user.TenantID, nil
}

// UserHasRole checks if the user has a specific role (from context).
func UserHasRole(ctx context.Context, role string) bool {
	user, err := GetUserContext(ctx)
	if err != nil {
		return false
	}
	return contains(user.Roles, role)
}

// UserHasScope checks if the user has a specific scope (from context).
func UserHasScope(ctx context.Context, scope string) bool {
	user, err := GetUserContext(ctx)
	if err != nil {
		return false
	}
	return contains(user.Scopes, scope)
}

// UserHasSchemaAccess checks if the user has access to a specific schema.
// Returns true if:
// - User has admin role
// - Schema is in user's schemas list
func UserHasSchemaAccess(ctx context.Context, schema string) bool {
	user, err := GetUserContext(ctx)
	if err != nil {
		return false
	}

	// Admin users have access to all schemas
	if contains(user.Roles, "admin") {
		return true
	}

	// Check if schema is in user's allowed schemas
	for _, allowed := range user.Schemas {
		if allowed == schema {
			return true
		}
	}
	return false
}

// WithUserContext adds user context to the Go context.
func WithUserContext(ctx context.Context, user *UserContext) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}

// ClaimsToUserContext converts JWT claims to UserContext.
func ClaimsToUserContext(claims *Claims) *UserContext {
	return &UserContext{
		APIKeyID:    claims.APIKeyID,
		AuthVersion: claims.AuthVersion,
		UserID:      claims.UserID,
		Email:       claims.Email,
		Username:    claims.Username,
		TenantID:    claims.TenantID,
		Schemas:     claims.Schemas,
		Roles:       claims.Roles,
		OrgID:       claims.OrgID,
		ProjectID:   claims.ProjectID,
		Scopes:      claims.Scopes,
	}
}

// contains checks if a slice contains a specific string (case-insensitive).
func contains(slice []string, item string) bool {
	itemLower := strings.ToLower(item)
	for _, s := range slice {
		if strings.ToLower(s) == itemLower {
			return true
		}
	}
	return false
}
