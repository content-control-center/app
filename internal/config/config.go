package config

import "github.com/kelseyhightower/envconfig"

type Config struct {
	Addr  string `envconfig:"ADDR"         default:":3000"`
	DSN   string `envconfig:"DATABASE_DSN" default:"file:data.db?cache=shared&_pragma=journal_mode(WAL)"`
	Debug bool   `envconfig:"DEBUG"        default:"false"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
