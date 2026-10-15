package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL, Origin, ClientID, ClientSecret, Port string
	Domains                                           []string
	DestructiveEnabled                                bool
}

func Load() (Config, error) {
	if flag := os.Getenv("ENABLE_DESTRUCTIVE_DEMO"); flag != "" && flag != "false" && flag != "true" {
		return Config{}, errors.New("ENABLE_DESTRUCTIVE_DEMO must be true or false")
	}
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), Origin: strings.TrimRight(os.Getenv("APP_ORIGIN"), "/"), ClientID: os.Getenv("GOOGLE_CLIENT_ID"), ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"), Port: os.Getenv("PORT"), DestructiveEnabled: os.Getenv("ENABLE_DESTRUCTIVE_DEMO") == "true"}
	for _, d := range strings.Split(os.Getenv("GOOGLE_WORKSPACE_DOMAINS"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			c.Domains = append(c.Domains, strings.ToLower(d))
		}
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("PORT must be between 1 and 65535")
	}
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("APP_ORIGIN must be an HTTPS origin")
	}
	if c.DatabaseURL == "" || c.ClientID == "" || c.ClientSecret == "" || len(c.Domains) == 0 {
		return errors.New("database and Google Workspace configuration required")
	}
	if os.Getenv("MIGRATION_DATABASE_URL") != "" {
		return errors.New("migration credentials must not be present in the server environment")
	}
	return nil
}
