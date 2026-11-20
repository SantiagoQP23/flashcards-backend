package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"DATABASE_URL": "postgres://localhost:5432/db"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "development" || cfg.Port != 3000 || cfg.DatabaseURL != "postgres://localhost:5432/db" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{"DATABASE_URL": "postgres://x", "PORT": "8080", "APP_ENV": "production"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 || cfg.Env != "production" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := load(env(map[string]string{"PORT": "abc", "APP_ENV": "staging"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL", "PORT", "APP_ENV"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
