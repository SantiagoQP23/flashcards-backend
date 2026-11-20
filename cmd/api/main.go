// Command api runs the Intenli Flashcards HTTP API.
//
// Usage:
//
//	api            start the HTTP server (runs pending migrations first when AUTO_MIGRATE is on)
//	api migrate    apply pending migrations and exit
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	// Time zone names are validated and used for "today", and the
	// distroless image has no zoneinfo of its own guaranteed.
	_ "time/tzdata"

	"github.com/SantiagoQP23/flashcards-backend/internal/auth"
	"github.com/SantiagoQP23/flashcards-backend/internal/config"
	"github.com/SantiagoQP23/flashcards-backend/internal/database"
	"github.com/SantiagoQP23/flashcards-backend/internal/dictionary"
	"github.com/SantiagoQP23/flashcards-backend/internal/httpapi"
	"github.com/SantiagoQP23/flashcards-backend/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) > 0 && args[0] == "migrate" {
		return migrations.Up(ctx, cfg.DatabaseURL)
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q", args[0])
	}

	if cfg.AutoMigrate {
		if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger:        logger,
			DB:            pool,
			Pool:          pool,
			CORSOrigins:   cfg.CORSOrigins,
			AccessTokens:  auth.NewAccessTokens(cfg.JWTSecret, cfg.AccessTokenTTL),
			Google:        auth.NewGoogleVerifier(cfg.GoogleClientIDs),
			RefreshTTL:    cfg.RefreshTokenTTL,
			AuthRateLimit: cfg.AuthRateLimit,
			Dictionary:    newDictionary(cfg),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.Env == config.EnvDevelopment {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// newDictionary is Free Dictionary, with Merriam-Webster covering its
// outages when MW_LEARNERS_API_KEY is set.
func newDictionary(cfg config.Config) dictionary.Provider {
	primary := dictionary.NewFreeDictionary(cfg.DictionaryTimeout)
	if mw := dictionary.NewMerriamWebster(cfg.MerriamWebsterKey, cfg.DictionaryTimeout); mw != nil {
		return dictionary.WithFallback(primary, mw)
	}
	return primary
}
