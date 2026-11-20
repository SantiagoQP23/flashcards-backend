// Package httpapi wires the Chi router, middleware and HTTP handlers.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/SantiagoQP23/flashcards-backend/internal/auth"
	"github.com/SantiagoQP23/flashcards-backend/internal/dictionary"
	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

// Pinger is the part of the database pool the health checks need.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Logger      *slog.Logger
	DB          Pinger
	Pool        *pgxpool.Pool
	CORSOrigins []string

	AccessTokens *auth.AccessTokens
	Google       auth.GoogleVerifier
	RefreshTTL   time.Duration
	// AuthRateLimit is requests per minute per IP on /v1/auth (0 disables).
	AuthRateLimit int
	// Dictionary answers word lookups that aren't cached yet. Nil serves
	// only cached words.
	Dictionary dictionary.Provider
}

type api struct {
	db         store.DB
	pool       *pgxpool.Pool
	access     *auth.AccessTokens
	google     auth.GoogleVerifier
	refreshTTL time.Duration
	now        func() time.Time
	dictionary dictionary.Provider
	lookups    singleflight.Group
}

func NewRouter(d Deps) http.Handler {
	a := &api{
		pool:       d.Pool,
		access:     d.AccessTokens,
		google:     d.Google,
		refreshTTL: d.RefreshTTL,
		now:        time.Now,
		dictionary: d.Dictionary,
	}
	if d.Pool != nil {
		a.db = d.Pool
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	if len(d.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: d.CORSOrigins,
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
			MaxAge:         300,
		}))
	}

	r.Get("/healthz", handleLiveness)
	r.Get("/readyz", handleReadiness(d.DB))

	r.Route("/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			if d.AuthRateLimit > 0 {
				r.Use(httprate.Limit(d.AuthRateLimit, time.Minute,
					httprate.WithKeyFuncs(httprate.KeyByIP),
					httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
						writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in a minute")
					})))
			}
			r.Post("/register", a.handleRegister)
			r.Post("/login", a.handleLogin)
			r.Post("/google", a.handleGoogle)
			r.Post("/refresh", a.handleRefresh)
			r.Post("/logout", a.handleLogout)
		})

		// Everything else needs a signed-in user.
		r.Group(func(r chi.Router) {
			r.Use(a.requireAuth)
			r.Get("/me", a.handleGetMe)
			r.Patch("/me", a.handleUpdateMe)
			r.Delete("/me", a.handleDeleteMe)

			r.Get("/decks", a.handleListDecks)
			r.Post("/decks", a.handleCreateDeck)
			r.Patch("/decks/{id}", a.handleRenameDeck)
			r.Delete("/decks/{id}", a.handleDeleteDeck)

			r.Get("/dictionary/words/{word}", a.handleLookupWord)
			r.Get("/dictionary/history", a.handleListHistory)
			r.Delete("/dictionary/history", a.handleClearHistory)

			r.Get("/vocabulary", a.handleListVocabulary)
			r.Post("/vocabulary", a.handleCreateVocabulary)
			r.Patch("/vocabulary/{id}", a.handleUpdateVocabulary)
			r.Delete("/vocabulary/{id}", a.handleDeleteVocabulary)
		})
	})

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return r
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
