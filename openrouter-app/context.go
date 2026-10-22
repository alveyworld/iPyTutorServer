package main

import (
	"context"
	"net/http"
)

type contextKey string

const sessionIDContextKey contextKey = "sessionID"

func sessionIDFromContext(ctx context.Context) (string, bool) {
	sessionID, ok := ctx.Value(sessionIDContextKey).(string)
	return sessionID, ok && sessionID != ""
}

func (a *authService) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, fromHeader, err := a.sessions.tokenFromRequest(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := a.sessions.claimsFromToken(token)
		if err != nil {
			if !fromHeader {
				// the cookie may belong to another tab's valid session
				a.sessions.clearCookie(w, r)
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sessionIDContextKey, claims.SessionID))
		next(w, r)
	}
}
