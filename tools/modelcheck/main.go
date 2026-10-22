// modelcheck verifies that the OpenRouter model planned for the D5 redesign honours the
// request settings the design depends on (reasoning off, max_tokens, stop sequences,
// streaming) and measures real output speed. It is standalone: standard library only,
// not part of the openrouter-app build.
//
//	OPENROUTER_API_KEY=... go run tools/modelcheck/main.go [-model deepseek/deepseek-v4-flash-0731] [-runs 5]
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type result struct {
	name         string
	status       int
	err          string
	provider     string
	finish       string
	content      string
	reasoningLen int // characters of reasoning text returned, if any
	usage        usage
	total        time.Duration
	ttft         time.Duration // streaming only
}

func (r result) outTokensPerSec() float64 {
	gen := r.total - r.ttft
	if r.ttft == 0 || gen <= 0 || r.usage.CompletionTokens == 0 {
		return 0
	}
	return float64(r.usage.CompletionTokens) / gen.Seconds()
}

var (
	apiKey string
	model  string
	client = &http.Client{Timeout: 120 * time.Second}
)

func main() {
	flag.StringVar(&model, "model", "deepseek/deepseek-v4-flash-0731", "OpenRouter model id")
	runs := flag.Int("runs", 5, "repetitions of the realistic interviewer turn")
	flag.Parse()
	apiKey = os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY is required")
		os.Exit(1)
	}
	fmt.Printf("Model: %s   Date: %s\n\n", model, time.Now().Format(time.RFC3339))

	short := []message{
		{Role: "system", Content: "You are a job interviewer. Reply in one or two sentences."},
		{Role: "user", Content: "I'd break the order program into reading the items, adding up prices, and printing the total."},
	}

	// 1–3: reasoning behaviour.
	var all []result
	all = append(all, call("1 reasoning: default", short, map[string]any{"max_tokens": 400}))
	all = append(all, call("2 reasoning: enabled=false", short, map[string]any{"max_tokens": 400, "reasoning": map[string]any{"enabled": false}}))
	all = append(all, call("3 reasoning: effort=none", short, map[string]any{"max_tokens": 400, "reasoning": map[string]any{"effort": "none"}}))

	// 4: max_tokens is a hard cap.
	long := []message{{Role: "user", Content: "Write a 600-word essay about loops in Python."}}
	all = append(all, call("4 max_tokens=60", long, map[string]any{"max_tokens": 60, "reasoning": map[string]any{"enabled": false}}))

	// 5: stop sequences cut off a simulated student reply.
	sim := []message{{Role: "user", Content: "Write a short mock interview transcript. Format each line as 'Interviewer: ...' then 'Student: ...'. Write 4 exchanges."}}
	all = append(all, call("5 stop=\\nStudent:", sim, map[string]any{"max_tokens": 300, "reasoning": map[string]any{"enabled": false}, "stop": []string{"\nStudent:", "\nCandidate:", "\nYou:"}}))
	// 5b: same, but only route to providers that support every parameter sent.
	all = append(all, call("5b stop + require_parameters", sim, map[string]any{"max_tokens": 300, "reasoning": map[string]any{"enabled": false}, "stop": []string{"\nStudent:", "\nCandidate:", "\nYou:"}, "provider": map[string]any{"require_parameters": true}}))

	printTable("Settings checks", all)
	verdicts(all)

	// 6–7: realistic interviewer turn (~2K prompt tokens, ≤250 out), streamed, repeated.
	prompt := interviewerPrompt()
	var plain, fast []result
	for i := 0; i < *runs; i++ {
		plain = append(plain, stream(fmt.Sprintf("6 interviewer #%d", i+1), prompt, map[string]any{"max_tokens": 250, "reasoning": map[string]any{"enabled": false}}))
		fast = append(fast, stream(fmt.Sprintf("7 interviewer sort=throughput #%d", i+1), prompt, map[string]any{"max_tokens": 250, "reasoning": map[string]any{"enabled": false}, "provider": map[string]any{"sort": "throughput"}}))
	}
	printTable("Realistic interviewer turn (streamed, reasoning off, max_tokens 250)", append(plain, fast...))
	summarize("6 default routing", plain)
	summarize("7 provider.sort=throughput", fast)

	// 8: levels-only call at mode close.
	lv := stream("8 levels-only", levelsPrompt(), map[string]any{"max_tokens": 60, "reasoning": map[string]any{"enabled": false}})
	printTable("Levels-only call", []result{lv})
	fmt.Printf("Levels output: %q\n", strings.TrimSpace(lv.content))
}

