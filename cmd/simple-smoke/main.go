// simple-smoke talks to GigaChat but only ever opens disposable local databases.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type eventSpec struct{ Key, Kind, Title, Date, Time, Status string }
type eventWant struct {
	Key, TitleContains, Date, Time, Status string
	ReminderMinutes                        *int
}
type expectation struct {
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
	Mode                               string `json:"mode"`
	Model                              string `json:"model,omitempty"`
	Scenarios, Passed, Turns, LLMCalls int
	MeanTurnMS                         float64          `json:"mean_turn_ms"`
	MeanLLMMS                          float64          `json:"mean_llm_ms"`
	Results                            []scenarioResult `json:"results"`
}
type recordingInterpreter struct {
	base    *llm.Simple
	fixture json.RawMessage
	last    *nlu.Operation
	err     string
}

func (*recordingInterpreter) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	panic("legacy interpretation used")
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
	r := report{Mode: mode, Model: model}
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
			r.Turns++
			r.LLMCalls += t.LLMCalls
			totalMS += t.LatencyMS
			if t.LLMCalls > 0 {
				llmMS += t.LatencyMS
			}
		}
		r.Results = append(r.Results, result)
		fmt.Fprintf(os.Stderr, "%s pass=%v\n", c.Name, result.Pass)
	}
	if r.Turns > 0 {
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
	s := assistant.New(db, p, &calendar.Service{Store: db, Search: engine, Location: loc})
	return s, func() { db.Close(); os.RemoveAll(dir) }, nil
}
func runScenario(ctx context.Context, c scenario, p *recordingInterpreter) (scenarioResult, error) {
	r := scenarioResult{Name: c.Name, Pass: true}
	s, close, err := scratch(ctx, p)
	if err != nil {
		return r, err
	}
	defer close()
	now, _ := time.Parse(time.RFC3339, "2026-10-17T12:30:00+03:00")
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
		tr := turnResult{Message: t.Message, Response: response, Operation: p.last, Error: p.err, LatencyMS: float64(time.Since(started).Microseconds()) / 1000, LLMCalls: calls}
		if err != nil {
			tr.Error = err.Error()
		}
		if response.Action.String() != t.Want.Action {
			tr.Failures = append(tr.Failures, "action want "+t.Want.Action)
		}
		if response.Pending != t.Want.Pending {
			tr.Failures = append(tr.Failures, "pending mismatch")
		}
		if t.Want.Count != nil && len(response.Events) != *t.Want.Count {
			tr.Failures = append(tr.Failures, "result count mismatch")
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
