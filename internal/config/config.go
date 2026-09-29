package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
	JWTSecret   []byte
	Port        string
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
		Port:        os.Getenv("PORT"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 bytes")
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	return c, nil
}
