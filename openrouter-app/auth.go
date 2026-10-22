package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"sync"
	"time"
)

const challengeTTL = 5 * time.Minute

type challengeEntry struct {
	nonce     []byte
	expiresAt time.Time
}

type challengeStore struct {
	mu    sync.Mutex
	items map[string]challengeEntry
}

func newChallengeStore() *challengeStore {
	store := &challengeStore{items: make(map[string]challengeEntry)}
	go store.cleanupLoop()
	return store
}

func (s *challengeStore) create() (string, string, error) {
	idBytes := make([]byte, 16)
	nonce := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}

	id := base64.RawURLEncoding.EncodeToString(idBytes)
	encodedNonce := base64.StdEncoding.EncodeToString(nonce)

	s.mu.Lock()
	s.items[id] = challengeEntry{
		nonce:     nonce,
		expiresAt: time.Now().Add(challengeTTL),
	}
	s.mu.Unlock()

	return id, encodedNonce, nil
}

func (s *challengeStore) consume(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.items[id]
	delete(s.items, id)
	if !ok {
		return nil, errors.New("challenge not found")
	}
	if time.Now().After(entry.expiresAt) {
		return nil, errors.New("challenge expired")
	}

	return entry.nonce, nil
}

func (s *challengeStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for id, entry := range s.items {
			if now.After(entry.expiresAt) {
				delete(s.items, id)
			}
		}
		s.mu.Unlock()
	}
}

type authService struct {
	privateKey *rsa.PrivateKey
	challenges *challengeStore
	sessions   *sessionManager
}

func newAuthService(privateKeyPEM string) (*authService, error) {
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	secret, err := sessionSecretFromEnv([]byte(privateKeyPEM))
	if err != nil {
		return nil, err
	}

	return &authService{
		privateKey: privateKey,
		challenges: newChallengeStore(),
		sessions:   newSessionManager(secret),
	}, nil
}

func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rsaKey, nil
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func (a *authService) handleChallenge(w http.ResponseWriter, r *http.Request) {
	challengeID, nonce, err := a.challenges.create()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"challenge_id": challengeID,
		"nonce":        nonce,
	})
}

type verifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Ciphertext  string `json:"ciphertext"`
}

func (a *authService) handleVerify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.ChallengeID == "" || req.Ciphertext == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	expectedNonce, err := a.challenges.consume(req.ChallengeID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ciphertext, err := base64.StdEncoding.DecodeString(req.Ciphertext)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, a.privateKey, ciphertext, nil)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if subtle.ConstantTimeCompare(plaintext, expectedNonce) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	token, err := a.sessions.create()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	a.sessions.setCookie(w, r, token)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"session_token": token})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
