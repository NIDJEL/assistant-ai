// context-eval runs isolated, multi-turn conversations against the real service
// and a local SQLite database. It never connects to the configured application DB.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

type seed struct{ Key, Kind, Title, Date, Time string }
type eventWant struct {
	Title, Kind, Date, Time string
	OccurrenceAt            string `json:"occurrence_at"`
	DurationMinutes         int    `json:"duration_minutes"`
	ReminderMinutes         *int   `json:"reminder_minutes"`
	SameAs                  string `json:"same_as"`
	SaveAs                  string `json:"save_as"`
}
type expectation struct {
	Type      string         `json:"turn_type"`
	Action    string         `json:"action"`
	Pending   bool           `json:"pending"`
	Reference string         `json:"reference"`
	Event     *eventWant     `json:"event"`
	Command   map[string]any `json:"command"`
	Count     *int           `json:"count"`
	MaxCalls  *int           `json:"max_model_calls"`
}
type turn struct {
	Message        string      `json:"message"`
	AdvanceMinutes int         `json:"advance_minutes"`
	Restart        bool        `json:"restart"`
	Expect         expectation `json:"expect"`
}
type scenario struct {
	ID, Category, Now string
	Seeds             []seed
	Turns             []turn
	Mandatory         bool
}
type metric struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Rate    float64 `json:"rate"`
}
type turnResult struct {
	Message      string          `json:"message"`
	Expected     expectation     `json:"expected"`
	Actual       domain.Response `json:"actual"`
	Command      domain.Command  `json:"command"`
	Type         string          `json:"turn_type"`
	Reference    string          `json:"reference"`
	Pass         bool            `json:"pass"`
	Checks       map[string]bool `json:"checks"`
	Calls        int             `json:"model_calls"`
	NetworkCalls int             `json:"network_calls"`
	LatencyMS    float64         `json:"latency_ms"`
	Error        string          `json:"error,omitempty"`
}
type scenarioResult struct {
	ID, Category string
	Pass         bool
	Turns        []turnResult
}
type report struct {
	Model         string                        `json:"model"`
	InputSHA256   string                        `json:"input_sha256"`
	PromptSHA256  string                        `json:"prompt_sha256"`
	Frozen        *evaluationProtocol           `json:"frozen,omitempty"`
	Metrics       map[string]metric             `json:"metrics"`
	Conversations int                           `json:"conversations"`
	Turns         int                           `json:"turns"`
	Unsafe        int                           `json:"unsafe_accepted_mutations"`
	Calls         int                           `json:"model_calls"`
	NetworkCalls  int                           `json:"network_calls"`
	CallsPerTurn  float64                       `json:"model_calls_per_turn"`
	WithoutLLM    float64                       `json:"turns_without_llm_percent"`
	Latency       map[string]map[string]float64 `json:"latency_ms"`
	Results       []scenarioResult              `json:"results"`
}

// Eval caches include the complete compact dialogue, model, prompt and schema.
// Cached responses are strictly decoded again, never trusted as executable data.
type recordingProvider struct {
	base                *llm.GigaChat
	cache               string
	cacheOnly           bool
	Calls, NetworkCalls int
	NetworkLatencies    []float64
	ContextLatencies    []float64
	LastError           string
}

