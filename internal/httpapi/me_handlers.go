package httpapi

import (
	"errors"
	"net/http"

	"github.com/SantiagoQP23/flashcards-backend/internal/store"
)

const maxDailyNewLimit = 500

// GET /v1/me
func (a *api) handleGetMe(w http.ResponseWriter, r *http.Request) {
	user, err := store.GetUserByID(r.Context(), a.db, userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid_token", "user no longer exists")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserDTO(user))
}

// PATCH /v1/me changes profile and study preferences. A field left out
// stays as it is; an empty display_name clears it.
func (a *api) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName     *string `json:"display_name"`
		Timezone        *string `json:"timezone"`
		Locale          *string `json:"locale"`
		PreferredAccent *string `json:"preferred_accent"`
		DailyNewLimit   *int    `json:"daily_new_limit"`
	}
	err := decodeJSON(w, r, &req)
	if err == nil && req.DisplayName == nil && req.Timezone == nil && req.Locale == nil &&
		req.PreferredAccent == nil && req.DailyNewLimit == nil {
		err = badRequest("at least one field is required")
	}
	if err == nil && req.DisplayName != nil {
		var name string
		name, err = cleanName(*req.DisplayName, "display_name", maxDisplayNameLength)
		req.DisplayName = &name
	}
	if err == nil && req.Timezone != nil {
		if *req.Timezone == "" {
			err = badRequest("timezone must be an IANA name like America/Guayaquil")
		} else {
			var tz string
			tz, err = cleanTimezone(*req.Timezone)
			req.Timezone = &tz
		}
	}
	if err == nil && req.Locale != nil && !validLocale(*req.Locale) {
		err = badRequest("locale must be es or en")
	}
	if err == nil && req.PreferredAccent != nil && *req.PreferredAccent != "US" && *req.PreferredAccent != "UK" {
		err = badRequest("preferred_accent must be US or UK")
	}
	if err == nil && req.DailyNewLimit != nil && (*req.DailyNewLimit < 0 || *req.DailyNewLimit > maxDailyNewLimit) {
		err = badRequest("daily_new_limit must be 0 to %d", maxDailyNewLimit)
	}
	if writeBadRequest(w, err) {
		return
	}

	user, err := store.UpdateUser(r.Context(), a.db, userIDFrom(r.Context()), store.UserUpdate{
		DisplayName:     req.DisplayName,
		Timezone:        req.Timezone,
		Locale:          req.Locale,
		PreferredAccent: req.PreferredAccent,
		DailyNewLimit:   req.DailyNewLimit,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid_token", "user no longer exists")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserDTO(user))
}

// DELETE /v1/me deletes the account and everything in it, and signs out
// every device (refresh tokens go with the user). Access tokens already
// issued fail from then on because the user is gone.
func (a *api) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	err := store.DeleteUser(r.Context(), a.db, userIDFrom(r.Context()))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
