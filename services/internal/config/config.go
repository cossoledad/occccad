package config

import (
	"bufio"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL    string
	ListenAddress  string
	WorkerAddress  string
	DataDirectory  string
	AllowedOrigins []string
	AdminEmail     string
	AdminName      string
	AdminPassword  string
	SessionTTL     time.Duration
	SecureCookies  bool
}

// LoadProjectEnv loads simple KEY=VALUE entries without overriding exported values.
// It searches the current directory and its parents so the control binary can be
// launched either from the repository root or from services/.
func LoadProjectEnv() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("OCCCCAD_ENV_FILE")); explicit != "" {
		return explicit, loadEnvFile(explicit)
	}
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(directory, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, loadEnvFile(candidate)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", nil
		}
		directory = parent
	}
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
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
		DatabaseURL:    databaseURL,
		ListenAddress:  value("OCCCCAD_SERVER_LISTEN", "0.0.0.0:8080"),
		WorkerAddress:  value("OCCCCAD_GEOMETRY_WORKER_ADDRESS", "127.0.0.1:51001"),
		DataDirectory:  value("OCCCCAD_DATA_DIR", "./data"),
		AllowedOrigins: splitList(os.Getenv("OCCCCAD_ALLOWED_ORIGINS")),
		AdminEmail:     value("OCCCCAD_ADMIN_EMAIL", "admin@occccad.local"),
		AdminName:      value("OCCCCAD_ADMIN_DISPLAY_NAME", "Administrator"),
		AdminPassword:  os.Getenv("OCCCCAD_ADMIN_PASSWORD"),
		SessionTTL:     sessionTTL,
		SecureCookies:  secureCookies,
	}
}

func splitList(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func value(name, fallback string) string {
	if result := os.Getenv(name); result != "" {
		return result
	}
	return fallback
}
