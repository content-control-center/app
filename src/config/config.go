package config

import "github.com/kelseyhightower/envconfig"

type Config struct {
	Addr              string `envconfig:"ADDR"                default:":9001"`
	DSN               string `envconfig:"DATABASE_DSN"        default:"file:data/app.db?cache=shared&_pragma=journal_mode(WAL)"`
	Debug             bool   `envconfig:"DEBUG"               default:"false"`
	SessionCookieName string `envconfig:"SESSION_COOKIE_NAME" default:"c3_session"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
