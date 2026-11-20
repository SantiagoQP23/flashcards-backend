package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SantiagoQP23/flashcards-backend/internal/auth"
	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

const maxDisplayNameLength = 100

// issueSession creates a refresh token in family (a new family when
// familyID is uuid.Nil) plus an access token.
func (a *api) issueSession(ctx context.Context, db store.DB, user store.User, familyID uuid.UUID) (sessionDTO, error) {
	if familyID == uuid.Nil {
		familyID = uuid.New()
	}
	refresh, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return sessionDTO{}, err
	}
	if err := store.CreateRefreshToken(ctx, db, user.ID, familyID, hash, a.now().Add(a.refreshTTL)); err != nil {
		return sessionDTO{}, err
	}
	access, err := a.access.Issue(user.ID)
	if err != nil {
		return sessionDTO{}, err
	}
	return sessionDTO{
		User:         toUserDTO(user),
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(a.access.TTL() / time.Second),
	}, nil
}

func validatePassword(pw string) error {
	n := len([]rune(pw))
	if n < auth.MinPasswordLength || n > auth.MaxPasswordLength {
		return badRequestCode("invalid_password", "password must be %d to %d characters", auth.MinPasswordLength, auth.MaxPasswordLength)
	}
	return nil
}

// POST /v1/auth/register
func (a *api) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Timezone    string `json:"timezone"`
	}
	err := decodeJSON(w, r, &req)
	if err == nil {
		req.Email, err = normalizeEmail(req.Email)
	}
	if err == nil {
		err = validatePassword(req.Password)
	}
	if err == nil {
		req.DisplayName, err = cleanName(req.DisplayName, "display_name", maxDisplayNameLength)
	}
	if err == nil {
		req.Timezone, err = cleanTimezone(req.Timezone)
	}
	if writeBadRequest(w, err) {
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(w, r, err)
		return
	}

	var session sessionDTO
	err = store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		user, err := store.CreateUser(r.Context(), tx, store.NewUser{
			Email:        req.Email,
			PasswordHash: hash,
			DisplayName:  req.DisplayName,
			Timezone:     req.Timezone,
			Locale:       requestLocale(r),
		})
		if err != nil {
			return err
		}
		if err := store.CreateStarterDeck(r.Context(), tx, user.ID, starterDeckName[user.Locale]); err != nil {
			return err
		}
		session, err = a.issueSession(r.Context(), tx, user, uuid.Nil)
		return err
	})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "email_taken", "an account with this email already exists")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

