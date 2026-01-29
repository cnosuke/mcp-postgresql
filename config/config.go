package config

import (
	"github.com/jinzhu/configor"
)

// Config - Application configuration
type Config struct {
	Log        string `yaml:"log" default:"" env:"LOG_PATH"`
	Debug      bool   `yaml:"debug" default:"false" env:"DEBUG"`
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
