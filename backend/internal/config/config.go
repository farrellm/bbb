// Package config reads every setting from the environment, with local-dev
// defaults so `make backend` runs with no environment at all.
package config

import (
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
}

// Load reads the environment.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: env("DATABASE_URL", "postgres://bbb:bbb@localhost:5435/bbb?sslmode=disable"),
		BBBURL:      env("BBB_URL", bbb.DefaultURL),
		SignupHosts: bbb.DefaultSignupHosts,
	}

	// The User-Agent says who is polling and how to reach them.
	cfg.UserAgent = "BBBWatch/1.0"
	if contact := os.Getenv("CONTACT_EMAIL"); contact != "" {
		cfg.UserAgent += " (+" + contact + ")"
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
