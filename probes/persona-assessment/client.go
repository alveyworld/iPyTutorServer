package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// The client talks to the server the way the browser does (static/app.js): RSA
// challenge login, bootstrap, then /api/chat with the visible history.

const (
	bootstrapTurnID = "ipyintervu-bootstrap"
	turnIDHeader    = "X-Turn-Id"
	sessionHeader   = "X-Session-Token"
	clientModel     = "deepseek/deepseek-v4-flash"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiClient struct {
	base  string
	http  *http.Client
	token string
}

func newAPIClient(base, token string) *apiClient {
	// The server allows several upstream calls per chat request, each up to 120s.
	return &apiClient{base: base, token: token, http: &http.Client{Timeout: 7 * time.Minute}}
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid public key PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not RSA")
	}
	return rsaKey, nil
}

func (c *apiClient) login(pub *rsa.PublicKey) (sessionID string, err error) {
	var challenge struct {
		ID    string `json:"challenge_id"`
		Nonce string `json:"nonce"`
	}
	if err := c.getJSON("/api/auth/challenge", &challenge); err != nil {
		return "", fmt.Errorf("challenge: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(challenge.Nonce)
	if err != nil {
		return "", err
	}
	cipher, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, nonce, nil)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{
		"challenge_id": challenge.ID,
		"ciphertext":   base64.StdEncoding.EncodeToString(cipher),
	})
	resp, err := c.http.Post(c.base+"/api/auth/verify", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verify: status %d", resp.StatusCode)
	}
	var verified struct {
		Token string `json:"session_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verified); err != nil {
		return "", err
	}
	c.token = verified.Token
	return sessionIDFromToken(verified.Token), nil
}

func sessionIDFromToken(token string) string {
	payload, err := base64.RawURLEncoding.DecodeString(strings.SplitN(token, ".", 2)[0])
	if err != nil {
		return ""
	}
	var claims struct {
		SID string `json:"sid"`
	}
	_ = json.Unmarshal(payload, &claims)
	return claims.SID
}

func (c *apiClient) bootstrap() (string, error) {
	status, body, err := c.post("/api/session/bootstrap", bootstrapTurnID, map[string]any{"model": clientModel})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("bootstrap: status %d: %s", status, truncate(string(body), 300))
	}
	var data struct {
		Assistant string `json:"assistant"`
	}
	err = json.Unmarshal(body, &data)
	return data.Assistant, err
}

// chat sends one turn, retrying with the same turn ID the way the browser does, so the
// server can replay a completed turn instead of running it twice.
func (c *apiClient) chat(turnID string, history []chatMessage) (string, error) {
	payload := map[string]any{"model": clientModel, "messages": history}
	var err error
	for attempt := 1; attempt <= 4; attempt++ {
		var status int
		var body []byte
		status, body, err = c.post("/api/chat", turnID, payload)
		if err == nil && status == http.StatusOK {
			var data struct {
				Choices []struct {
					Message chatMessage `json:"message"`
				} `json:"choices"`
			}
			if err = json.Unmarshal(body, &data); err == nil && len(data.Choices) > 0 {
				return data.Choices[0].Message.Content, nil
			}
			err = fmt.Errorf("unexpected chat body: %s", truncate(string(body), 300))
		} else if err == nil {
			err = fmt.Errorf("chat: status %d: %s", status, truncate(strings.TrimSpace(string(body)), 300))
			if status != http.StatusBadGateway && status != http.StatusGatewayTimeout && status != http.StatusServiceUnavailable {
				return "", err
			}
		}
		time.Sleep(time.Duration(attempt) * 1500 * time.Millisecond)
	}
	return "", err
}

func (c *apiClient) state() (map[string]any, error) {
	var st map[string]any
	err := c.getJSON("/api/session/state", &st)
	return st, err
}

func (c *apiClient) post(path, turnID string, payload any) (int, []byte, error) {
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(turnIDHeader, turnID)
	req.Header.Set(sessionHeader, c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}

func (c *apiClient) getJSON(path string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, c.base+path, nil)
	if c.token != "" {
		req.Header.Set(sessionHeader, c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var ipyFencePattern = regexp.MustCompile("(?is)```(?:json)?\\s*_ipy(?:intervu)?\\s*\\n.*?\\n```")

// visibleContent mirrors app.js stripClientVisibleAssistantContent.
func visibleContent(content string) string {
	stripped := ipyFencePattern.ReplaceAllString(content, "")
	if n := strings.Count(stripped, "```"); n%2 == 1 {
		open := strings.LastIndex(stripped, "```")
		after := strings.ToLower(strings.TrimSpace(stripped[open+3:]))
		if after == "" || strings.HasPrefix(after, "json") || strings.HasPrefix(after, "_") {
			stripped = stripped[:open]
		}
	}
	return strings.TrimRight(stripped, " \t\r\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func newTurnID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
