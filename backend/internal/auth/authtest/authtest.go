// Package authtest is a fake OpenID provider for tests: it signs ID tokens
// with its own key, checks PKCE on the token endpoint, and lets a test play
// the user's part on the consent page.
package authtest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
)

// ClientID is the OAuth client the fake provider issues tokens for.
const ClientID = "test-client"

// Account is who signs in on the fake consent page.
type Account struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Provider is a running fake provider.
type Provider struct {
	srv    *httptest.Server
	signer jose.Signer
	jwks   jose.JSONWebKeySet

	mu     sync.Mutex
	grants map[string]grant
}

type grant struct {
	claims    map[string]any
	challenge string
}

// New starts a provider that stops when the test ends.
func New(t *testing.T) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       jose.JSONWebKey{Key: key, KeyID: "k1", Algorithm: string(jose.RS256)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{
		signer: signer,
		jwks: jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: string(jose.RS256), Use: "sig"},
		}},
		grants: map[string]grant{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(p.jwks)
	})
	mux.HandleFunc("POST /token", p.token)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// Config is the provider's OpenID configuration.
func (p *Provider) Config() oidc.ProviderConfig {
	return oidc.ProviderConfig{
		IssuerURL:  p.srv.URL,
		AuthURL:    p.srv.URL + "/authorize",
		TokenURL:   p.srv.URL + "/token",
		JWKSURL:    p.srv.URL + "/jwks",
		Algorithms: []string{oidc.RS256},
	}
}

// Consent plays the user signing in as acct on the page at authURL (where
// the app redirected the browser). It returns the URL the provider would
// redirect back to: the app's callback with a code and the state.
func (p *Provider) Consent(t *testing.T, authURL string, acct Account) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != ClientID || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("bad authorize request: %s", authURL)
	}
	code := rand.Text()
	p.mu.Lock()
	p.grants[code] = grant{
		challenge: q.Get("code_challenge"),
		claims: map[string]any{
			"iss":            p.srv.URL,
			"aud":            ClientID,
			"sub":            acct.Subject,
			"email":          acct.Email,
			"email_verified": acct.EmailVerified,
			"name":           acct.Name,
			"nonce":          q.Get("nonce"),
			"iat":            time.Now().Unix(),
			"exp":            time.Now().Add(time.Hour).Unix(),
		},
	}
	p.mu.Unlock()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	bq := back.Query()
	bq.Set("code", code)
	bq.Set("state", q.Get("state"))
	back.RawQuery = bq.Encode()
	return back.String()
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	g, ok := p.grants[r.PostForm.Get("code")]
	delete(p.grants, r.PostForm.Get("code"))
	p.mu.Unlock()

	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	payload, _ := json.Marshal(g.claims)
	sig, err := p.signer.Sign(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idToken, _ := sig.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": idToken,
	})
}
