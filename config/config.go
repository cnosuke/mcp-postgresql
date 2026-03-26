package config

import (
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
