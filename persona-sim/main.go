// Command persona-sim runs simulated students through full IPyIntervu
// assessments and reports where the assigned rating differs from the
// student's intended level, with the full prompt/response chain for each run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type config struct {
	ServerDir        string
	KeyFile          string
	Weeks            []int
	Personas         []persona
	Runs             int
	Concurrency      int
	MaxTurns         int
	InterviewerModel string
	StudentModel     string
	OutDir           string
}

var weekConcepts = map[int]string{
	1: "Week 1 - Problem Decomposition",
	2: "Week 2 - Variables & Expressions",
	3: "Week 3 - Input & Type Casting",
	4: "Week 4 - String Methods",
	5: "Week 5 - Conditionals (if/elif/else)",
	6: "Week 6 - for Loops (Repetition over sequences)",
	7: "Week 7 - while Loops & Menus",
	8: "Week 8 - Lists",
	9: "Week 9 - Lists and Files",
}

func main() {
	var cfg config
	var weeks, personaKeys string
	flag.StringVar(&cfg.ServerDir, "server-dir", "../openrouter-app", "path to the openrouter-app server source")
	flag.StringVar(&cfg.KeyFile, "key-file", "", "file containing the OpenRouter API key (default: $OPENROUTER_API_KEY)")
	flag.StringVar(&weeks, "weeks", "1,2,5", "comma-separated week numbers to assess")
	flag.StringVar(&personaKeys, "personas", "not_ready,competent,exceptional,engineer", "comma-separated personas")
	flag.IntVar(&cfg.Runs, "runs", 2, "runs per persona per week")
	flag.IntVar(&cfg.Concurrency, "concurrency", 4, "assessments run in parallel")
	flag.IntVar(&cfg.MaxTurns, "max-turns", 30, "student messages allowed before a run is marked stuck")
	flag.StringVar(&cfg.InterviewerModel, "model", "deepseek/deepseek-v4-flash", "model the client requests (matches static/app.js)")
	flag.StringVar(&cfg.StudentModel, "student-model", "deepseek/deepseek-v4-flash", "OpenRouter model that plays the students")
	flag.StringVar(&cfg.OutDir, "out", "", "output directory (default results/<timestamp>)")
	flag.Parse()

	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if cfg.KeyFile != "" {
		data, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			log.Fatalf("read key file: %v", err)
		}
		apiKey = strings.TrimSpace(string(data))
	}
	if apiKey == "" {
		log.Fatal("set OPENROUTER_API_KEY or pass -key-file")
	}
	for _, w := range strings.Split(weeks, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(w))
		if err != nil || weekConcepts[n] == "" {
			log.Fatalf("invalid week %q", w)
		}
		cfg.Weeks = append(cfg.Weeks, n)
	}
	for _, k := range strings.Split(personaKeys, ",") {
		p, ok := personaByKey(strings.TrimSpace(k))
		if !ok {
			log.Fatalf("unknown persona %q", k)
		}
		cfg.Personas = append(cfg.Personas, p)
	}
	if cfg.OutDir == "" {
		cfg.OutDir = filepath.Join("results", time.Now().Format("20060102-150405"))
	}
	serverDir, err := filepath.Abs(cfg.ServerDir)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.OutDir, "runs"), 0o755); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	proxy, err := startRecordingProxy(filepath.Join(cfg.OutDir, "prompts"))
	if err != nil {
		log.Fatal(err)
	}
	binDir, err := os.MkdirTemp("", "persona-sim-bin")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(binDir)
	serverCtx, killServer := context.WithCancel(context.Background())
	defer killServer()
	baseURL, _, err := buildAndStartServer(serverCtx, serverDir, binDir, proxy.apiURL, apiKey, filepath.Join(cfg.OutDir, "server.log"))
	if err != nil {
		log.Fatal(err)
	}
	pub, err := loadPublicKey(filepath.Join(serverDir, "env", "ipyintervu-pub.pem"))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("server %s (upstream via recording proxy), output %s", baseURL, cfg.OutDir)

	sim := &simulator{
		baseURL: baseURL, cfg: cfg, proxy: proxy, serverDir: serverDir, outDir: cfg.OutDir,
		student:   &studentLLM{apiKey: apiKey, model: cfg.StudentModel, http: &http.Client{Timeout: 3 * time.Minute}},
		newClient: func() *apiClient { return newAPIClient(baseURL, pub, cfg.InterviewerModel) },
	}

	var jobs []job
	for _, w := range cfg.Weeks {
		for _, p := range cfg.Personas {
			for r := 0; r < cfg.Runs; r++ {
				jobs = append(jobs, job{
					ID:      fmt.Sprintf("w%d-%s-%d", w, p.Key, r+1),
					Persona: p, Week: w, Concept: weekConcepts[w],
					Major: p.Majors[r%len(p.Majors)],
				})
			}
		}
	}
	log.Printf("%d assessments queued (%d weeks x %d personas x %d runs), concurrency %d",
		len(jobs), len(cfg.Weeks), len(cfg.Personas), cfg.Runs, cfg.Concurrency)

	queue := make(chan job)
	var mu sync.Mutex
	var results []runResult
	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				res := sim.run(ctx, j)
				if err := writeRun(cfg.OutDir, res); err != nil {
					log.Printf("write %s: %v", res.ID, err)
				}
				mu.Lock()
				results = append(results, res)
				done := len(results)
				if err := writeSummary(cfg, results, len(jobs)); err != nil {
					log.Printf("write summary: %v", err)
				}
				mu.Unlock()
				verdict := "MATCH"
				if !res.Completed {
					verdict = "INCOMPLETE"
				} else if !res.Match {
					verdict = "MISMATCH"
				}
				log.Printf("[%d/%d] %-26s %-10s expected=%-13s got=%-13s buckets=%s turns=%d upstream=%d handoffs=%d %.0fs %s",
					done, len(jobs), res.ID, verdict, res.Expected, orDash(res.FinalRating), bucketString(res),
					len(res.Turns), res.UpstreamTotal, sumCounts(res.HandoffKinds), res.DurationSec, res.Error)
			}
		}()
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		queue <- j
	}
	close(queue)
	wg.Wait()

	if err := writeSummary(cfg, results, len(jobs)); err != nil {
		log.Fatal(err)
	}
	log.Printf("report: %s", filepath.Join(cfg.OutDir, "summary.md"))
}
