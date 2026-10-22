package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func sessionIDSeenBy(t *testing.T, auth *authService, cookieToken, headerToken string) (string, *httptest.ResponseRecorder) {
	t.Helper()
	var seen string
	handler := auth.requireSession(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = sessionIDFromContext(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/api/session/state", nil)
	if cookieToken != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieToken})
	}
	if headerToken != "" {
		req.Header.Set(sessionTokenHeader, headerToken)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return seen, rec
}

// Two tabs share one cookie jar: the second tab's login overwrites the cookie,
// but each tab must stay on the session named by its own header token.
func TestHeaderTokenKeepsTabsOnTheirOwnSession(t *testing.T) {
	auth := &authService{sessions: newSessionManager([]byte("test-secret"))}
	tabA, err := auth.sessions.create()
	if err != nil {
		t.Fatal(err)
	}
	tabB, err := auth.sessions.create()
	if err != nil {
		t.Fatal(err)
	}
	claimsA, _ := auth.sessions.claimsFromToken(tabA)
	claimsB, _ := auth.sessions.claimsFromToken(tabB)

	// cookie now belongs to tab B; tab A still sends its own header token
	if got, _ := sessionIDSeenBy(t, auth, tabB, tabA); got != claimsA.SessionID {
		t.Fatalf("tab A resolved to session %q, want its own %q", got, claimsA.SessionID)
	}
	if got, _ := sessionIDSeenBy(t, auth, tabB, tabB); got != claimsB.SessionID {
		t.Fatalf("tab B resolved to session %q, want %q", got, claimsB.SessionID)
	}
	// old cached client without the header still works via the cookie
	if got, _ := sessionIDSeenBy(t, auth, tabB, ""); got != claimsB.SessionID {
		t.Fatalf("cookie fallback resolved to %q, want %q", got, claimsB.SessionID)
	}
}

func TestInvalidHeaderTokenIsRejectedWithoutCookieFallback(t *testing.T) {
	auth := &authService{sessions: newSessionManager([]byte("test-secret"))}
	valid, err := auth.sessions.create()
	if err != nil {
		t.Fatal(err)
	}

	got, rec := sessionIDSeenBy(t, auth, valid, "forged.token")
	if rec.Code != http.StatusUnauthorized || got != "" {
		t.Fatalf("status=%d session=%q, want 401 and no session", rec.Code, got)
	}
	if setCookie := rec.Header().Get("Set-Cookie"); setCookie != "" {
		t.Fatalf("bad header token cleared the shared cookie: %q", setCookie)
	}
}