// POST /v1/auth/login
func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if writeBadRequest(w, decodeJSON(w, r, &req)) {
		return
	}
	invalid := func() {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
	}

	email, err := normalizeEmail(req.Email)
	if err != nil {
		auth.CheckDummyPassword(req.Password)
		invalid()
		return
	}
	user, err := store.GetUserByEmail(r.Context(), a.db, email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, r, err)
		return
	}
	// Unknown emails and Google-only accounts take as long as a wrong
	// password, so the response doesn't reveal which accounts exist.
	if errors.Is(err, store.ErrNotFound) || user.PasswordHash == "" {
		auth.CheckDummyPassword(req.Password)
		invalid()
		return
	}
	ok, err := auth.CheckPassword(req.Password, user.PasswordHash)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !ok {
		invalid()
		return
	}
	session, err := a.issueSession(r.Context(), a.db, user, uuid.Nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// POST /v1/auth/google
//
// The app signs in with Google and sends the ID token. We find the user by
// Google account, else link it to the account with the same email, else
// create a new account.
func (a *api) handleGoogle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDToken  string `json:"id_token"`
		Timezone string `json:"timezone"`
	}
	err := decodeJSON(w, r, &req)
	if err == nil && req.IDToken == "" {
		err = badRequest("id_token is required")
	}
	if err == nil {
		req.Timezone, err = cleanTimezone(req.Timezone)
	}
	if writeBadRequest(w, err) {
		return
	}
	identity, err := a.google.Verify(r.Context(), req.IDToken)
	switch {
	case errors.Is(err, auth.ErrGoogleNotConfigured):
		writeError(w, http.StatusNotImplemented, "google_not_configured", "Google sign-in is not configured on this server")
		return
	case err != nil:
		writeError(w, http.StatusUnauthorized, "invalid_google_token", "Google token is invalid or expired")
		return
	case !identity.EmailVerified:
		writeError(w, http.StatusUnauthorized, "email_not_verified", "your Google email address is not verified")
		return
	}
	email, err := normalizeEmail(identity.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_google_token", "Google token has an unusable email")
		return
	}
	name, _ := cleanName(identity.Name, "name", maxDisplayNameLength)

	var session sessionDTO
	err = store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		ctx := r.Context()
		user, err := store.GetUserByGoogleSub(ctx, tx, identity.Subject)
		switch {
		case err == nil:
			// Returning Google user.
		case errors.Is(err, store.ErrNotFound):
			user, err = store.GetUserByEmailForUpdate(ctx, tx, email)
			switch {
			case err == nil:
				// Link Google to the account with this email. Its email was
				// never proven if it signed up with a password, so whoever
				// set that password may not own the address: drop the
				// password and sign out its sessions.
				dropPassword := !user.EmailVerified && user.PasswordHash != ""
				if user, err = store.LinkGoogle(ctx, tx, user.ID, identity.Subject, dropPassword); err != nil {
					return err
				}
				if dropPassword {
					if err := store.RevokeAllRefreshTokens(ctx, tx, user.ID); err != nil {
						return err
					}
				}
			case errors.Is(err, store.ErrNotFound):
				user, err = store.CreateUser(ctx, tx, store.NewUser{
					Email:         email,
					EmailVerified: true,
					GoogleSub:     identity.Subject,
					DisplayName:   name,
					Timezone:      req.Timezone,
					Locale:        requestLocale(r),
				})
				if err != nil {
					return err
				}
				if err := store.CreateStarterDeck(ctx, tx, user.ID, starterDeckName[user.Locale]); err != nil {
					return err
				}
			default:
				return err
			}
		default:
			return err
		}
		session, err = a.issueSession(ctx, tx, user, uuid.Nil)
		return err
	})
	if errors.Is(err, store.ErrConflict) {
		// Two first sign-ins raced; the other one won. Retrying succeeds.
		writeError(w, http.StatusConflict, "retry", "please try again")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

var errRefreshRejected = errors.New("refresh rejected")

// refreshReuseGrace is how long a just-rotated refresh token may be used
// again (a retry after a lost response) without revoking its family.
const refreshReuseGrace = 60 * time.Second

// POST /v1/auth/refresh rotates the refresh token. Reusing an already
// rotated token revokes every token in its family (likely theft), except
// within refreshReuseGrace of its rotation.
func (a *api) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if writeBadRequest(w, decodeJSON(w, r, &req)) {
		return
	}

	var session sessionDTO
	reused := false
	err := store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		ctx := r.Context()
		tok, err := store.GetRefreshTokenForUpdate(ctx, tx, auth.HashOpaqueToken(req.RefreshToken))
		if errors.Is(err, store.ErrNotFound) {
			return errRefreshRejected
		}
		if err != nil {
			return err
		}
		if tok.RevokedAt != nil {
			// A phone on a bad network can lose the response to a refresh
			// and retry with the old token. Within the grace window, while
			// the family is still signed in, answer with a new session in
			// the same family instead of treating it as theft.
			if tok.RotatedAt != nil && a.now().Sub(*tok.RotatedAt) < refreshReuseGrace {
				live, err := store.FamilyHasLiveToken(ctx, tx, tok.FamilyID, a.now())
				if err != nil {
					return err
				}
				if live {
					user, err := store.GetUserByID(ctx, tx, tok.UserID)
					if err != nil {
						return err
					}
					session, err = a.issueSession(ctx, tx, user, tok.FamilyID)
					return err
				}
			}
			reused = true
			return store.RevokeRefreshFamily(ctx, tx, tok.FamilyID)
		}
		if !a.now().Before(tok.ExpiresAt) {
			return errRefreshRejected
		}
		if err := store.RotateRefreshToken(ctx, tx, tok.ID, a.now()); err != nil {
			return err
		}
		user, err := store.GetUserByID(ctx, tx, tok.UserID)
		if err != nil {
			return err
		}
		session, err = a.issueSession(ctx, tx, user, tok.FamilyID)
		return err
	})
	if err == nil && reused {
		err = errRefreshRejected
	}
	if errors.Is(err, errRefreshRejected) {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// POST /v1/auth/logout revokes the sign-in the refresh token belongs to.
// Unknown tokens succeed too, so signing out never fails on the phone.
func (a *api) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if writeBadRequest(w, decodeJSON(w, r, &req)) {
		return
	}
	err := store.InTx(r.Context(), a.pool, func(tx pgx.Tx) error {
		tok, err := store.GetRefreshTokenForUpdate(r.Context(), tx, auth.HashOpaqueToken(req.RefreshToken))
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return store.RevokeRefreshFamily(r.Context(), tx, tok.FamilyID)
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