func body(msgs []message, extra map[string]any) []byte {
	req := map[string]any{"model": model, "messages": msgs, "usage": map[string]any{"include": true}}
	for k, v := range extra {
		req[k] = v
	}
	b, _ := json.Marshal(req)
	return b
}

func post(ctx context.Context, payload []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "iPyInterVu modelcheck")
	return client.Do(req)
}

func call(name string, msgs []message, extra map[string]any) result {
	r := result{name: name}
	start := time.Now()
	resp, err := post(context.Background(), body(msgs, extra))
	if err != nil {
		r.err = err.Error()
		return r
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	r.total = time.Since(start)
	r.status = resp.StatusCode
	var out struct {
		Provider string `json:"provider"`
		Choices  []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
		Usage usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		r.err = "bad json: " + truncate(string(raw), 200)
		return r
	}
	if out.Error != nil {
		r.err = out.Error.Message
	}
	r.provider = out.Provider
	r.usage = out.Usage
	if len(out.Choices) > 0 {
		r.finish = out.Choices[0].FinishReason
		r.content = out.Choices[0].Message.Content
		r.reasoningLen = len(out.Choices[0].Message.Reasoning)
	}
	return r
}

func stream(name string, msgs []message, extra map[string]any) result {
	r := result{name: name}
	extra["stream"] = true
	start := time.Now()
	resp, err := post(context.Background(), body(msgs, extra))
	if err != nil {
		r.err = err.Error()
		return r
	}
	defer resp.Body.Close()
	r.status = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		r.err = truncate(string(raw), 200)
		r.total = time.Since(start)
		return r
	}
	var content, reasoning strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue // SSE comments such as ": OPENROUTER PROCESSING"
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Provider string `json:"provider"`
			Choices  []struct {
				FinishReason *string `json:"finish_reason"`
				Delta        struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *usage `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Provider != "" {
			r.provider = chunk.Provider
		}
		if chunk.Usage != nil {
			r.usage = *chunk.Usage
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" && r.ttft == 0 {
				r.ttft = time.Since(start)
			}
			content.WriteString(c.Delta.Content)
			reasoning.WriteString(c.Delta.Reasoning)
			if c.FinishReason != nil && *c.FinishReason != "" {
				r.finish = *c.FinishReason
			}
		}
	}
	r.total = time.Since(start)
	r.content = content.String()
	r.reasoningLen = reasoning.Len()
	return r
}

func printTable(title string, rs []result) {
	fmt.Printf("== %s ==\n", title)
	fmt.Printf("%-34s %4s %-12s %-7s %6s %6s %6s %6s %7s %7s %6s\n",
		"test", "http", "provider", "finish", "in", "out", "reas", "cached", "ttft_s", "total_s", "tok/s")
	for _, r := range rs {
		fmt.Printf("%-34s %4d %-12s %-7s %6d %6d %6d %6d %7.2f %7.2f %6.0f\n",
			truncate(r.name, 34), r.status, truncate(r.provider, 12), r.finish,
			r.usage.PromptTokens, r.usage.CompletionTokens, r.usage.CompletionTokensDetails.ReasoningTokens,
			r.usage.PromptTokensDetails.CachedTokens, r.ttft.Seconds(), r.total.Seconds(), r.outTokensPerSec())
		if r.err != "" {
			fmt.Printf("   error: %s\n", r.err)
		}
	}
	fmt.Println()
}

func verdicts(rs []result) {
	byName := map[string]result{}
	for _, r := range rs {
		byName[strings.Fields(r.name)[0]] = r
	}
	reasoningOff := func(r result) bool {
		return r.err == "" && r.usage.CompletionTokensDetails.ReasoningTokens == 0 && r.reasoningLen == 0
	}
	fmt.Println("== Verdicts ==")
	fmt.Printf("Reasoning on by default:          %v (reasoning tokens %d)\n",
		!reasoningOff(byName["1"]), byName["1"].usage.CompletionTokensDetails.ReasoningTokens)
	fmt.Printf("reasoning.enabled=false works:    %v\n", reasoningOff(byName["2"]))
	fmt.Printf("reasoning.effort=none works:      %v\n", reasoningOff(byName["3"]))
	m := byName["4"]
	fmt.Printf("max_tokens=60 honoured:           %v (out %d, finish %s)\n",
		m.err == "" && m.usage.CompletionTokens <= 60 && m.finish == "length", m.usage.CompletionTokens, m.finish)
	s := byName["5"]
	fmt.Printf("stop sequence honoured:           %v (finish %s, contains 'Student:' %v)\n",
		s.err == "" && s.finish == "stop" && !strings.Contains(s.content, "Student:"), s.finish, strings.Contains(s.content, "Student:"))
	sr := byName["5b"]
	fmt.Printf("stop + require_parameters:        %v (finish %s, provider %s, contains 'Student:' %v)\n",
		sr.err == "" && sr.finish == "stop" && !strings.Contains(sr.content, "Student:"), sr.finish, sr.provider, strings.Contains(sr.content, "Student:"))
	fmt.Println()
}

func summarize(label string, rs []result) {
	var totals, ttfts, rates []float64
	for _, r := range rs {
		if r.err != "" {
			continue
		}
		totals = append(totals, r.total.Seconds())
		ttfts = append(ttfts, r.ttft.Seconds())
		rates = append(rates, r.outTokensPerSec())
	}
	if len(totals) == 0 {
		fmt.Printf("%s: no successful runs\n\n", label)
		return
	}
	fmt.Printf("%s: total p50 %.2fs max %.2fs | ttft p50 %.2fs | output p50 %.0f tok/s (%d/%d ok)\n\n",
		label, pct(totals, 50), pct(totals, 100), pct(ttfts, 50), pct(rates, 50), len(totals), len(rs))
}

func pct(v []float64, p int) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := (len(s) - 1) * p / 100
	return s[i]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// interviewerPrompt approximates the D5 interviewer call: ~250-token template, scenario,
// notes, and six transcript messages, padded to roughly 2K prompt tokens.
func interviewerPrompt() []message {
	system := `You are Jordan, team lead at Brightline Bakery (small-batch bakery with online ordering).
You are interviewing a candidate with a Nutrition Science background for an entry-level role that uses Python.

How you speak:
- You are a colleague at the company, not a teacher. Never mention weeks, courses, classes, homework, or what the candidate has studied.
- Warm, brief, professional. React to what the candidate just said in at most two sentences, then ask exactly ONE question. Then stop.
- Never explain, hint, give examples of answers, say whether they were right, or answer your own question. Never write the candidate's reply.
- Use only these Python ideas: input/process/output; variables, assignment, expressions; int, float, str, bool; input(); int(), float(), str(); string methods strip, upper, lower, split; if/elif/else; comparisons and boolean logic.
- Plain text, under 90 words.

--- (dynamic below this line) ---
Scenario: Customers order boxes of pastries online. Orders over $50 get 10% off, and a $4.99 delivery fee applies unless the customer picks up in store. The team wants a small script that asks for the order subtotal and whether the customer is picking up, then prints the final amount due.
Colleague's private notes: The candidate has identified the inputs and the discount step. Probe how they would decide whether to add the delivery fee. Do not re-ask: What inputs does the program need?
Your task for this reply: React briefly, then ask one follow-up in the direction of the colleague's notes.`
	// Pad with realistic background so the prompt is near 2K tokens.
	system += "\n\nCompany background: " + strings.Repeat("Brightline Bakery ships fresh pastries to local customers and runs a small storefront; the online ordering team maintains simple scripts that support daily operations, pricing rules, and order summaries. ", 25)
	return []message{
		{Role: "system", Content: system},
		{Role: "assistant", Content: "Hi, I'm Jordan, a team lead at Brightline Bakery. Customers order pastry boxes online... What information would the script need to get from the user?"},
		{Role: "user", Content: "It needs the subtotal of the order and whether they're picking it up or getting it delivered."},
		{Role: "assistant", Content: "Thanks. Once you have the subtotal, what would the script do with it first?"},
		{Role: "user", Content: "I'd check if the subtotal is more than 50 and if it is, take 10 percent off by multiplying by 0.9."},
		{Role: "assistant", Content: "Got it. How would you store the subtotal so you can do that math on it?"},
		{Role: "user", Content: "I'd use float(input()) because input gives back a string and prices can have cents."},
	}
}

func levelsPrompt() []message {
	return []message{
		{Role: "system", Content: `Label the candidate's final answer against the rubric dimensions for this mode.
Dimensions: decomposition, correctness, understanding, ai_use.
Levels: not_ready, competent, exceptional. Only list dimensions the answer gives evidence for.
Output exactly one line and nothing else, in this form:
LEVELS: dimension=level; dimension=level`},
		{Role: "user", Content: "Question: How did you check that the AI's suggestion for the discount logic was right?\nAnswer: I asked it to write the if statement, then I tested it with 40, 50 and 60 dollar orders to make sure only the 60 got the discount, and I changed its variable names to match ours."},
	}
}
