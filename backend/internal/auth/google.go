package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// GoogleProvider is Google's OpenID configuration. It is fixed here rather
// than discovered, so the server starts without reaching Google; the
// signing keys are still fetched (and cached) on first use.
var GoogleProvider = oidc.ProviderConfig{ //nolint:gosec // G101: endpoints, not credentials
	IssuerURL:  "https://accounts.google.com",
	AuthURL:    "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:   "https://oauth2.googleapis.com/token",
	JWKSURL:    "https://www.googleapis.com/oauth2/v3/certs",
	Algorithms: []string{oidc.RS256},
}

// Identity is a verified Google account.
type Identity struct {
	Subject string
	Email   string
	Name    string
	Picture string
}

// ErrEmailUnverified means Google has not verified the account's email.
var ErrEmailUnverified = errors.New("email not verified")

// Google runs the authorization code flow with PKCE against an OpenID
// provider: Google in production, a fake one in tests.
type Google struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewGoogle builds a client for the provider. redirectURL is the absolute
// callback URL registered with the OAuth client.
func NewGoogle(ctx context.Context, p oidc.ProviderConfig, clientID, clientSecret, redirectURL string) *Google {
	provider := p.NewProvider(ctx)
	return &Google{
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}
}

// AuthCodeURL is where to send the browser to sign in. selectAccount makes
// Google show its account chooser even when one account is signed in.
func (g *Google) AuthCodeURL(fl Flow, selectAccount bool) string {
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(fl.Verifier), oidc.Nonce(fl.Nonce)}
	if selectAccount {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	return g.oauth.AuthCodeURL(fl.State, opts...)
}

// Exchange trades the authorization code for tokens and verifies the ID
// token: signature, issuer, audience, expiry, the flow's nonce, and that
// the email is verified.
func (g *Google) Exchange(ctx context.Context, code string, fl Flow) (Identity, error) {
	tok, err := g.oauth.Exchange(ctx, code, oauth2.VerifierOption(fl.Verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange code: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return Identity{}, errors.New("no id_token in token response")
	}
	idt, err := g.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("verify id token: %w", err)
	}
	if idt.Nonce != fl.Nonce {
		return Identity{}, errors.New("id token nonce mismatch")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("id token claims: %w", err)
	}
	if claims.Email == "" || !claims.EmailVerified {
		return Identity{}, ErrEmailUnverified
	}
	return Identity{
		Subject: idt.Subject,
		Email:   claims.Email,
		Name:    claims.Name,
		Picture: claims.Picture,
	}, nil
}
