package main

import (
	"bytes"
	"context"
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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	bootstrapTurnID = "ipyintervu-bootstrap"
	turnIDHeader    = "X-Turn-Id"
	sessionHeader   = "X-Session-Token"
)

// buildAndStartServer compiles a private copy of the server whose OpenRouter URL
// points at the recording proxy, then runs it on a free localhost port.
func buildAndStartServer(ctx context.Context, serverDir, binDir, upstreamURL, apiKey, logPath string) (string, *exec.Cmd, error) {
	bin := filepath.Join(binDir, "openrouter-app-sim")
	build := exec.Command("go", "build", "-ldflags", "-X main.openRouterURL="+upstreamURL, "-o", bin, ".")
	build.Dir = serverDir
	if out, err := build.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("build server: %v\n%s", err, out)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	port := fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	logFile, err := os.Create(logPath)
	if err != nil {
		return "", nil, err
	}
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(),
		"OPENROUTER_API_KEY="+apiKey,
		"AUTH_PRIVATE_KEY_FILE="+filepath.Join(serverDir, "env", "ipyintervu-key.pem"),
		"PORT="+port,
		"LISTEN_ADDR=127.0.0.1",
		"SECURE_COOKIES=false",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}

	base := "http://127.0.0.1:" + port
	for i := 0; i < 100; i++ {
		if resp, err := http.Get(base + "/healthz"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base, cmd, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return "", nil, fmt.Errorf("server did not become healthy; see %s", logPath)
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

type apiClient struct {
	base   string
	pub    *rsa.PublicKey
	http   *http.Client
	token  string
	model  string
	SessID string
}

func newAPIClient(base string, pub *rsa.PublicKey, model string) *apiClient {
	// The server allows up to 3 upstream turns per chat request, each up to 120s.
	return &apiClient{base: base, pub: pub, model: model, http: &http.Client{Timeout: 7 * time.Minute}}
}

func (c *apiClient) login() error {
	var challenge struct {
		ID    string `json:"challenge_id"`
		Nonce string `json:"nonce"`
	}
	if err := c.getJSON("/api/auth/challenge", &challenge); err != nil {
		return fmt.Errorf("challenge: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(challenge.Nonce)
	if err != nil {
		return err
	}
	cipher, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, c.pub, nonce, nil)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"challenge_id": challenge.ID,
		"ciphertext":   base64.StdEncoding.EncodeToString(cipher),
	})
	resp, err := c.http.Post(c.base+"/api/auth/verify", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verify: status %d", resp.StatusCode)
	}
	var verified struct {
		Token string `json:"session_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verified); err != nil {
		return err
	}
	c.token = verified.Token
	c.SessID = sessionIDFromToken(verified.Token)
	return nil
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
	status, body, err := c.post("/api/session/bootstrap", bootstrapTurnID, map[string]any{"model": c.model})
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

// chat sends one turn, retrying with the same turn ID the way the browser does,
// so the server can replay a completed turn instead of running it twice.
func (c *apiClient) chat(turnID string, history []chatMessage) (content string, attempts int, err error) {
	payload := map[string]any{"model": c.model, "messages": history}
	for attempts = 1; attempts <= 4; attempts++ {
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
				return data.Choices[0].Message.Content, attempts, nil
			}
			err = fmt.Errorf("unexpected chat body: %s", truncate(string(body), 300))
		} else if err == nil {
			err = fmt.Errorf("chat: status %d: %s", status, truncate(strings.TrimSpace(string(body)), 300))
			if status != http.StatusBadGateway && status != http.StatusGatewayTimeout && status != http.StatusServiceUnavailable {
				return "", attempts, err
			}
		}
		time.Sleep(time.Duration(attempts) * 1500 * time.Millisecond)
	}
	return "", attempts - 1, err
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

var (
	ipyFencePattern = regexp.MustCompile("(?is)```(?:json)?\\s*_ipy(?:intervu)?\\s*\\n.*?\\n```")
	ipyJSONPattern  = regexp.MustCompile("(?is)```(?:json)?\\s*_ipy(?:intervu)?\\s*\\n(.*?)\\n```")
)

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
