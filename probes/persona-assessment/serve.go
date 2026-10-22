package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type serverInfo struct {
	Base      string    `json:"base"`
	LogPath   string    `json:"logPath"`
	ServerDir string    `json:"serverDir"`
	Started   time.Time `json:"started"`
	PID       int       `json:"pid"`
}

func serverInfoPath() string { return filepath.Join(resultsDir(), "server.json") }

func loadServerInfo() (serverInfo, error) {
	var info serverInfo
	data, err := os.ReadFile(serverInfoPath())
	if err != nil {
		return info, errors.New("no server running: start one with `probe serve` (leave it running)")
	}
	err = json.Unmarshal(data, &info)
	return info, err
}

// cmdServe builds the server from the current source and runs it with the D5 engine on
// a free localhost port, logging to results/. It blocks until interrupted.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	serverDir := fs.String("server-dir", filepath.Join(probeDir(), "..", "..", "openrouter-app"), "openrouter-app source directory")
	port := fs.Int("port", 0, "port to listen on (0 = any free port)")
	fs.Parse(args)

	dir, err := filepath.Abs(*serverDir)
	if err != nil {
		return err
	}
	apiKey, err := openRouterKey()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(runsDir(), 0o755); err != nil {
		return err
	}
	bin := filepath.Join(resultsDir(), "openrouter-app-probe")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build server: %v\n%s", err, out)
	}

	if *port == 0 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		*port = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}
	logPath := filepath.Join(resultsDir(), "server-"+time.Now().Format("20060102-150405")+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"OPENROUTER_API_KEY="+apiKey,
		"IPY_ENGINE=d5",
		"AUTH_PRIVATE_KEY_FILE="+filepath.Join(dir, "env", "ipyintervu-key.pem"),
		fmt.Sprintf("PORT=%d", *port),
		"LISTEN_ADDR=127.0.0.1",
		"SECURE_COOKIES=false",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return err
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", *port)
	healthy := false
	for i := 0; i < 100 && !healthy; i++ {
		if resp, err := http.Get(base + "/healthz"); err == nil {
			resp.Body.Close()
			healthy = resp.StatusCode == http.StatusOK
		}
		if !healthy {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !healthy {
		_ = cmd.Process.Kill()
		return fmt.Errorf("server did not become healthy; see %s", logPath)
	}
	info := serverInfo{Base: base, LogPath: logPath, ServerDir: dir, Started: time.Now(), PID: os.Getpid()}
	data, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(serverInfoPath(), data, 0o644); err != nil {
		return err
	}
	defer os.Remove(serverInfoPath())
	fmt.Printf("D5 server up at %s (log %s). Leave this running; stop with Ctrl-C.\n", base, logPath)

	err = cmd.Wait()
	if ctx.Err() != nil {
		fmt.Println("server stopped")
		return nil
	}
	return fmt.Errorf("server exited: %v; see %s", err, logPath)
}

var envKeyPattern = regexp.MustCompile(`OPENROUTER_API_KEY=["']?([^"'\s]+)`)

// openRouterKey finds the key the same way tools/run-local.sh does, plus ~/.openrouter.key.
func openRouterKey() (string, error) {
	if k := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); k != "" {
		return k, nil
	}
	home, _ := os.UserHomeDir()
	if data, err := os.ReadFile(filepath.Join(home, ".openrouter-env")); err == nil {
		if m := envKeyPattern.FindSubmatch(data); m != nil {
			return string(m[1]), nil
		}
	}
	if data, err := os.ReadFile(filepath.Join(home, ".openrouter.key")); err == nil {
		if k := strings.TrimSpace(string(data)); k != "" {
			return k, nil
		}
	}
	return "", errors.New("no OpenRouter key: set OPENROUTER_API_KEY or create ~/.openrouter.key")
}
