// plan-eval uses synthetic conversations and disposable SQLite databases only.
// It never opens the application's configured database or contacts a VPS.
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
	"runtime"
	"sort"
	"strings"
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

type eventSpec struct{ Key, Kind, Title, Date, Time, Status, Priority string }
type want struct {
	Route, Action     string
	Pending           bool
	Count, DBCount    *int
	Query, Start, End *string
	Event             *eventSpec
	Changes           []eventSpec
	Absent            []string
}
type turn struct {
	Message string
	Expect  want
	Restart bool
}
type scenario struct {
	ID, Category, Now string
	Seeds             []eventSpec
	Turns             []turn
}
type metric struct {
	Correct, Total int
	Percent        float64
}
type turnResult struct {
	Message        string                  `json:"message"`
	Expected       want                    `json:"expected"`
	Actual         domain.Response         `json:"actual"`
	Command        domain.Command          `json:"command"`
	Route          assistant.RouteDecision `json:"route"`
	Checks         map[string]bool         `json:"checks"`
	Pass           bool                    `json:"pass"`
	Calls          int                     `json:"semantic_calls"`
	PlanValid      bool                    `json:"plan_valid"`
	LatencyMS      float64                 `json:"latency_ms"`
	Error          string                  `json:"error,omitempty"`
	PlanRaw        string                  `json:"plan_raw,omitempty"`
	EvidenceField  string                  `json:"evidence_field,omitempty"`
	EvidenceReason string                  `json:"evidence_reason,omitempty"`
	WrongValue     int                     `json:"wrong_value_mutations"`
}
type conversationResult struct {
	ID, Category string
	Pass         bool
	Turns        []turnResult
}
type report struct {
	Canonicalizations    int               `json:"semantic_canonicalizations"`
	Model                string            `json:"model"`
	InputSHA256          string            `json:"input_sha256"`
	Protocol             *protocol         `json:"frozen,omitempty"`
	Metrics              map[string]metric `json:"metrics"`
	Conversations, Turns int
	FastPathPercent      float64                       `json:"fast_path_percent"`
	LLMPercent           float64                       `json:"llm_percent"`
	CallsPerTurn         float64                       `json:"semantic_calls_per_turn"`
	Unsafe               int                           `json:"unsafe_accepted_mutations"`
	WrongValue           int                           `json:"wrong_value_mutations"`
	ConfirmedWrongValue  int                           `json:"confirmed_wrong_value_mutations"`
	EvidenceRejections   int                           `json:"evidence_guard_rejections"`
	EvidenceMismatches   int                           `json:"evidence_value_mismatches"`
	EvidenceBreakdown    map[string]int                `json:"evidence_rejection_breakdown"`
	Clarifications       int                           `json:"clarification_turns"`
	VerifierCalls        int                           `json:"verifier_calls"`
	NetworkCalls         int                           `json:"network_inference_requests"`
	Latency              map[string]map[string]float64 `json:"latency_ms"`
	Results              []conversationResult          `json:"results"`
}
type protocol struct {
	Model        string            `json:"model"`
	GoVersion    string            `json:"go_version"`
	FrozenAt     string            `json:"frozen_at"`
	InputSHA256  string            `json:"input_sha256"`
	PromptSHA256 string            `json:"prompt_sha256"`
	SchemaSHA256 string            `json:"schema_sha256"`
	Files        map[string]string `json:"files"`
}

func hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func frozen(input []byte) (protocol, error) {
	schema, _ := json.Marshal(nlp.SemanticPlanSchema())
	p := protocol{FrozenAt: time.Now().UTC().Format(time.RFC3339), InputSHA256: hash(input), PromptSHA256: hash([]byte(llm.PlanPrompt())), SchemaSHA256: hash(schema), Files: map[string]string{}}
	cfg, err := config.Load()
	if err != nil {
		return p, err
	}
	p.Model, p.GoVersion = cfg.GigaChat.Model, runtime.Version()
	for _, root := range []string{"internal", "cmd", "tests"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".go") {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				p.Files[filepath.ToSlash(path)] = hash(b)
			}
			return nil
		})
		if err != nil {
			return p, err
		}
	}
	for _, path := range []string{"go.mod", "go.sum", "internal/search/data/russian.json", "scripts/plan_corpus.py", "scripts/plan_fresh_v3.py", "scripts/plan_safety_corpus.py", "scripts/plan_evidence_corpus.py", "scripts/plan_fresh_v6.py", "scripts/plan_policy_dev.py", "scripts/plan_fresh_v7.py", "scripts/plan_policy_report.py", "scripts/verify_plan_freeze.py", "scripts/plan_architecture_dev.py", "scripts/plan_fresh_v8.py", "scripts/verify_v8_freeze.py", "scripts/plan_v8_report.py", "testdata/semantic/plan_architecture_dev.jsonl", "testdata/semantic/plan_omission_adversarial.jsonl", "testdata/semantic/semantic_plan_evidence_v3.schema.json"} {
		b, err := os.ReadFile(path)
		if err != nil {
			return p, err
		}
		p.Files[path] = hash(b)
	}
	return p, nil
}

