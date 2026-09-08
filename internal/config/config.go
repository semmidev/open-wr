package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds global server settings.
type ServerConfig struct {
	Listen        string `yaml:"listen"`
	Origin        string `yaml:"origin"`
	CookieSecret  string `yaml:"cookie_secret"`
	SecureCookies bool   `yaml:"secure_cookies"` // enforce Secure flag on cookies (true in production)
	RedisAddr     string `yaml:"redis_addr"`
	RedisEnabled  bool   `yaml:"redis_enabled"`
	LogLevel      string `yaml:"log_level"`     // debug | info | warn | error
	AdminAPIKey   string `yaml:"admin_api_key"` // protects /api/rooms endpoints; empty = no auth (dev only)
	CORSOrigin    string `yaml:"cors_origin"`   // allowed origin for status CORS; "*" by default if empty
}

// RoomConfig holds per-room waiting room settings.
type RoomConfig struct {
	ID                     string `yaml:"id"`
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	Path                   string `yaml:"path"`
	Host                   string `yaml:"host"`
	Enabled                bool   `yaml:"enabled"`
	QueueAll               bool   `yaml:"queue_all"`
	NewUsersPerMinute      int64  `yaml:"new_users_per_minute"`
	TotalActiveUsers       int64  `yaml:"total_active_users"`
	SessionDurationMinutes int    `yaml:"session_duration_minutes"`
	QueueingMethod         string `yaml:"queueing_method"` // fifo | random
	CookieName             string `yaml:"cookie_name"`
	CustomPageTemplate     string `yaml:"custom_page_template"`
	DisableSessionRenewal  bool   `yaml:"disable_session_renewal"`
	JSONResponseEnabled    bool   `yaml:"json_response_enabled"`
}

// Config is the top-level config struct.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Rooms  []RoomConfig `yaml:"rooms"`
}

const minSecretLen = 32

// Load reads the YAML config at path, applies defaults, overrides from env, then validates.
func Load(path string) (*Config, error) {
	f, err := os.Open(path) // #nosec G304 -- config path is explicitly provided by user flag
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var c Config
	if err := yaml.NewDecoder(f).Decode(&c); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	applyDefaults(&c)
	overrideFromEnv(&c)

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &c, nil
}

// applyDefaults sets sensible defaults for unspecified fields.
func applyDefaults(c *Config) {
	if c.Server.Listen == "" {
		c.Server.Listen = ":8080"
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = "info"
	}
	if c.Server.CORSOrigin == "" {
		c.Server.CORSOrigin = "*"
	}
	// NOTE: intentionally no default CookieSecret — Validate() will reject empty.

	for i := range c.Rooms {
		r := &c.Rooms[i]
		if r.CookieName == "" {
			r.CookieName = "__owr"
		}
		if r.QueueingMethod == "" {
			r.QueueingMethod = "fifo"
		}
		if r.NewUsersPerMinute == 0 {
			r.NewUsersPerMinute = 60
		}
		if r.TotalActiveUsers == 0 {
			r.TotalActiveUsers = 200
		}
		if r.SessionDurationMinutes == 0 {
			r.SessionDurationMinutes = 10
		}
	}
}

// overrideFromEnv applies environment variable overrides for 12-factor compliance.
// Env vars take precedence over YAML values.
func overrideFromEnv(c *Config) {
	if v := os.Getenv("OPENWR_LISTEN"); v != "" {
		c.Server.Listen = v
	}
	if v := os.Getenv("OPENWR_ORIGIN"); v != "" {
		c.Server.Origin = v
	}
	if v := os.Getenv("OPENWR_COOKIE_SECRET"); v != "" {
		c.Server.CookieSecret = v
	}
	if v := os.Getenv("OPENWR_REDIS_ADDR"); v != "" {
		c.Server.RedisAddr = v
	}
	if v := os.Getenv("OPENWR_REDIS_ENABLED"); v != "" {
		c.Server.RedisEnabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("OPENWR_LOG_LEVEL"); v != "" {
		c.Server.LogLevel = v
	}
	if v := os.Getenv("OPENWR_ADMIN_API_KEY"); v != "" {
		c.Server.AdminAPIKey = v
	}
	if v := os.Getenv("OPENWR_CORS_ORIGIN"); v != "" {
		c.Server.CORSOrigin = v
	}
	if v := os.Getenv("OPENWR_SECURE_COOKIES"); v != "" {
		c.Server.SecureCookies, _ = strconv.ParseBool(v)
	}
}

// Validate checks for configuration correctness.
func (c *Config) Validate() error {
	var errs []string

	if len(c.Server.CookieSecret) < minSecretLen {
		errs = append(errs, fmt.Sprintf(
			"server.cookie_secret must be at least %d characters (got %d); set OPENWR_COOKIE_SECRET env var",
			minSecretLen, len(c.Server.CookieSecret),
		))
	}

	if c.Server.RedisEnabled && c.Server.RedisAddr == "" {
		errs = append(errs, "server.redis_addr is required when redis_enabled is true")
	}

	ids := make(map[string]struct{}, len(c.Rooms))
	for i, r := range c.Rooms {
		prefix := fmt.Sprintf("rooms[%d] (id=%q)", i, r.ID)

		if r.ID == "" {
			errs = append(errs, prefix+": id is required")
		} else if _, dup := ids[r.ID]; dup {
			errs = append(errs, fmt.Sprintf("rooms: duplicate id %q", r.ID))
		} else {
			ids[r.ID] = struct{}{}
		}

		if r.Path == "" {
			errs = append(errs, prefix+": path is required")
		}

		if r.NewUsersPerMinute <= 0 {
			errs = append(errs, prefix+": new_users_per_minute must be > 0")
		}
		if r.TotalActiveUsers <= 0 {
			errs = append(errs, prefix+": total_active_users must be > 0")
		}
		if r.SessionDurationMinutes <= 0 {
			errs = append(errs, prefix+": session_duration_minutes must be > 0")
		}

		switch r.QueueingMethod {
		case "fifo", "random":
		default:
			errs = append(errs, prefix+": queueing_method must be 'fifo' or 'random'")
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
