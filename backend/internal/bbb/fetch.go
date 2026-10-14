package bbb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultURL is the Big Blue Bulletin page.
const DefaultURL = "https://www.pingry.org/experience/community-life/bbb"

// maxBodyBytes bounds a fetch; the page is about 134 KB.
const maxBodyBytes = 8 << 20

// Fetcher downloads the page.
type Fetcher struct {
	Client    *http.Client
	URL       string
	UserAgent string
}

// NewFetcher returns a Fetcher with a 20 s timeout.
func NewFetcher(url, userAgent string) *Fetcher {
	return &Fetcher{
		Client:    &http.Client{Timeout: 20 * time.Second},
		URL:       url,
		UserAgent: userAgent,
	}
}

// Fetched is one download of the page.
type Fetched struct {
	Status int
	Body   []byte
	Hash   string // hex sha256 of Body
}

// StatusError is returned for a non-200 response.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return fmt.Sprintf("bbb fetch: HTTP %d", e.Status) }

// Fetch downloads the page. A non-200 response returns the status in
// Fetched along with a *StatusError.
func (f *Fetcher) Fetch(ctx context.Context) (Fetched, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return Fetched{}, fmt.Errorf("bbb fetch: %w", err)
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "text/html")

	resp, err := f.Client.Do(req)
	if err != nil {
		return Fetched{}, fmt.Errorf("bbb fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Fetched{Status: resp.StatusCode}, &StatusError{resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Fetched{Status: resp.StatusCode}, fmt.Errorf("bbb fetch: read body: %w", err)
	}
	return Fetched{Status: resp.StatusCode, Body: body, Hash: BodyHash(body)}, nil
}

// BodyHash is the hex sha256 of a raw page body.
func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