type recording struct {
	interval          time.Duration
	lastAttempt       time.Time
	base              *llm.GigaChat
	cache             string
	replay            bool
	Calls, Network    int
	Canonicalizations int
	Latencies         []float64
	LastPlan          nlp.SemanticPlan
	LastError         string
	LastRaw           string
}

func (p *recording) Name() string    { return p.base.Name() }
func (p *recording) ModelID() string { return p.base.ModelID() }
func (p *recording) ParsePlan(ctx context.Context, req llm.SemanticRequest, d nlp.DialogueContext) (nlp.SemanticPlan, llm.ProviderMeta, error) {
	p.Calls++
	p.LastError = ""
	p.LastRaw = ""
	key, _ := json.Marshal([]any{p.ModelID(), llm.PlanPrompt(), nlp.SemanticPlanSchema(), req, d})
	path := filepath.Join(p.cache, hash(key)+".json")
	var meta llm.ProviderMeta
	if raw, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(raw, &meta); err != nil {
			return nlp.SemanticPlan{}, meta, err
		}
	} else {
		if !os.IsNotExist(err) {
			return nlp.SemanticPlan{}, meta, err
		}
		if p.replay {
			return nlp.SemanticPlan{}, meta, errors.New("plan cache miss")
		}
		for attempt := 0; ; attempt++ {
			pause := time.Until(p.lastAttempt.Add(p.interval))
			if attempt > 0 {
				pause = time.Duration(attempt*5) * time.Second
			}
			if pause > 0 {
				select {
				case <-time.After(pause):
				case <-ctx.Done():
					return nlp.SemanticPlan{}, meta, ctx.Err()
				}
			}
			p.lastAttempt = time.Now()
			p.Network++
			_, meta, err = p.base.ParsePlan(ctx, req, d)
			if meta.InferenceMS > 0 {
				p.Latencies = append(p.Latencies, meta.InferenceMS)
			}
			// Transport throttling only; never retry an interpreted model plan.
			transient := err != nil && (strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), "HTTP 502") || strings.Contains(err.Error(), "HTTP 503") || strings.Contains(err.Error(), "transport failed"))
			if !transient || attempt >= 3 {
				break
			}
		}
		p.LastRaw = meta.Raw
		if meta.Raw != "" {
			b, _ := json.Marshal(meta)
			if e := os.WriteFile(path, b, 0600); e != nil {
				return nlp.SemanticPlan{}, meta, e
			}
		}
		if err != nil {
			p.LastError = err.Error()
			return nlp.SemanticPlan{}, meta, err
		}
	}
	p.LastRaw = meta.Raw
	var rawPlan nlp.SemanticPlan
	if json.Unmarshal([]byte(meta.Raw), &rawPlan) == nil {
		_, n := nlp.CanonicalizePlanSource(rawPlan, req.Text)
		p.Canonicalizations += n
	}
	v, err := nlp.DecodeSemanticPlanSource([]byte(meta.Raw), req.Text)
	p.LastPlan = v
	if err != nil {
		p.LastError = err.Error()
	}
	return v, meta, err
}

