package config

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL   string
	ListenAddress string
	WorkerAddress string
	WebDirectory  string
	DataDirectory string
	AdminEmail    string
	AdminName     string
	AdminPassword string
	SessionTTL    time.Duration
	SecureCookies bool
}

func Load() Config {
	databaseURL := os.Getenv("OCCCCAD_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = (&url.URL{
			Scheme: "postgres",
			User: url.UserPassword(
				value("OCCCCAD_POSTGRES_USER", "occccad"),
				os.Getenv("OCCCCAD_POSTGRES_PASSWORD"),
			),
			Host: net.JoinHostPort(
				value("OCCCCAD_POSTGRES_HOST", "127.0.0.1"),
				value("OCCCCAD_POSTGRES_PORT", "5432"),
			),
			Path:     "/" + value("OCCCCAD_POSTGRES_DB", "occccad"),
			RawQuery: "search_path=occccad%2Cpublic",
		}).String()
	}
	sessionTTL, err := time.ParseDuration(value("OCCCCAD_SESSION_DURATION", "12h"))
	if err != nil || sessionTTL <= 0 {
		sessionTTL = 12 * time.Hour
	}
	secureCookies, _ := strconv.ParseBool(value("OCCCCAD_SECURE_COOKIES", "false"))
	return Config{
		DatabaseURL:   databaseURL,
		ListenAddress: value("OCCCCAD_SERVER_LISTEN", "0.0.0.0:8080"),
		WorkerAddress: value("OCCCCAD_GEOMETRY_WORKER_ADDRESS", "127.0.0.1:51001"),
		WebDirectory:  value("OCCCCAD_WEB_DIR", "../web/apps/cad/dist"),
		DataDirectory: value("OCCCCAD_DATA_DIR", "./data"),
		AdminEmail:    value("OCCCCAD_ADMIN_EMAIL", "admin@occccad.local"),
		AdminName:     value("OCCCCAD_ADMIN_DISPLAY_NAME", "Administrator"),
		AdminPassword: os.Getenv("OCCCCAD_ADMIN_PASSWORD"),
		SessionTTL:    sessionTTL,
		SecureCookies: secureCookies,
	}
}

func value(name, fallback string) string {
	if result := os.Getenv(name); result != "" {
		return result
	}
	return fallback
}
