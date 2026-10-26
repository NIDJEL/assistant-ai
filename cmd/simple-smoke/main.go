// simple-smoke talks to GigaChat but only ever opens disposable local databases.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/evaluation"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"github.com/google/uuid"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type eventSpec struct{ Key, Kind, Title, Date, Time, Status string }
type eventWant struct {
	NoParticipants                         bool
	Key, TitleContains, Date, Time, Status string
	ReminderMinutes                        *int
	RecurrenceFrequency                    string
	RecurrenceCount                        *int
	RecurrenceUntil                        string
}
type expectation struct {
	AllowedActions []string
	ReplyContains  []string
	Operations     []string
	Action         string
	Pending        bool
	Count, DBCount *int
	Event          *eventWant
	Records        []eventWant
	Absent         []string
}
type turn struct {
	Message   string
	Operation json.RawMessage
	Want      expectation
}
type scenario struct {
	Now   string
	Name  string
	Seeds []eventSpec
	Turns []turn
}
type turnResult struct {
	Message   string          `json:"message"`
	Response  domain.Response `json:"response"`
	Operation *nlu.Operation  `json:"operation,omitempty"`
	Error     string          `json:"error,omitempty"`
	Failures  []string        `json:"failures,omitempty"`
	LatencyMS float64         `json:"latency_ms"`
	LLMCalls  int             `json:"llm_calls"`
}
type scenarioResult struct {
	Name  string       `json:"name"`
	Pass  bool         `json:"pass"`
	Turns []turnResult `json:"turns"`
}
type report struct {
	Source                                          evaluation.Source
	CorpusSHA256, CreatedAt, Timezone               string
	P50MS, P95MS                                    float64
	Repairs, ExpectedOperations, ReturnedOperations int
	Mode                                            string `json:"mode"`
	Model                                           string `json:"model,omitempty"`
	Scenarios, Passed, Turns, LLMCalls              int
	MeanTurnMS                                      float64          `json:"mean_turn_ms"`
	MeanLLMMS                                       float64          `json:"mean_llm_ms"`
	Results                                         []scenarioResult `json:"results"`
}
type recordingInterpreter struct {
	queryModel   search.QueryModel
	searchBudget time.Duration
	base         *llm.Simple
	fixture      json.RawMessage
	last         *nlu.Operation
	err          string
}

var postgresMode bool

func (*recordingInterpreter) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	panic("legacy interpretation used")
}
func (p *recordingInterpreter) RepairValidation(ctx context.Context, text string, d nlu.Context, previous nlu.Operation, issue nlu.ValidationIssue) (nlu.Operation, error) {
	if p.base == nil {
		return previous, nil
	}
	op, err := p.base.RepairValidation(ctx, text, d, previous, issue)
	p.last = &op
	p.err = ""
	if err != nil {
		p.err = err.Error()
	}
	return op, err
}
func (p *recordingInterpreter) InterpretOperation(ctx context.Context, text string, d nlu.Context) (nlu.Operation, error) {
	var op nlu.Operation
	var err error
	if p.base != nil {
		op, err = p.base.InterpretOperation(ctx, text, d)
	} else {
		op, err = nlu.Decode(p.fixture)
	}
	p.last = &op
	p.err = ""
	if err != nil {
		p.err = err.Error()
	}
	return op, err
}

