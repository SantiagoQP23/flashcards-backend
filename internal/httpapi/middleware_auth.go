package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type ctxKey int

const ctxUserID ctxKey = iota

// requireAuth verifies the Bearer access token and stores the user ID.
func (a *api) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing access token")
			return
		}
		userID, err := a.access.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_token", "access token is invalid or expired")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUserID, userID)))
	})
}

// userIDFrom returns the signed-in user. Only valid behind requireAuth.
func userIDFrom(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxUserID).(uuid.UUID)
	return id
}
