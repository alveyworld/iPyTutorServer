package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	sessionCookieName  = "ipyintervu_session"
	sessionTokenHeader = "X-Session-Token"
)

type sessionClaims struct {
	ExpiresAt int64  `json:"exp"`
	SessionID string `json:"sid"`
}

type sessionManager struct {
	secret []byte
	ttl    time.Duration
}

func newSessionManager(secret []byte) *sessionManager {
	return &sessionManager{
		secret: secret,
		ttl:    24 * time.Hour,
	}
}

func sessionSecretFromEnv(privateKeyPEM []byte) ([]byte, error) {
	if secret := os.Getenv("AUTH_SESSION_SECRET"); secret != "" {
		return []byte(secret), nil
	}

	sum := sha256.Sum256(privateKeyPEM)
	return sum[:], nil
}

func (m *sessionManager) create() (string, error) {
	sessionID := make([]byte, 16)
	if _, err := rand.Read(sessionID); err != nil {
		return "", err
	}

	claims := sessionClaims{
		ExpiresAt: time.Now().Add(m.ttl).Unix(),
		SessionID: base64.RawURLEncoding.EncodeToString(sessionID),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	payloadEncoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payloadEncoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadEncoded + "." + signature, nil
}

func (m *sessionManager) claimsFromToken(token string) (sessionClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return sessionClaims{}, errors.New("invalid session token")
	}

	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return sessionClaims{}, errors.New("invalid session signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionClaims{}, fmt.Errorf("decode session payload: %w", err)
	}

	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return sessionClaims{}, fmt.Errorf("parse session payload: %w", err)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return sessionClaims{}, errors.New("session expired")
	}

	return claims, nil
}

func (m *sessionManager) validate(token string) error {
	_, err := m.claimsFromToken(token)
	return err
}

func (m *sessionManager) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookies(r),
		MaxAge:   int(m.ttl.Seconds()),
	})
}

func (m *sessionManager) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookies(r),
		MaxAge:   -1,
	})
}

// tokenFromRequest prefers the per-page header token over the cookie. The
// cookie is shared by every tab in the browser, so a second tab logging in
// would otherwise move this tab onto the other tab's session.
func (m *sessionManager) tokenFromRequest(r *http.Request) (token string, fromHeader bool, err error) {
	if headerToken := strings.TrimSpace(r.Header.Get(sessionTokenHeader)); headerToken != "" {
		return headerToken, true, nil
	}
	token, err = m.tokenFromCookie(r)
	return token, false, err
}

func (m *sessionManager) tokenFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("missing session cookie")
	}
	return cookie.Value, nil
}

func secureCookies(r *http.Request) bool {
	if os.Getenv("SECURE_COOKIES") == "false" {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
