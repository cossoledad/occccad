package config

import (
	"net"
	"net/url"
	"os"
)

type Config struct {
	DatabaseURL   string
	ListenAddress string
	WorkerAddress string
	WebDirectory  string
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
	return Config{
		DatabaseURL:   databaseURL,
		ListenAddress: value("OCCCCAD_SERVER_LISTEN", "0.0.0.0:8080"),
		WorkerAddress: value("OCCCCAD_GEOMETRY_WORKER_ADDRESS", "127.0.0.1:51001"),
		WebDirectory:  value("OCCCCAD_WEB_DIR", "../web/apps/demo/dist"),
	}
}

func value(name, fallback string) string {
	if result := os.Getenv(name); result != "" {
		return result
	}
	return fallback
}
