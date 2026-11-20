package config

import (
	"log/slog"
	"slices"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_ENV", "")
	t.Setenv("AUTO_MIGRATE", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != EnvDevelopment || cfg.HTTPAddr != ":8080" || !cfg.AutoMigrate || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !slices.Equal(cfg.CORSOrigins, []string{"http://localhost:8081"}) {
		t.Fatalf("unexpected CORS origins: %v", cfg.CORSOrigins)
	}
}

func TestLoadProductionDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "a-production-secret-of-32-characters!")
	t.Setenv("AUTO_MIGRATE", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoMigrate {
		t.Fatal("AUTO_MIGRATE should default to false in production")
	}
	if len(cfg.CORSOrigins) != 0 {
		t.Fatalf("production should allow no browser origins by default, got %v", cfg.CORSOrigins)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_ENV", "staging-ish")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unknown APP_ENV")
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	for key, value := range map[string]string{"AUTO_MIGRATE": "sometimes", "LOG_LEVEL": "loud"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			t.Setenv("APP_ENV", "")
			t.Setenv(key, value)
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %s=%s", key, value)
			}
		})
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a, ,b ,")
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("splitList = %v", got)
	}
}

func TestLoadAuthDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_ENV", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("ACCESS_TOKEN_TTL", "")
	t.Setenv("REFRESH_TOKEN_TTL", "")
	t.Setenv("GOOGLE_CLIENT_IDS", "")
	t.Setenv("AUTH_RATE_LIMIT", "")
	t.Setenv("DICTIONARY_TIMEOUT", "")
	t.Setenv("MW_LEARNERS_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.JWTSecret) != devJWTSecret || cfg.AccessTokenTTL != 15*time.Minute ||
		cfg.RefreshTokenTTL != 720*time.Hour || cfg.GoogleClientIDs != nil || cfg.AuthRateLimit != 20 ||
		cfg.DictionaryTimeout != 3*time.Second || cfg.MerriamWebsterKey != "" {
		t.Fatalf("unexpected auth defaults: %+v", cfg)
	}
}

func TestLoadRequiresJWTSecretInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when JWT_SECRET is missing in production")
	}
	t.Setenv("JWT_SECRET", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when JWT_SECRET is too short")
	}
}