func main() {
	flag.BoolVar(&postgresMode, "postgres", false, "use disposable local TEST_DATABASE_URL schemas (database name must contain semantic_test)")
	mode := flag.String("mode", "fixture", "fixture or live")
	input := flag.String("input", "testdata/simple_llm/smoke.json", "small realistic corpus")
	output := flag.String("output", "artifacts/simple_llm_smoke.json", "result file")
	only := flag.String("only", "", "optional scenario name substring")
	interactive := flag.Bool("interactive", false, "chat with GigaChat using a disposable database")
	flag.Parse()
	if err := run(*mode, *input, *output, *only, *interactive); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(mode, input, output, only string, interactive bool) error {
	ctx := context.Background()
	p := &recordingInterpreter{}
	model := ""
	if mode != "fixture" && mode != "live" {
		return errors.New("mode must be fixture or live")
	}
	if mode == "live" || interactive {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		g, err := llm.NewGigaChat(cfg.GigaChat)
		if err != nil {
			return err
		}
		defer g.Close()
		p.base = &llm.Simple{Provider: g}
		if cfg.SearchMode == "cloud" {
			p.queryModel = llm.CloudSearch{Client: g}
			p.searchBudget = cfg.SearchBudget
		}
		model = g.ModelID()
	}
	if interactive {
		return chat(ctx, p, output, model)
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var cases []scenario
	if err = json.Unmarshal(data, &cases); err != nil {
		return err
	}
	r := report{Mode: mode, Model: model, Source: evaluation.Identity(), CorpusSHA256: fmt.Sprintf("%x", sha256.Sum256(data)), CreatedAt: time.Now().UTC().Format(time.RFC3339), Timezone: "Europe/Moscow"}
	latencies := []float64{}
	var totalMS, llmMS float64
	for _, c := range cases {
		if only != "" && !strings.Contains(c.Name, only) {
			continue
		}
		result, err := runScenario(ctx, c, p)
		if err != nil {
			return err
		}
		r.Scenarios++
		if result.Pass {
			r.Passed++
		}
		for _, t := range result.Turns {
			latencies = append(latencies, t.LatencyMS)
			if t.Response.Diagnostics != nil {
				r.Repairs += t.Response.Diagnostics.Repairs
			}
			r.ReturnedOperations += len(t.Response.Operations)
			r.Turns++
			r.LLMCalls += t.LLMCalls
			totalMS += t.LatencyMS
			if t.LLMCalls > 0 {
				llmMS += t.LatencyMS
			}
		}
		for _, turn := range c.Turns {
			r.ExpectedOperations += len(turn.Want.Operations)
		}
		r.Results = append(r.Results, result)
		fmt.Fprintf(os.Stderr, "%s pass=%v\n", c.Name, result.Pass)
	}
	if r.Turns > 0 {
		sort.Float64s(latencies)
		r.P50MS = latencies[(len(latencies)-1)/2]
		r.P95MS = latencies[(len(latencies)*95+99)/100-1]
		r.MeanTurnMS = totalMS / float64(r.Turns)
	}
	if r.LLMCalls > 0 {
		r.MeanLLMMS = llmMS / float64(r.LLMCalls)
	}
	data, err = json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0750); err != nil {
		return err
	}
	if err = os.WriteFile(output, data, 0600); err != nil {
		return err
	}
	fmt.Printf("%d/%d scenarios; %d turns; %d model calls; mean turn %.0f ms; mean LLM turn %.0f ms\n", r.Passed, r.Scenarios, r.Turns, r.LLMCalls, r.MeanTurnMS, r.MeanLLMMS)
	if r.Scenarios == 0 || r.Passed != r.Scenarios {
		return errors.New("smoke scenarios failed; inspect result file")
	}
	return nil
}
func scratch(ctx context.Context, p *recordingInterpreter) (*assistant.Service, func(), error) {
	if postgresMode {
		return postgresScratch(ctx, p)
	}
	dir, err := os.MkdirTemp("", "gena-simple-")
	if err != nil {
		return nil, nil, err
	}
	db, err := sqlite.Open(ctx, filepath.Join(dir, "smoke.sqlite3"))
	if err != nil {
		os.RemoveAll(dir)
		return nil, nil, err
	}
	engine, err := search.New()
	if err != nil {
		db.Close()
		os.RemoveAll(dir)
		return nil, nil, err
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	if p.queryModel != nil {
		engine.EnableCloud(p.queryModel, p.searchBudget)
	}
	s := assistant.New(db, p, &calendar.Service{Store: db, Search: engine, Location: loc})
	return s, func() { db.Close(); os.RemoveAll(dir) }, nil
}
func runScenario(ctx context.Context, c scenario, p *recordingInterpreter) (scenarioResult, error) {
	ctx = repository.WithUser(ctx, uuid.NewString())
	r := scenarioResult{Name: c.Name, Pass: true}
	s, close, err := scratch(ctx, p)
	if err != nil {
		return r, err
	}
	defer close()
	now, _ := time.Parse(time.RFC3339, "2026-10-17T12:30:00+03:00")
	if c.Now != "" {
		now, err = time.Parse(time.RFC3339, c.Now)
		if err != nil {
			return r, err
		}
	}
	keys := map[string]int64{}
	err = s.Store.Write(ctx, func(tx repository.Tx) error {
		for _, seed := range c.Seeds {
			kind, err := domain.ParseEventType(seed.Kind)
			if err != nil {
				return err
			}
			e := domain.Event{Kind: kind, Title: seed.Title, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
			if seed.Date != "" {
				e.Date, err = domain.ParseDate(seed.Date)
				if err != nil {
					return err
				}
			}
			if seed.Time != "" {
				e.StartTime, err = domain.ParseTime(seed.Time)
				if err != nil {
					return err
				}
			}
			if seed.Status == "done" {
				e.Status = domain.StatusDone
			}
			v, err := tx.Create(ctx, e, now)
			if err != nil {
				return err
			}
			keys[seed.Key] = v.ID
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	for i, t := range c.Turns {
		p.fixture, p.last, p.err = t.Operation, nil, ""
		calls := 0
		s.ObserveTurn = func(o assistant.DialogueObservation) { calls = o.SemanticCalls }
		started := time.Now()
		response, err := s.Respond(ctx, t.Message, "smoke", fmt.Sprint(i), now.Add(time.Duration(i)*time.Second))
		if response.Diagnostics != nil {
			calls = response.Diagnostics.ModelCalls
		}
		tr := turnResult{Message: t.Message, Response: response, Operation: p.last, Error: p.err, LatencyMS: float64(time.Since(started).Microseconds()) / 1000, LLMCalls: calls}
		if err != nil {
			tr.Error = err.Error()
		}
		actionOK := response.Action.String() == t.Want.Action
		for _, allowed := range t.Want.AllowedActions {
			actionOK = actionOK || response.Action.String() == allowed
		}
		if !actionOK {
			tr.Failures = append(tr.Failures, "action want "+t.Want.Action)
		}
		if response.Pending != t.Want.Pending {
			tr.Failures = append(tr.Failures, "pending mismatch")
		}
		for _, fragment := range t.Want.ReplyContains {
			if !strings.Contains(strings.ToLower(response.Reply), strings.ToLower(fragment)) {
				tr.Failures = append(tr.Failures, "reply missing: "+fragment)
			}
		}
		if t.Want.Count != nil && len(response.Events) != *t.Want.Count {
			tr.Failures = append(tr.Failures, "result count mismatch")
		}
		if t.Want.Operations != nil {
			if len(response.Operations) != len(t.Want.Operations) {
				tr.Failures = append(tr.Failures, "operation omitted or added")
			} else {
				for i, status := range t.Want.Operations {
					if response.Operations[i].Status != status {
						tr.Failures = append(tr.Failures, fmt.Sprintf("operation %d status want %s", i+1, status))
					}
				}
			}
		}
		if t.Want.Event != nil {
			if len(response.Events) != 1 || !matches(response.Events[0], *t.Want.Event) {
				tr.Failures = append(tr.Failures, "event mismatch")
			}
		}
		err = s.Store.Read(ctx, func(tx repository.Tx) error {
			events, err := tx.Events(ctx, domain.Filter{})
			if err != nil {
				return err
			}
			if t.Want.DBCount != nil && len(events) != *t.Want.DBCount {
				tr.Failures = append(tr.Failures, "database count mismatch")
			}
			for _, want := range t.Want.Records {
				found := false
				for _, e := range events {
					if (want.Key == "" || keys[want.Key] == e.ID) && matches(e, want) {
						found = true
						break
					}
				}
				if !found {
					tr.Failures = append(tr.Failures, "record mismatch: "+want.Key)
				}
			}
			for _, key := range t.Want.Absent {
				for _, e := range events {
					if e.ID == keys[key] {
						tr.Failures = append(tr.Failures, "record still present: "+key)
					}
				}
			}
			return nil
		})
		if err != nil {
			return r, err
		}
		if tr.Error != "" || len(tr.Failures) > 0 {
			r.Pass = false
		}
		r.Turns = append(r.Turns, tr)
	}
	return r, nil
}
func matches(e domain.Event, w eventWant) bool {
	if w.NoParticipants && len(e.Participants) > 0 {
		return false
	}
	if w.RecurrenceUntil != "" && (e.Recurrence == nil || e.Recurrence.Until == nil || e.Recurrence.Until.String() != w.RecurrenceUntil) {
		return false
	}
	if w.RecurrenceFrequency != "" && (e.Recurrence == nil || e.Recurrence.Frequency != w.RecurrenceFrequency) {
		return false
	}
	if w.RecurrenceCount != nil && (e.Recurrence == nil || e.Recurrence.Count != *w.RecurrenceCount) {
		return false
	}
	if w.TitleContains != "" && !strings.Contains(strings.ToLower(e.Title), strings.ToLower(w.TitleContains)) {
		return false
	}
	if w.Date != "" && (e.Date == nil || e.Date.String() != w.Date) {
		return false
	}
	if w.Time != "" && (e.StartTime == nil || e.StartTime.String() != w.Time) {
		return false
	}
	if w.Status != "" && e.Status.String() != w.Status {
		return false
	}
	if w.ReminderMinutes != nil && (e.ReminderMinutes == nil || *e.ReminderMinutes != *w.ReminderMinutes) {
		return false
	}
	return true
}
func chat(ctx context.Context, p *recordingInterpreter, output, model string) error {
	ctx = repository.WithUser(ctx, uuid.NewString())
	s, close, err := scratch(ctx, p)
	if err != nil {
		return err
	}
	defer close()
	fmt.Println("Гена: локальный тест, временная пустая БД. /quit — выход.")
	transcript := report{Mode: "interactive", Model: model}
	conversation := scenarioResult{Name: "exploratory local dialogue"}
	scanner := bufio.NewScanner(os.Stdin)
	i := 0
	for scanner.Scan() {
		text := scanner.Text()
		if text == "/quit" {
			break
		}
		i++
		p.last, p.err = nil, ""
		started := time.Now()
		calls := 0
		s.ObserveTurn = func(o assistant.DialogueObservation) { calls = o.SemanticCalls }
		r, err := s.Respond(ctx, text, "manual", fmt.Sprint(i), time.Now())
		if err != nil {
			return err
		}
		fmt.Println("Гена:", r.Reply)
		conversation.Turns = append(conversation.Turns, turnResult{Message: text, Response: r, Operation: p.last, Error: p.err, LatencyMS: float64(time.Since(started).Microseconds()) / 1000, LLMCalls: calls})
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	transcript.Scenarios, transcript.Turns = 1, len(conversation.Turns)
	for _, t := range conversation.Turns {
		transcript.LLMCalls += t.LLMCalls
		transcript.MeanTurnMS += t.LatencyMS
	}
	if transcript.Turns > 0 {
		transcript.MeanTurnMS /= float64(transcript.Turns)
	}
	transcript.Results = []scenarioResult{conversation}
	data, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0750); err != nil {
		return err
	}
	return os.WriteFile(output, data, 0600)
}

func postgresScratch(ctx context.Context, p *recordingInterpreter) (*assistant.Service, func(), error) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "semantic_test") || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		return nil, nil, errors.New("postgres smoke requires a local disposable semantic_test database")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	schema := "smoke_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		return nil, nil, err
	}
	cleanup := func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() }
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := postgres.Open(ctx, u.String())
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	close := func() { db.Close(); cleanup() }
	if _, err = db.DB.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Disposable evaluation')`, repository.UserID(ctx)); err != nil {
		close()
		return nil, nil, err
	}
	engine, err := search.New()
	if err != nil {
		close()
		return nil, nil, err
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	if p.queryModel != nil {
		engine.EnableCloud(p.queryModel, p.searchBudget)
	}
	return assistant.New(db, p, &calendar.Service{Store: db, Search: engine, Location: loc}), close, nil
}
