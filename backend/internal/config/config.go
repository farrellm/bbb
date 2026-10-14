// Package config reads every setting from the environment, with local-dev
// defaults so `make backend` runs with no environment at all.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/farrellm/bbb/internal/bbb"
)

// Config holds every environment-derived setting.
type Config struct {
	DatabaseURL string
	// BBBURL is the page to poll; point it at a local fixture server to test.
	BBBURL    string
	UserAgent string
	// PollInterval is the time between polls. Cloudflare caches the page
	// for 300 s, so polling faster gains nothing.
	PollInterval time.Duration
	// NightPollInterval, when non-zero, replaces PollInterval from 11pm to
	// 6am school time.
	NightPollInterval time.Duration
	// SchoolTZ is the school's time zone, used for deadline years and the
	// overnight back-off.
	SchoolTZ    *time.Location
	SignupHosts []string

	// ListenAddr is where the HTTP server binds. The default is for
	// development; the deployed unit binds 127.0.0.1:8093, reached only
	// through Tailscale Funnel, so the two can run side by side.
	ListenAddr string
	// PublicURL is the origin users reach the app at, with no trailing
	// slash. The OAuth redirect URI and invite links are built from it, and
	// an https URL turns on Secure cookies. Locally it is the Vite dev
	// server, which proxies /api here, so cookies stay same-origin.
	PublicURL string
	// WebDir is the built frontend to serve at "/". Empty serves only the
	// API, since in development Vite serves the app.
	WebDir string
	// SessionSecret signs the session and sign-in cookies.
	SessionSecret      string
	GoogleClientID     string
	GoogleClientSecret string
	// AdminEmails become admins when they sign in, and can create their
	// account without an invite. Lowercased.
	AdminEmails []string

	// VAPID keys sign Web Push requests (generate with -gen-vapid). Without
	// them push is off. VAPIDSubject is how push services can reach the
	// sender: a mailto: or https: URL.
	VAPIDPublic  string
	VAPIDPrivate string
	VAPIDSubject string
}

// devSessionSecret is the local-dev default. Load refuses it when PublicURL
// is https, so a deploy can't run with it by accident.
const devSessionSecret = "bbb-local-development-session-secret" //nolint:gosec // G101: dev-only, refused over https

// SecureCookies reports whether cookies need the Secure flag.
func (c Config) SecureCookies() bool {
	return strings.HasPrefix(c.PublicURL, "https://")
}

// PushConfigured reports whether Web Push has keys.
func (c Config) PushConfigured() bool {
	return c.VAPIDPublic != "" && c.VAPIDPrivate != ""
}

// GoogleConfigured reports whether Google sign-in has credentials.
func (c Config) GoogleConfigured() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

// Load reads the environment.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: env("DATABASE_URL", "postgres://bbb:bbb@localhost:5435/bbb?sslmode=disable"),
		BBBURL:      env("BBB_URL", bbb.DefaultURL),
		SignupHosts: bbb.DefaultSignupHosts,

		ListenAddr:         env("LISTEN_ADDR", "127.0.0.1:8094"),
		PublicURL:          strings.TrimRight(env("PUBLIC_URL", "http://localhost:5173"), "/"),
		WebDir:             os.Getenv("WEB_DIR"),
		SessionSecret:      env("SESSION_SECRET", devSessionSecret),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	}
	if cfg.SecureCookies() && cfg.SessionSecret == devSessionSecret {
		return cfg, errors.New("SESSION_SECRET must be set when PUBLIC_URL is https")
	}
	if len(cfg.SessionSecret) < 32 {
		return cfg, errors.New("SESSION_SECRET must be at least 32 characters")
	}
	for e := range strings.SplitSeq(os.Getenv("ADMIN_EMAILS"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			cfg.AdminEmails = append(cfg.AdminEmails, e)
		}
	}

	// The User-Agent says who is polling and how to reach them.
	cfg.UserAgent = "BBBWatch/1.0"
	contact := os.Getenv("CONTACT_EMAIL")
	if contact != "" {
		cfg.UserAgent += " (+" + contact + ")"
	}

	cfg.VAPIDPublic = os.Getenv("VAPID_PUBLIC")
	cfg.VAPIDPrivate = os.Getenv("VAPID_PRIVATE")
	if (cfg.VAPIDPublic == "") != (cfg.VAPIDPrivate == "") {
		return cfg, errors.New("set both VAPID_PUBLIC and VAPID_PRIVATE, or neither")
	}
	cfg.VAPIDSubject = os.Getenv("VAPID_SUBJECT")
	if cfg.VAPIDSubject == "" && contact != "" {
		cfg.VAPIDSubject = "mailto:" + contact
	}
	if cfg.VAPIDSubject == "" {
		cfg.VAPIDSubject = cfg.PublicURL
	}
	// Apple refuses pushes whose subject isn't one of these.
	if cfg.PushConfigured() && !strings.HasPrefix(cfg.VAPIDSubject, "mailto:") &&
		!strings.HasPrefix(cfg.VAPIDSubject, "https://") {
		return cfg, errors.New("VAPID_SUBJECT (or CONTACT_EMAIL) must give a mailto: or https: URL")
	}

	var err error
	if cfg.PollInterval, err = duration("POLL_INTERVAL", 5*time.Minute); err != nil {
		return cfg, err
	}
	if cfg.NightPollInterval, err = duration("NIGHT_POLL_INTERVAL", 0); err != nil {
		return cfg, err
	}
	if cfg.SchoolTZ, err = time.LoadLocation(env("SCHOOL_TZ", "America/New_York")); err != nil {
		return cfg, fmt.Errorf("SCHOOL_TZ: %w", err)
	}

	// SIGNUP_HOSTS_EXTRA adds comma-separated patterns to the built-in list.
	for h := range strings.SplitSeq(os.Getenv("SIGNUP_HOSTS_EXTRA"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			cfg.SignupHosts = append(cfg.SignupHosts[:len(cfg.SignupHosts):len(cfg.SignupHosts)], h)
		}
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func duration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}
