package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jinzhu/configor"
)

// PresetConfig defines a named connection preset
type PresetConfig struct {
	Host         string `yaml:"host"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	Port         int    `yaml:"port" default:"5432"`
	Database     string `yaml:"database"`
	Schema       string `yaml:"schema" default:"public"`
	SSLMode      string `yaml:"sslmode" default:"disable"`
	DSN          string `yaml:"dsn"`
	ReadOnly     bool   `yaml:"read_only" default:"false"`
	QueryTimeout int    `yaml:"query_timeout" default:"0"` // 0 means fall back to global postgresql.query_timeout
}

type GoogleOAuthConfig struct {
	ClientID       string   `yaml:"client_id" default:"" env:"GOOGLE_CLIENT_ID"`
	ClientSecret   string   `yaml:"client_secret" default:"" env:"GOOGLE_CLIENT_SECRET"`
	AllowedDomains []string `yaml:"allowed_domains" env:"GOOGLE_ALLOWED_DOMAINS"`
	AllowedEmails  []string `yaml:"allowed_emails" env:"GOOGLE_ALLOWED_EMAILS"`
}

type PreregisteredClient struct {
	ClientID     string   `yaml:"client_id"`
	ClientName   string   `yaml:"client_name"`
	RedirectURIs []string `yaml:"redirect_uris"`
}

type OAuthConfig struct {
	Enabled    bool                  `yaml:"enabled" default:"false" env:"OAUTH_ENABLED"`
	Issuer     string                `yaml:"issuer" default:"" env:"OAUTH_ISSUER"`
	SigningKey  string               `yaml:"signing_key" default:"" env:"OAUTH_SIGNING_KEY"`
	TokenExpiry int                  `yaml:"token_expiry" default:"3600" env:"OAUTH_TOKEN_EXPIRY"`
	Google     GoogleOAuthConfig     `yaml:"google"`
	Clients    []PreregisteredClient `yaml:"clients"`
}

func (c *OAuthConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Issuer == "" {
		return errors.New("oauth: issuer is required")
	}
	u, err := url.Parse(c.Issuer)
	if err != nil {
		return fmt.Errorf("oauth: invalid issuer URL: %w", err)
	}
	if u.Scheme != "https" {
		return errors.New("oauth: issuer must use https scheme")
	}
	if u.Host == "" {
		return errors.New("oauth: issuer must have a host")
	}
	if u.Path != "" && u.Path != "/" {
		return errors.New("oauth: issuer must not have a path component")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("oauth: issuer must not have query or fragment")
	}
	if len(c.SigningKey) < 32 {
		return fmt.Errorf("oauth: signing_key must be at least 32 bytes (got %d)", len(c.SigningKey))
	}
	if c.Google.ClientID == "" {
		return errors.New("oauth: google.client_id is required")
	}
	if c.Google.ClientSecret == "" {
		return errors.New("oauth: google.client_secret is required")
	}
	return nil
}

func (c *OAuthConfig) NormalizedIssuer() string {
	return strings.TrimRight(c.Issuer, "/")
}

// Config - Application configuration
type Config struct {
	Log        string `yaml:"log" default:"" env:"LOG_PATH"`
	LogLevel   string `yaml:"log_level" default:"info" env:"LOG_LEVEL"`
	PostgreSQL struct {
		Host         string `yaml:"host" default:"localhost" env:"POSTGRES_HOST"`
		User         string `yaml:"user" default:"postgres" env:"POSTGRES_USER"`
		Password     string `yaml:"password" default:"" env:"POSTGRES_PASSWORD"`
		Port         int    `yaml:"port" default:"5432" env:"POSTGRES_PORT"`
		Database     string `yaml:"database" default:"postgres" env:"POSTGRES_DATABASE"`
		Schema       string `yaml:"schema" default:"public" env:"POSTGRES_SCHEMA"`
		SSLMode      string `yaml:"sslmode" default:"disable" env:"POSTGRES_SSLMODE"`
		DSN          string `yaml:"dsn" default:"" env:"POSTGRES_DSN"`
		ReadOnly     bool   `yaml:"read_only" default:"false" env:"POSTGRES_READ_ONLY"`
		QueryTimeout int    `yaml:"query_timeout" default:"30" env:"POSTGRES_QUERY_TIMEOUT"`
	} `yaml:"postgresql"`
	HTTP struct {
		Host           string   `yaml:"host" default:"127.0.0.1" env:"HTTP_HOST"`
		Port           int      `yaml:"port" default:"8080" env:"HTTP_PORT"`
		Endpoint       string   `yaml:"endpoint" default:"/mcp" env:"HTTP_ENDPOINT"`
		AuthToken      string   `yaml:"auth_token" default:"" env:"HTTP_AUTH_TOKEN"`
		AllowedOrigins []string `yaml:"allowed_origins" env:"HTTP_ALLOWED_ORIGINS"`
	} `yaml:"http"`
	OAuth   OAuthConfig             `yaml:"oauth"`
	Presets map[string]PresetConfig `yaml:"presets"`
}

// LoadConfig - Load configuration file
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{}
	err := configor.New(&configor.Config{
		Debug:      false,
		Verbose:    false,
		Silent:     true,
		AutoReload: false,
	}).Load(cfg, path)
	return cfg, err
}