func (p *recordingProvider) Name() string    { return p.base.Name() }
func (p *recordingProvider) ModelID() string { return p.base.ModelID() }
func (p *recordingProvider) cached(key any, call func() (llm.ProviderMeta, error)) (llm.ProviderMeta, error) {
	p.Calls++
	b, _ := json.Marshal(key)
	name := filepath.Join(p.cache, fmt.Sprintf("%x.json", sha256.Sum256(b)))
	if raw, err := os.ReadFile(name); err == nil {
		var m llm.ProviderMeta
		if err := json.Unmarshal(raw, &m); err != nil {
			return m, err
		}
		return m, nil
	}
	if p.cacheOnly {
		return llm.ProviderMeta{}, errors.New("context eval cache miss")
	}
	p.NetworkCalls++
	m, err := call()
	if err != nil {
		p.LastError = err.Error()
		if m.Raw != "" {
			os.MkdirAll(p.cache, 0700)
			raw, _ := json.Marshal(m)
			os.WriteFile(name+".rejected.json", raw, 0600)
		}
		return m, err
	}
	p.NetworkLatencies = append(p.NetworkLatencies, m.InferenceMS)
	if err := os.MkdirAll(p.cache, 0700); err != nil {
		return m, err
	}
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(name, raw, 0600); err != nil {
		return m, err
	}
	return m, nil
}
func (p *recordingProvider) Parse(ctx context.Context, req llm.SemanticRequest) (nlp.SemanticCommand, llm.ProviderMeta, error) {
	m, err := p.cached([]any{p.ModelID(), llm.SemanticPrompt(), nlp.SemanticSchema(), req}, func() (llm.ProviderMeta, error) { _, m, e := p.base.Parse(ctx, req); return m, e })
	if err != nil {
		return nlp.SemanticCommand{}, m, err
	}
	c, err := nlp.DecodeSemantic([]byte(m.Raw))
	return c, m, err
}
func (p *recordingProvider) ParseTurn(ctx context.Context, req llm.SemanticRequest, d nlp.DialogueContext) (nlp.ContextualTurn, llm.ProviderMeta, error) {
	beforeNetwork := p.NetworkCalls
	m, err := p.cached([]any{p.ModelID(), llm.ContextualPrompt(), nlp.ContextualSchema(), req, d}, func() (llm.ProviderMeta, error) { _, m, e := p.base.ParseTurn(ctx, req, d); return m, e })
	if err != nil {
		return nlp.ContextualTurn{}, m, err
	}
	if p.NetworkCalls > beforeNetwork {
		p.ContextLatencies = append(p.ContextLatencies, m.InferenceMS)
	}
	c, err := nlp.DecodeContextualTurn([]byte(m.Raw))
	return c, m, err
}
func main() {
	input := flag.String("input", "testdata/semantic/context_dev.jsonl", "conversation corpus")
	output := flag.String("output", "artifacts/context/dev.json", "JSON report")
	cache := flag.String("cache", "artifacts/cache/context", "strict raw provider cache")
	cacheOnly := flag.Bool("cache-only", false, "replay raw outputs; no network")
	limit := flag.Int("limit", 0, "conversation limit")
	freeze := flag.String("freeze", "", "freeze input, prompts, schemas and local source hashes; no inference")
	protocolPath := flag.String("protocol", "", "require an unchanged frozen protocol before inference")
	flag.Parse()
	content, err := os.ReadFile(*input)
	if err != nil {
		fatal(err)
	}
	var protocol *evaluationProtocol
	if *freeze != "" || *protocolPath != "" {
		frozen, err := freezeProtocol(*input, content)
		if err != nil {
			fatal(err)
		}
		protocol = &frozen
		if *freeze != "" {
			raw, _ := json.MarshalIndent(frozen, "", "  ")
			f, err := os.OpenFile(*freeze, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				fatal(err)
			}
			_, err = f.Write(raw)
			f.Close()
			if err != nil {
				fatal(err)
			}
			fmt.Printf("frozen conversations input_sha256=%s files=%d\n", frozen.InputSHA256, len(frozen.Files))
			return
		}
		raw, err := os.ReadFile(*protocolPath)
		if err != nil {
			fatal(err)
		}
		var saved evaluationProtocol
		if err := json.Unmarshal(raw, &saved); err != nil {
			fatal(err)
		}
		frozen.FrozenAt = saved.FrozenAt
		if !reflect.DeepEqual(saved, frozen) {
			fatal(errors.New("frozen input/prompt/schema/code changed: evaluation refused"))
		}
		protocol = &saved
	}
	if !*cacheOnly && os.Getenv("GIGACHAT_INTEGRATION") != "1" {
		fatal(errors.New("live evaluation requires GIGACHAT_INTEGRATION=1"))
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	cfg.GigaChat.Timeout = 60 * time.Second
	g, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		fatal(err)
	}
	defer g.Close()
	p := &recordingProvider{base: g, cache: *cache, cacheOnly: *cacheOnly}
	model := &llm.Semantic{Provider: p, FastPath: true}
	result := report{Model: g.ModelID(), InputSHA256: fmt.Sprintf("%x", sha256.Sum256(content)), PromptSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(llm.ContextualPrompt()))), Metrics: map[string]metric{}, Latency: map[string]map[string]float64{}}
	result.Frozen = protocol
	if err := os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		fatal(err)
	}
	progress, err := os.Create(*output + ".progress.jsonl")
	if err != nil {
		fatal(err)
	}
	defer progress.Close()
	engine, _ := search.New()
	loc, _ := time.LoadLocation("Europe/Moscow")
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	latencies := map[string][]float64{}
	noLLM := 0
	failures := 0
	for scanner.Scan() {
		var sc scenario
		if err := json.Unmarshal(scanner.Bytes(), &sc); err != nil {
			fatal(err)
		}
		if *limit > 0 && result.Conversations >= *limit {
			break
		}
		if sc.ID == "" || len(sc.Turns) < 1 {
			fatal(errors.New("every scenario needs an ID and at least one turn"))
		}
		now, err := time.Parse(time.RFC3339, sc.Now)
		if err != nil {
			fatal(err)
		}
		now = now.In(loc)
		dir, err := os.MkdirTemp("", "gena-context-")
		if err != nil {
			fatal(err)
		}
		path := filepath.Join(dir, "eval.sqlite3")
		db, err := sqlite.Open(context.Background(), path)
		if err != nil {
			fatal(err)
		}
		cal := &calendar.Service{Store: db, Search: engine, Location: loc}
		service := assistant.New(db, model, cal)
		keys := map[string]int64{}
		for _, seed := range sc.Seeds {
			kind, _ := domain.ParseEventType(seed.Kind)
			e := domain.Event{Kind: kind, Title: seed.Title, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
			if seed.Date != "" {
				e.Date, _ = domain.ParseDate(seed.Date)
			}
			if seed.Time != "" {
				e.StartTime, _ = domain.ParseTime(seed.Time)
			}
			err := db.Write(context.Background(), func(tx repository.Tx) error {
				created, err := tx.Create(context.Background(), e, now)
				if err == nil {
					keys[seed.Key] = created.ID
				}
				return err
			})
			if err != nil {
				fatal(err)
			}
		}
		sr := scenarioResult{ID: sc.ID, Category: sc.Category, Pass: true}
		for i, tr := range sc.Turns {
			if tr.Restart {
				db.Close()
				db, err = sqlite.Open(context.Background(), path)
				if err != nil {
					fatal(err)
				}
				cal = &calendar.Service{Store: db, Search: engine, Location: loc}
				service = assistant.New(db, model, cal)
			}
			var observation assistant.DialogueObservation
			service.ObserveTurn = func(o assistant.DialogueObservation) { observation = o }
			clock := now.Add(time.Duration(tr.AdvanceMinutes) * time.Minute)
			before := snapshot(db)
			p.LastError = ""
			calls, network := p.Calls, p.NetworkCalls
			start := time.Now()
			r, err := service.Respond(context.Background(), tr.Message, "evaluation", fmt.Sprintf("turn-%d", i), clock)
			took := float64(time.Since(start).Microseconds()) / 1000
			actual := turnResult{Message: tr.Message, Expected: tr.Expect, Actual: r, Command: observation.Command, Type: observation.Type, Reference: observation.Reference, Calls: p.Calls - calls, NetworkCalls: p.NetworkCalls - network, LatencyMS: took, Checks: map[string]bool{}}
			if err != nil {
				actual.Error = err.Error()
			} else if p.LastError != "" {
				actual.Error = p.LastError
			}
			want := tr.Expect
			checks := actual.Checks
			checks["action"] = err == nil && r.Action.String() == want.Action
			checks["pending"] = r.Pending == want.Pending
			if want.MaxCalls != nil {
				checks["model_call_budget"] = actual.Calls <= *want.MaxCalls
			}
			if want.Type != "" {
				checks["turn_type_accuracy"] = observation.Type == want.Type
			}
			if want.Reference != "" {
				checks["reference_resolution_accuracy"] = observation.Reference == want.Reference
			}
			if want.Event != nil {
				if want.Event.SameAs != "" {
					checks["target_resolution_accuracy"] = false
				}
				ok := len(r.Events) == 1
				if ok {
					e, w := r.Events[0], want.Event
					ok = (w.Title == "" || e.Title == w.Title) && (w.Kind == "" || e.Kind.String() == w.Kind) && (w.Date == "" || e.Date != nil && e.Date.String() == w.Date) && (w.Time == "" || e.StartTime != nil && e.StartTime.String() == w.Time) && (w.DurationMinutes == 0 || int(e.Duration/time.Minute) == w.DurationMinutes) && (w.ReminderMinutes == nil || e.ReminderMinutes != nil && *e.ReminderMinutes == *w.ReminderMinutes)
					if w.OccurrenceAt != "" {
						at, err := time.Parse(time.RFC3339, w.OccurrenceAt)
						ok = ok && err == nil && e.OccurrenceAt != nil && e.OccurrenceAt.Equal(at)
					}
					if w.SameAs != "" {
						targetOK := e.ID == keys[w.SameAs]
						checks["target_resolution_accuracy"] = targetOK
						ok = ok && targetOK
					}
					if w.SaveAs != "" {
						keys[w.SaveAs] = e.ID
					}
				}
				checks["event"] = ok
			}
			if want.Count != nil {
				checks["result_count"] = len(r.Events) == *want.Count
			}
			if want.Command != nil {
				raw, _ := json.Marshal(observation.Command)
				var cmd map[string]any
				json.Unmarshal(raw, &cmd)
				ok := true
				for field, wantValue := range want.Command {
					if !reflect.DeepEqual(cmd[field], wantValue) {
						ok = false
					}
				}
				checks["domain_command_accuracy"] = ok
			}
			after := snapshot(db)
			if want.Action == "create" || want.Action == "update" || want.Action == "delete" || want.Action == "complete" || want.Action == "list" || want.Action == "search" {
				commandOK := checks["action"] && observation.Command.Action.String() == want.Action
				if want.Event != nil {
					commandOK = commandOK && checks["event"]
				}
				if want.Count != nil {
					commandOK = commandOK && checks["result_count"]
				}
				if explicit, ok := checks["domain_command_accuracy"]; ok {
					commandOK = commandOK && explicit
				}
				checks["domain_command_accuracy"] = commandOK
			}
			readonly := want.Action == "clarify" || want.Action == "help" || want.Action == "cancel" || want.Action == "list" || want.Action == "search"
			changed := !reflect.DeepEqual(before, after)
			if readonly {
				checks["no_mutation"] = !changed
			}
			if changed && (readonly || checks["target_resolution_accuracy"] == false && want.Event != nil && want.Event.SameAs != "") {
				result.Unsafe++
			}
			actual.Pass = true
			for name, ok := range checks {
				actual.Pass = actual.Pass && ok
				if name != "event" && name != "pending" && name != "action" && name != "no_mutation" && name != "result_count" {
					add(result.Metrics, name, ok)
				}
			}
			if want.Type == "slot_reply" {
				add(result.Metrics, "pending_slot_completion", actual.Pass)
			}
			if want.Type == "reference" || want.Type == "refinement" || want.Type == "slot_reply" {
				add(result.Metrics, "context_retention_accuracy", actual.Pass)
			}
			add(result.Metrics, "new_command_vs_continuation_accuracy", (observation.Type == "command") == (want.Type == "command"))
			add(result.Metrics, "end_to_end_accuracy", actual.Pass)
			if actual.Calls == 0 {
				noLLM++
				latencies["without_llm"] = append(latencies["without_llm"], took)
				if want.Type == "slot_reply" {
					latencies["deterministic_slot_reply"] = append(latencies["deterministic_slot_reply"], float64(observation.InterpretationLatency.Microseconds())/1000)
				}
			} else if actual.NetworkCalls > 0 {
				latencies["network_turn"] = append(latencies["network_turn"], took)
			}
			result.Turns++
			sr.Pass = sr.Pass && actual.Pass
			sr.Turns = append(sr.Turns, actual)
		}
		db.Close()
		os.RemoveAll(dir)
		if len(sc.Turns) > 1 {
			add(result.Metrics, "multi_turn_success_rate", sr.Pass)
		} else {
			add(result.Metrics, "single_turn_success_rate", sr.Pass)
		}
		if sc.Mandatory {
			add(result.Metrics, "mandatory_contextual_scenarios", sr.Pass)
		}
		result.Conversations++
		result.Results = append(result.Results, sr)
		json.NewEncoder(progress).Encode(sr)
		if !sr.Pass && failures < 20 {
			fmt.Printf("FAIL %s (%s)\n", sc.ID, sc.Category)
			failures++
		}
		if result.Conversations%10 == 0 {
			fmt.Printf("progress conversations=%d turns=%d model_calls=%d network=%d\n", result.Conversations, result.Turns, p.Calls, p.NetworkCalls)
		}
	}
	if err := scanner.Err(); err != nil {
		fatal(err)
	}
	result.Calls, result.NetworkCalls = p.Calls, p.NetworkCalls
	if result.Turns > 0 {
		result.CallsPerTurn = float64(p.Calls) / float64(result.Turns)
		result.WithoutLLM = 100 * float64(noLLM) / float64(result.Turns)
	}
	latencies["gigachat_inference"] = p.NetworkLatencies
	latencies["contextual_gigachat"] = p.ContextLatencies
	for name, values := range latencies {
		if len(values) == 0 {
			continue
		}
		sort.Float64s(values)
		sum := 0.
		for _, x := range values {
			sum += x
		}
		result.Latency[name] = map[string]float64{"n": float64(len(values)), "mean": sum / float64(len(values)), "p50": values[len(values)/2], "p95": values[min(len(values)-1, int(float64(len(values))*.95))]}
	}
	raw, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(*output, raw, 0600); err != nil {
		fatal(err)
	}
	summary := result
	summary.Results = nil
	summary.Frozen = nil // Full hashes stay in the report/protocol, not console progress.
	raw, _ = json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(raw))
}

func snapshot(db *sqlite.Database) []domain.Event {
	var events []domain.Event
	err := db.Read(context.Background(), func(tx repository.Tx) error {
		var err error
		events, err = tx.Events(context.Background(), domain.Filter{})
		return err
	})
	if err != nil {
		fatal(err)
	}
	return events
}
func add(metrics map[string]metric, name string, pass bool) {
	m := metrics[name]
	m.Total++
	if pass {
		m.Correct++
	}
	m.Rate = 100 * float64(m.Correct) / float64(m.Total)
	metrics[name] = m
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
