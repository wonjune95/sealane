package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig is read from OIDC_* environment variables (the Helm chart injects them).
type OIDCConfig struct {
	IssuerURL     string
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	GroupsClaim   string
	UsernameClaim string
	Roles         map[string][]string // app role → IdP groups that receive it
	DefaultRole   string              // "" = users without a matching group are refused
	DisplayName   string
}

// OIDCConfigFromEnv returns nil, nil when OIDC_ISSUER_URL is unset (OIDC disabled).
func OIDCConfigFromEnv() (*OIDCConfig, error) {
	issuer := os.Getenv("OIDC_ISSUER_URL")
	if issuer == "" {
		return nil, nil
	}
	c := &OIDCConfig{
		IssuerURL:     issuer,
		ClientID:      os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret:  os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:   os.Getenv("OIDC_REDIRECT_URL"),
		Scopes:        splitList(envOr("OIDC_SCOPES", "openid profile email groups"), " "),
		GroupsClaim:   envOr("OIDC_GROUPS_CLAIM", "groups"),
		UsernameClaim: envOr("OIDC_USERNAME_CLAIM", "email"),
		Roles: map[string][]string{
			"admin":  splitList(os.Getenv("OIDC_ROLES_ADMIN"), ","),
			"editor": splitList(os.Getenv("OIDC_ROLES_EDITOR"), ","),
			"viewer": splitList(os.Getenv("OIDC_ROLES_VIEWER"), ","),
		},
		DefaultRole: os.Getenv("OIDC_DEFAULT_ROLE"),
		DisplayName: envOr("OIDC_DISPLAY_NAME", "SSO"),
	}
	if c.ClientID == "" || c.ClientSecret == "" || c.RedirectURL == "" {
		return nil, errors.New("OIDC_CLIENT_ID, OIDC_CLIENT_SECRET and OIDC_REDIRECT_URL are required when OIDC_ISSUER_URL is set")
	}
	if c.DefaultRole != "" && !ValidRole(c.DefaultRole) {
		return nil, fmt.Errorf("OIDC_DEFAULT_ROLE %q is not one of admin, editor, viewer", c.DefaultRole)
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitList(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ErrNoRole means the IdP authenticated the user but no group maps to a Sealane role.
var ErrNoRole = errors.New("account has no role in Sealane")

// OIDC runs the authorization-code + PKCE login against one provider. Discovery is lazy
// so an unreachable IdP never blocks the boot or local logins.
type OIDC struct {
	cfg      OIDCConfig
	mu       sync.Mutex
	provider *oidc.Provider
}

func NewOIDC(cfg OIDCConfig) *OIDC { return &OIDC{cfg: cfg} }

// oidcHTTP bounds every IdP call: discovery, token exchange and (via the provider) JWKS.
var oidcHTTP = &http.Client{Timeout: 10 * time.Second}

func (o *OIDC) DisplayName() string { return o.cfg.DisplayName }

func (o *OIDC) discover(ctx context.Context) (*oidc.Provider, error) {
	// ponytail: one mutex serializes first discovery; fine at login rates.
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return o.provider, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, oidcHTTP), o.cfg.IssuerURL) // the provider keeps this client for JWKS
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	o.provider = p
	return p, nil
}

func (o *OIDC) oauth(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID: o.cfg.ClientID, ClientSecret: o.cfg.ClientSecret, RedirectURL: o.cfg.RedirectURL,
		Endpoint: p.Endpoint(), Scopes: o.cfg.Scopes,
	}
}

// AuthCodeURL is the IdP redirect; verifier is the PKCE secret the caller keeps in the state cookie.
func (o *OIDC) AuthCodeURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	p, err := o.discover(ctx)
	if err != nil {
		return "", err
	}
	return o.oauth(p).AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// Exchange redeems the callback code, verifies the id_token (signature, audience, expiry,
// nonce) and maps the user's groups to a Sealane role.
func (o *OIDC) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	p, err := o.discover(ctx)
	if err != nil {
		return Identity{}, err
	}
	ctx = oidc.ClientContext(ctx, oidcHTTP)
	tok, err := o.oauth(p).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("token exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Identity{}, errors.New("token response has no id_token")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: o.cfg.ClientID}).Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("id_token: %w", err)
	}
	if idt.Nonce != nonce {
		return Identity{}, errors.New("id_token nonce mismatch")
	}
	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, err
	}
	name, _ := claims[o.cfg.UsernameClaim].(string)
	if name == "" {
		name = idt.Subject
	}
	var groups []string
	if gs, ok := claims[o.cfg.GroupsClaim].([]any); ok {
		for _, g := range gs {
			if s, ok := g.(string); ok {
				groups = append(groups, s)
			}
		}
	}
	role, ok := MapRole(groups, o.cfg.Roles, o.cfg.DefaultRole)
	if !ok {
		return Identity{}, ErrNoRole
	}
	return Identity{Name: name, Role: role, Provider: ProviderOIDC}, nil
}