func main() {
	input := flag.String("input", "testdata/semantic/plan_complex_dev.jsonl", "synthetic conversation corpus")
	output := flag.String("output", "artifacts/plan/dev.json", "report")
	cache := flag.String("cache", "artifacts/cache/plan", "raw model responses, strict decoding on replay")
	replay := flag.Bool("replay", false, "offline raw-response replay")
	freeze := flag.String("freeze", "", "write new immutable protocol; no inference")
	protocolPath := flag.String("protocol", "", "require matching frozen protocol before and after inference")
	limit := flag.Int("limit", 0, "DEV conversation limit (forbidden for holdout)")
	export := flag.String("export-schema", "", "write versioned schema; no inference")
	interval := flag.Duration("interval", 1200*time.Millisecond, "minimum interval between network attempts")
	flag.Parse()
	if *export != "" {
		writeJSON(*export, nlp.SemanticPlanSchema())
		return
	}
	if strings.Contains(strings.ToLower(*input), "v2") || strings.Contains(strings.ToLower(*input), "v5") || strings.Contains(strings.ToLower(*input), "v6") || strings.Contains(strings.ToLower(*input), "v7") {
		fatal(errors.New("v2/v5/v6/v7 audit corpora must not be rerun"))
	}
	content, err := os.ReadFile(*input)
	check(err)
	var saved *protocol
	if *freeze != "" || *protocolPath != "" {
		p, err := frozen(content)
		check(err)
		if *freeze != "" {
			b, _ := json.MarshalIndent(p, "", "  ")
			f, err := os.OpenFile(*freeze, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			check(err)
			_, err = f.Write(b)
			check(err)
			check(f.Close())
			fmt.Printf("Frozen input %s, source files %d\n", p.InputSHA256, len(p.Files))
			return
		}
		var old protocol
		b, err := os.ReadFile(*protocolPath)
		check(err)
		check(json.Unmarshal(b, &old))
		p.FrozenAt = old.FrozenAt
		if !reflect.DeepEqual(p, old) {
			fatal(errors.New("frozen input, schema, prompt or source changed"))
		}
		saved = &old
	}
	if strings.Contains(strings.ToLower(*input), "heldout") && (*protocolPath == "" || *limit != 0) {
		fatal(errors.New("holdout requires freeze protocol and full run"))
	}
	if saved != nil {
		if _, err := os.Stat(*output); err == nil {
			fatal(errors.New("frozen output already exists; refusing re-evaluation"))
		}
		if _, err := os.Stat(*output + ".progress.jsonl"); err == nil {
			fatal(errors.New("frozen inference already started; refusing to rerun"))
		}
	}
	if !*replay && os.Getenv("GIGACHAT_INTEGRATION") != "1" {
		fatal(errors.New("live synthetic evaluation requires GIGACHAT_INTEGRATION=1"))
	}
	cfg, err := config.Load()
	check(err)
	cfg.GigaChat.Timeout = 60 * time.Second
	g, err := llm.NewGigaChat(cfg.GigaChat)
	check(err)
	defer g.Close()
	check(os.MkdirAll(*cache, 0700))
	check(os.MkdirAll(filepath.Dir(*output), 0700))
	p := &recording{base: g, cache: *cache, replay: *replay, interval: *interval}
	model := &llm.Planner{Provider: p}
	result := report{Model: g.ModelID(), InputSHA256: hash(content), Protocol: saved, Metrics: map[string]metric{}, Latency: map[string]map[string]float64{}, EvidenceBreakdown: map[string]int{}}
	progress, err := os.Create(*output + ".progress.jsonl")
	check(err)
	defer progress.Close()
	engine, err := search.New()
	check(err)
	loc, err := time.LoadLocation("Europe/Moscow")
	check(err)
	latency := map[string][]float64{}
	fast, calls, failures := 0, 0, 0
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	ids := map[string]bool{}
	for scanner.Scan() {
		var sc scenario
		check(json.Unmarshal(scanner.Bytes(), &sc))
		if sc.ID == "" || ids[sc.ID] || len(sc.Turns) == 0 {
			fatal(errors.New("invalid/duplicate scenario"))
		}
		ids[sc.ID] = true
		if *limit > 0 && result.Conversations >= *limit {
			break
		}
		now, err := time.Parse(time.RFC3339, sc.Now)
		check(err)
		now = now.In(loc)
		dir, err := os.MkdirTemp("", "gena-plan-eval-")
		check(err)
		db, err := sqlite.Open(context.Background(), filepath.Join(dir, "isolated.sqlite3"))
		check(err)
		cal := &calendar.Service{Store: db, Search: engine, Location: loc}
		service := assistant.New(db, model, cal)
		keys := map[string]int64{}
		for _, seed := range sc.Seeds {
			e := domain.Event{Title: seed.Title, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
			e.Kind, err = domain.ParseEventType(seed.Kind)
			check(err)
			if seed.Date != "" {
				e.Date, err = domain.ParseDate(seed.Date)
				check(err)
			}
			if seed.Time != "" {
				e.StartTime, err = domain.ParseTime(seed.Time)
				check(err)
			}
			if seed.Status != "" {
				e.Status, err = domain.ParseStatus(seed.Status)
				check(err)
			}
			if seed.Priority != "" {
				e.Priority, err = domain.ParsePriority(seed.Priority)
				check(err)
			}
			check(e.Validate())
			check(db.Write(context.Background(), func(tx repository.Tx) error {
				x, e := tx.Create(context.Background(), e, now)
				if x != nil {
					keys[seed.Key] = x.ID
				}
				return e
			}))
		}
		cr := conversationResult{ID: sc.ID, Category: sc.Category, Pass: true}
		for i, tr := range sc.Turns {
			if tr.Restart {
				service = assistant.New(db, model, cal)
			}
			var observation assistant.DialogueObservation
			service.ObserveTurn = func(o assistant.DialogueObservation) { observation = o }
			before := events(db)
			beforeCalls := p.Calls
			start := time.Now()
			r, err := service.Respond(context.Background(), tr.Message, sc.ID, fmt.Sprint(i), now.Add(time.Duration(i)*time.Second))
			ms := float64(time.Since(start).Microseconds()) / 1000
			actual := events(db)
			t := turnResult{Message: tr.Message, Expected: tr.Expect, Actual: r, Command: observation.Command, Route: observation.Route, PlanValid: observation.PlanValid, Calls: p.Calls - beforeCalls, LatencyMS: ms, Checks: map[string]bool{}, Pass: true}
			t.EvidenceField, t.EvidenceReason = observation.EvidenceField, observation.EvidenceReason
			if t.EvidenceReason != "" {
				result.EvidenceRejections++
				result.EvidenceBreakdown[t.EvidenceField+":"+t.EvidenceReason]++
				if t.EvidenceReason == "value_mismatch" || t.EvidenceReason == "source_destination_overlap" {
					result.EvidenceMismatches++
				}
			}
			if r.Action == domain.ActionClarify {
				result.Clarifications++
			}
			if t.Calls > 0 {
				t.PlanRaw = p.LastRaw
			}
			if err != nil {
				t.Error = err.Error()
			} else if p.LastError != "" && t.Calls > 0 {
				t.Error = p.LastError
			}
			w := tr.Expect
			t.Checks["execution"] = err == nil && r.Action.String() == w.Action && r.Pending == w.Pending
			t.Checks["routing"] = observation.Route.Path == w.Route
			t.Checks["one_semantic_call"] = t.Calls <= 1 && t.Calls == observation.SemanticCalls
			if w.Count != nil {
				t.Checks["result_count"] = len(r.Events) == *w.Count
			}
			if w.DBCount != nil {
				t.Checks["db_count"] = len(actual) == *w.DBCount
			}
			if w.Query != nil {
				t.Checks["query"] = equivalentQuery(engine, observation.Command.Query, *w.Query)
			}
			if w.Start != nil {
				t.Checks["period_start"] = observation.Command.PeriodStart != nil && observation.Command.PeriodStart.String() == *w.Start
			}
			if w.End != nil {
				t.Checks["period_end"] = observation.Command.PeriodEnd != nil && observation.Command.PeriodEnd.String() == *w.End
			}
			if w.Event != nil {
				t.Checks["event"] = len(r.Events) > 0 && matches(r.Events[0], *w.Event)
				if w.Action == "create" && len(r.Events) > 0 {
					t.Checks["event"] = matchesCreated(r.Events[0], *w.Event)
				}
				if len(r.Events) > 0 && w.Event.Key != "" {
					keys[w.Event.Key] = r.Events[0].ID
				}
			}
			allowed := map[int64]bool{}
			for _, e := range w.Changes {
				id := keys[e.Key]
				allowed[id] = true
				found := false
				for _, x := range actual {
					if x.ID == id && matches(x, e) {
						found = true
					}
				}
				t.Checks["changed_"+e.Key] = found
			}
			for _, key := range w.Absent {
				id := keys[key]
				allowed[id] = true
				found := false
				for _, x := range actual {
					if x.ID == id {
						found = true
					}
				}
				t.Checks["absent_"+key] = !found
			}
			unsafe := unsafeChanges(before, actual, allowed, w.Action == "create")
			result.Unsafe += unsafe
			t.Checks["no_unsafe_mutation"] = unsafe == 0
			t.WrongValue = wrongValueChanges(before, actual, w, keys)
			result.WrongValue += t.WrongValue
			if observation.Route.Reason == "confirmation" {
				result.ConfirmedWrongValue += t.WrongValue
			}
			t.Checks["no_wrong_value_mutation"] = t.WrongValue == 0
			for _, ok := range t.Checks {
				if !ok {
					t.Pass = false
				}
			}
			if !t.Pass {
				cr.Pass = false
				failures++
			}
			add(&result, "end_to_end", t.Pass)
			add(&result, "routing_accuracy", t.Checks["routing"])
			add(&result, "category_"+sc.Category, t.Pass)
			if !strings.HasPrefix(sc.Category, "simple_") {
				add(&result, "complex_command_accuracy", t.Pass)
			}
			if t.Calls > 0 {
				add(&result, "semantic_plan_validity", t.PlanValid)
			}
			if t.Route.FastPathMatched {
				fast++
			}
			calls += t.Calls
			result.Turns++
			latency[t.Route.Path] = append(latency[t.Route.Path], ms)
			cr.Turns = append(cr.Turns, t)
		}
		add(&result, "multi_turn_success", cr.Pass)
		result.Results = append(result.Results, cr)
		result.Conversations++
		check(json.NewEncoder(progress).Encode(cr))
		check(progress.Sync())
		check(db.Close())
		check(cleanTemporaryDatabase(dir))
		if result.Conversations%10 == 0 {
			fmt.Printf("Conversations %d; turns %d; failures %d; unsafe %d; network %d\n", result.Conversations, result.Turns, failures, result.Unsafe, p.Network)
		}
	}
	check(scanner.Err())
	if result.Turns == 0 {
		fatal(errors.New("empty corpus"))
	}
	result.FastPathPercent = 100 * float64(fast) / float64(result.Turns)
	result.LLMPercent = 100 - result.FastPathPercent
	result.CallsPerTurn = float64(calls) / float64(result.Turns)
	result.NetworkCalls = p.Network
	result.Canonicalizations = p.Canonicalizations
	for k, v := range latency {
		result.Latency[k+"_end_to_end"] = percentiles(v)
	}
	result.Latency["llm_network_inference"] = percentiles(p.Latencies)
	if saved != nil {
		current, err := frozen(content)
		check(err)
		current.FrozenAt = saved.FrozenAt
		if !reflect.DeepEqual(current, *saved) {
			fatal(errors.New("freeze changed during inference"))
		}
	}
	writeJSON(*output, result)
	console := result
	console.Results = nil
	console.Protocol = nil
	b, _ := json.MarshalIndent(console, "", "  ")
	fmt.Println(string(b))
	if failures > 0 {
		os.Exit(1)
	}
}
func events(db repository.Store) []domain.Event {
	var e []domain.Event
	check(db.Read(context.Background(), func(tx repository.Tx) error {
		var err error
		e, err = tx.Events(context.Background(), domain.Filter{})
		return err
	}))
	return e
}

func cleanTemporaryDatabase(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	temp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Dir(abs), temp) || !strings.HasPrefix(filepath.Base(abs), "gena-plan-eval-") {
		return errors.New("refusing cleanup outside evaluator temporary directory")
	}
	return os.RemoveAll(abs)
}
func matches(e domain.Event, w eventSpec) bool {
	return (w.Title == "" || e.Title == w.Title) && (w.Kind == "" || e.Kind.String() == w.Kind) && (w.Date == "" || e.Date != nil && e.Date.String() == w.Date) && (w.Time == "" || e.StartTime != nil && e.StartTime.String() == w.Time) && (w.Status == "" || e.Status.String() == w.Status) && (w.Priority == "" || e.Priority.String() == w.Priority)
}

// An empty create date/time means the user supplied no schedule, not a wildcard
// permitting the model to invent one. Update gold uses preservation separately.
func matchesCreated(e domain.Event, w eventSpec) bool {
	return matches(e, w) && (w.Date != "" || e.Date == nil) && (w.Time != "" || e.StartTime == nil) &&
		(w.Status != "" || e.Status == domain.StatusOpen) && (w.Priority != "" || e.Priority == domain.PriorityNormal)
}

// Inflection changes do not change a query's meaning. Require exact lexical
// token coverage in both directions using the same established search engine.
func equivalentQuery(engine *search.Engine, a, b string) bool {
	if a == b {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	x, certain := engine.TitleMatches([]domain.Event{{Title: a}}, b, false)
	y, alsoCertain := engine.TitleMatches([]domain.Event{{Title: b}}, a, false)
	return certain && alsoCertain && len(x) == 1 && len(y) == 1
}
func unsafeChanges(before, after []domain.Event, allowed map[int64]bool, allowCreate bool) int {
	b, a := map[int64]string{}, map[int64]string{}
	for _, e := range before {
		v, _ := json.Marshal(e)
		b[e.ID] = string(v)
	}
	for _, e := range after {
		v, _ := json.Marshal(e)
		a[e.ID] = string(v)
	}
	n := 0
	for id, old := range b {
		if a[id] != old && !allowed[id] {
			n++
		}
	}
	created := 0
	for id := range a {
		if _, exists := b[id]; !exists && !allowCreate {
			n++
		}
		if _, exists := b[id]; !exists {
			created++
		}
	}
	if allowCreate && created > 1 {
		n += created - 1
	}
	return n
}

// Count records actually written with wrong expected values, including allowed
// IDs. A safe refusal with no write is an accuracy failure, not a wrong write.
func wrongValueChanges(before, after []domain.Event, w want, keys map[string]int64) int {
	old := map[int64]domain.Event{}
	for _, e := range before {
		old[e.ID] = e
	}
	n := 0
	for _, e := range after {
		prev, exists := old[e.ID]
		if exists && reflect.DeepEqual(prev, e) {
			continue
		}
		if !exists {
			if w.Event != nil && !matchesCreated(e, *w.Event) {
				n++
			}
			continue
		}
		for _, expected := range w.Changes {
			if keys[expected.Key] == e.ID && !matchesUpdate(prev, e, expected) {
				n++
				break
			}
		}
		for _, key := range w.Absent {
			if keys[key] == e.ID {
				n++
				break
			}
		}
	}
	for _, expected := range w.Changes {
		id := keys[expected.Key]
		if _, exists := old[id]; !exists {
			continue
		}
		found := false
		for _, e := range after {
			if e.ID == id {
				found = true
			}
		}
		if !found {
			n++
		}
	}
	return n
}

// Empty gold fields mean preserve the original value on update. This catches
// an extra incorrect date/time/status change on an otherwise authorized ID.
func matchesUpdate(prev, actual domain.Event, s eventSpec) bool {
	if !matches(actual, s) {
		return false
	}
	if s.Date == "" && !reflect.DeepEqual(prev.Date, actual.Date) {
		return false
	}
	if s.Time == "" && (!reflect.DeepEqual(prev.StartTime, actual.StartTime) || prev.StartSecond != actual.StartSecond) {
		return false
	}
	if prev.Duration != actual.Duration || !reflect.DeepEqual(prev.ReminderMinutes, actual.ReminderMinutes) || !reflect.DeepEqual(prev.ReminderEnabled, actual.ReminderEnabled) || !reflect.DeepEqual(prev.Recurrence, actual.Recurrence) {
		return false
	}
	if s.Title == "" {
		s.Title = prev.Title
	}
	if s.Kind == "" {
		s.Kind = prev.Kind.String()
	}
	if s.Status == "" {
		s.Status = prev.Status.String()
	}
	if s.Priority == "" {
		s.Priority = prev.Priority.String()
	}
	if s.Date == "" && prev.Date != nil {
		s.Date = prev.Date.String()
	}
	if s.Time == "" && prev.StartTime != nil {
		s.Time = prev.StartTime.String()
	}
	return matches(actual, s)
}
func add(r *report, key string, ok bool) {
	m := r.Metrics[key]
	m.Total++
	if ok {
		m.Correct++
	}
	m.Percent = 100 * float64(m.Correct) / float64(m.Total)
	r.Metrics[key] = m
}
func percentiles(v []float64) map[string]float64 {
	if len(v) == 0 {
		return map[string]float64{"n": 0}
	}
	sort.Float64s(v)
	return map[string]float64{"n": float64(len(v)), "p50": v[(len(v)-1)/2], "p95": v[(len(v)*95+99)/100-1]}
}
func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	check(err)
	check(os.WriteFile(path, b, 0600))
}
func check(err error) {
	if err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
