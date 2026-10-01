package configs

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App      AppConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Kafka    KafkaConfig
	CORS     CORSConfig
}

type AppConfig struct {
	Name  string
	Env   string
	Port  string
	Debug bool
}

type DatabaseConfig struct {
	Mode        string
	URL         string
	MaxConns    int32
	AutoMigrate bool
	Seed        bool
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Issuer     string
}

type KafkaConfig struct {
	Enabled     bool
	Brokers     []string
	TopicPrefix string
}

type CORSConfig struct {
	AllowedOrigins []string
}

// Load builds the configuration from .env (if present) and process environment.
func Load() Config {
	loadDotEnv(".env")

	mode := strings.ToLower(getStr("DB_MODE", ""))
	if mode == "" {
		if getStr("DATABASE_URL", "") != "" {
			mode = "postgres"
		} else {
			mode = "memory"
		}
	}

	return Config{
		App: AppConfig{
			Name:  getStr("APP_NAME", "gin-api"),
			Env:   getStr("APP_ENV", "development"),
			Port:  getStr("APP_PORT", "8080"),
			Debug: getStr("APP_ENV", "development") != "production",
		},
		Database: DatabaseConfig{
			Mode:        mode,
			URL:         getStr("DATABASE_URL", ""),
			MaxConns:    getInt32("DB_MAX_CONNS", 10),
			AutoMigrate: getBool("DB_AUTO_MIGRATE", true),
			Seed:        getBool("DB_SEED", true),
		},
		JWT: JWTConfig{
			Secret:     getStr("JWT_SECRET", "dev-secret-change-me"),
			AccessTTL:  getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: getDuration("JWT_REFRESH_TTL", 168*time.Hour),
			Issuer:     getStr("JWT_ISSUER", "gin-api"),
		},
		Kafka: KafkaConfig{
			Enabled:     getBool("KAFKA_ENABLED", false),
			Brokers:     getList("KAFKA_BROKERS", []string{"localhost:9092"}),
			TopicPrefix: getStr("KAFKA_TOPIC_PREFIX", "dev"),
		},
		CORS: CORSConfig{
			AllowedOrigins: getList("CORS_ORIGINS", []string{"*"}),
		},
	}
}

func getStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getInt32(key string, def int32) int32 {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return def
	}
	return int32(n)
}

func getDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// loadDotEnv parses a simple KEY=VALUE file without overriding existing vars.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			val = strings.Trim(val, `"'`)
			if key == "" {
				continue
			}
			if _, exists := os.LookupEnv(key); !exists {
				os.Setenv(key, val)
			}
		}
	}
}
