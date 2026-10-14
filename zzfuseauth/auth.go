// Package zzfuseauth binds OTLP ingestion credentials to operator-configured tenants.
package zzfuseauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensionauth"
)

const ReceiptMetadataKey = "x-zzfuse-ingestion-receipt"

var (
	Type               = component.MustNewType("zzfuse_auth")
	ErrUnauthenticated = errors.New("valid tenant ingestion credentials are required")
	safeName           = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

type Credential struct {
	Token  configopaque.String `mapstructure:"token"`
	Schema string              `mapstructure:"schema"`
	Epoch  uint64              `mapstructure:"epoch"`
}

type Config struct {
	SigningKey  configopaque.String   `mapstructure:"signing_key"`
	Credentials map[string]Credential `mapstructure:"credentials"`
}

func (c *Config) Validate() error {
	if len(c.SigningKey) < 32 || strings.IndexFunc(string(c.SigningKey), unicode.IsSpace) >= 0 {
		return errors.New("signing_key requires at least 32 bytes without whitespace")
	}
	if len(c.Credentials) == 0 {
		return errors.New("at least one tenant ingestion credential is required")
	}
	tokens := make(map[[32]byte]bool)
	for id, credential := range c.Credentials {
		if !safeName.MatchString(id) {
			return errors.New("credential IDs must be lowercase SQL-style names of at most 63 characters")
		}
		if !safeName.MatchString(credential.Schema) || strings.HasPrefix(credential.Schema, "pg_") || credential.Schema == "public" || credential.Schema == "admin" || credential.Schema == "information_schema" {
			return fmt.Errorf("credential %s requires a tenant schema", id)
		}
		token := string(credential.Token)
		if token == string(c.SigningKey) {
			return errors.New("signing_key must be independent of ingestion tokens")
		}
		if len(token) < 32 || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
			return fmt.Errorf("credential %s requires a token of at least 32 bytes without whitespace", id)
		}
		if credential.Epoch == 0 {
			return fmt.Errorf("credential %s requires an epoch greater than zero", id)
		}
		hash := sha256.Sum256([]byte(token))
		if tokens[hash] {
			return errors.New("ingestion tokens must be unique across credentials")
		}
		tokens[hash] = true
	}
	return nil
}

// Tenant is authenticated routing identity, never derived from resource attributes.
type Tenant struct {
	CredentialID string `json:"credential_id"`
	Schema       string `json:"schema"`
	Epoch        uint64 `json:"epoch"`
}

// Resolver validates the signed, non-secret context receipt, including after WAL replay.
type Resolver interface {
	ResolveTenant(context.Context) (Tenant, error)
	ResolveIngressTenant(context.Context) (Tenant, error)
}

type authenticatedCredential struct {
	digest [32]byte
	tenant Tenant
}
type Authenticator struct {
	key         []byte
	credentials []authenticatedCredential
	tenants     map[string]Tenant
}

var _ extension.Extension = (*Authenticator)(nil)
var _ extensionauth.Server = (*Authenticator)(nil)
var _ Resolver = (*Authenticator)(nil)

func New(c *Config) (*Authenticator, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	a := &Authenticator{key: []byte(c.SigningKey), tenants: make(map[string]Tenant, len(c.Credentials))}
	for id, credential := range c.Credentials {
		tenant := Tenant{CredentialID: id, Schema: credential.Schema, Epoch: credential.Epoch}
		a.credentials = append(a.credentials, authenticatedCredential{sha256.Sum256([]byte(credential.Token)), tenant})
		a.tenants[id] = tenant
	}
	return a, nil
}

func NewFactory() extension.Factory {
	return extension.NewFactory(Type, func() component.Config { return &Config{Credentials: map[string]Credential{}} },
		func(_ context.Context, _ extension.Settings, c component.Config) (extension.Extension, error) {
			return New(c.(*Config))
		}, component.StabilityLevelBeta)
}

func (*Authenticator) Start(context.Context, component.Host) error { return nil }
func (*Authenticator) Shutdown(context.Context) error              { return nil }

func (a *Authenticator) Authenticate(ctx context.Context, sources map[string][]string) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	var values []string
	for name, supplied := range sources {
		if strings.EqualFold(name, "authorization") {
			values = append(values, supplied...)
		}
	}
	if len(values) != 1 {
		return ctx, ErrUnauthenticated
	}
	parts := strings.Split(values[0], " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return ctx, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(parts[1]))
	var matched Tenant
	for _, credential := range a.credentials {
		if subtle.ConstantTimeCompare(digest[:], credential.digest[:]) == 1 {
			matched = credential.tenant
		}
	}
	if matched.CredentialID == "" {
		return ctx, ErrUnauthenticated
	}
	receipt, err := a.sign(matched)
	if err != nil {
		return ctx, err
	}
	info := client.FromContext(ctx)
	info.Auth = authData{tenant: matched, authenticator: a}
	// Strip producer metadata, especially Authorization and any forged receipt.
	// The standard persistent exporter queue serializes client.Metadata, not Auth.
	info.Metadata = client.NewMetadata(map[string][]string{ReceiptMetadataKey: {receipt}})
	return client.NewContext(ctx, info), nil
}

func (a *Authenticator) sign(tenant Tenant) (string, error) {
	body, err := json.Marshal(tenant)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte("zzfuse-ingestion-v1:"))
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (a *Authenticator) ResolveTenant(ctx context.Context) (Tenant, error) {
	values := client.FromContext(ctx).Metadata.Get(ReceiptMetadataKey)
	if len(values) != 1 || len(values[0]) > 1024 {
		return Tenant{}, ErrUnauthenticated
	}
	parts := strings.Split(values[0], ".")
	if len(parts) != 2 {
		return Tenant{}, ErrUnauthenticated
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Tenant{}, ErrUnauthenticated
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Tenant{}, ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte("zzfuse-ingestion-v1:"))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), signature) {
		return Tenant{}, ErrUnauthenticated
	}
	var tenant Tenant
	if err := json.Unmarshal(body, &tenant); err != nil {
		return Tenant{}, ErrUnauthenticated
	}
	current, ok := a.tenants[tenant.CredentialID]
	if !ok || current != tenant {
		return Tenant{}, ErrUnauthenticated
	}
	return tenant, nil
}

// ResolveIngressTenant requires this authenticator's private authentication
// marker. A copied queue receipt is never a credential for fresh ingestion.
// ResolveTenant remains receipt-only for the Collector's persisted delivery.
func (a *Authenticator) ResolveIngressTenant(ctx context.Context) (Tenant, error) {
	if err := ctx.Err(); err != nil {
		return Tenant{}, err
	}
	identity, ok := client.FromContext(ctx).Auth.(authData)
	if !ok || identity.authenticator != a {
		return Tenant{}, ErrUnauthenticated
	}
	tenant, err := a.ResolveTenant(ctx)
	if err != nil || tenant != identity.tenant {
		return Tenant{}, ErrUnauthenticated
	}
	return tenant, nil
}

type authData struct {
	tenant        Tenant
	authenticator *Authenticator
}

func (a authData) GetAttribute(name string) any {
	switch name {
	case "schema":
		return a.tenant.Schema
	case "credential_id":
		return a.tenant.CredentialID
	case "epoch":
		return a.tenant.Epoch
	}
	return nil
}
func (authData) GetAttributeNames() []string { return []string{"schema", "credential_id", "epoch"} }
