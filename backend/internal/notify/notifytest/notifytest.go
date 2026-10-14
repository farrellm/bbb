// Package notifytest is a fake Web Push service for tests. It stands in
// for every push host, makes subscriptions the way a browser does, and
// decrypts what the server sends to them (RFC 8291), so tests check the
// notification a phone would actually show.
package notifytest

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/farrellm/bbb/internal/notify"
)

// Service is a running fake push service.
type Service struct {
	t   *testing.T
	srv *httptest.Server

	mu     sync.Mutex
	subs   map[string]*ecdh.PrivateKey // endpoint path → device key
	auth   map[string][]byte
	status map[string][]int // endpoint path → statuses to answer, in turn
	got    []Push
}

// Push is one notification as the device received it.
type Push struct {
	Endpoint string
	Topic    string
	TTL      string
	Payload  notify.Payload
}

// New starts a service that stops when the test ends.
func New(t *testing.T) *Service {
	t.Helper()
	s := &Service{t: t, subs: map[string]*ecdh.PrivateKey{}, auth: map[string][]byte{}, status: map[string][]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.receive))
	t.Cleanup(s.srv.Close)
	return s
}

// Client sends every request to the fake service, whatever its host, so
// subscriptions can carry real push-service endpoints.
func (s *Service) Client() *http.Client {
	target, _ := url.Parse(s.srv.URL)
	return &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Subscription is what PushSubscription.toJSON() gives a browser.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Subscribe makes a new device subscription on Apple's push host.
func (s *Service) Subscribe() Subscription {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		s.t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	path := "/3/device/" + rand.Text()
	s.mu.Lock()
	s.subs[path], s.auth[path] = key, auth
	s.mu.Unlock()

	var sub Subscription
	sub.Endpoint = "https://web.push.apple.com" + path
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	sub.Keys.Auth = base64.RawURLEncoding.EncodeToString(auth)
	return sub
}

// Answer makes the service reply to the next pushes for endpoint with
// these statuses, one each, before going back to 201.
func (s *Service) Answer(endpoint string, statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := pathOf(endpoint)
	s.status[p] = append(s.status[p], statuses...)
}

// Pushes returns what has been received so far and forgets it.
func (s *Service) Pushes() []Push {
	s.mu.Lock()
	defer s.mu.Unlock()
	got := s.got
	s.got = nil
	return got
}

func pathOf(endpoint string) string {
	u, _ := url.Parse(endpoint)
	return u.Path
}

func (s *Service) receive(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	key, ok := s.subs[r.URL.Path]
	auth := s.auth[r.URL.Path]
	var status int
	if q := s.status[r.URL.Path]; len(q) > 0 {
		status, s.status[r.URL.Path] = q[0], q[1:]
	}
	s.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusGone)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "vapid t=") ||
		r.Header.Get("Content-Encoding") != "aes128gcm" {
		s.t.Errorf("push without VAPID auth or aes128gcm: %v", r.Header)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	body, _ := io.ReadAll(r.Body)
	plain, err := decrypt(key, auth, body)
	if err != nil {
		s.t.Errorf("decrypt push: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var p notify.Payload
	if err := json.Unmarshal(plain, &p); err != nil {
		s.t.Errorf("push payload %q: %v", plain, err)
	}
	s.mu.Lock()
	s.got = append(s.got, Push{
		Endpoint: "https://web.push.apple.com" + r.URL.Path,
		Topic:    r.Header.Get("Topic"), TTL: r.Header.Get("TTL"), Payload: p,
	})
	s.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

// decrypt opens an aes128gcm message (RFC 8188) encrypted for the device
// key with the RFC 8291 key schedule. The whole message is one record.
func decrypt(key *ecdh.PrivateKey, authSecret, msg []byte) ([]byte, error) {
	if len(msg) < 21 {
		return nil, errors.New("short message")
	}
	// Header: salt (16), record size (4), key id length (1), key id.
	salt, idLen := msg[:16], int(msg[20])
	if len(msg) < 21+idLen {
		return nil, errors.New("short header")
	}
	senderPub, ciphertext := msg[21:21+idLen], msg[21+idLen:]

	pub, err := ecdh.P256().NewPublicKey(senderPub)
	if err != nil {
		return nil, err
	}
	shared, err := key.ECDH(pub)
	if err != nil {
		return nil, err
	}
	info := append(append([]byte("WebPush: info\x00"), key.PublicKey().Bytes()...), senderPub...)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, string(info), 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	// The last record ends with a 2 delimiter, then zero padding.
	plain = bytes.TrimRight(plain, "\x00")
	if len(plain) == 0 || plain[len(plain)-1] != 2 {
		return nil, errors.New("bad padding delimiter")
	}
	return plain[:len(plain)-1], nil
}
