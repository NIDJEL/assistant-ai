package contract

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type recordedCase struct {
	ID, Text, Source string
	Tags             []string
	Expected         struct {
		Action, Title, Query string
		Kind, Date, Time     *string
		StartSecond          int `json:"start_second"`
	}
	Normalization struct{ Raw string }
}
type recordedProvider struct {
	model string
	raw   map[string]string
}

func (p *recordedProvider) Name() string    { return "gigachat" }
func (p *recordedProvider) ModelID() string { return p.model }
func (p *recordedProvider) Parse(_ context.Context, req llm.SemanticRequest) (nlp.SemanticCommand, llm.ProviderMeta, error) {
	raw, ok := p.raw[req.Text]
	if !ok {
		return nlp.SemanticCommand{}, llm.ProviderMeta{}, errors.New("saved response missing; network is disabled")
	}
	s, err := nlp.DecodeSemantic([]byte(raw))
	return s, llm.ProviderMeta{Raw: raw}, err
}
func recordedGiga(t *testing.T) ([]recordedCase, *llm.Semantic) {
	t.Helper()
	name := os.Getenv("GIGACHAT_EVAL_ARTIFACT")
	if name == "" {
		t.Skip("requires local saved GIGACHAT_EVAL_ARTIFACT; never uses network")
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(root(), name)
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal("cannot read saved evaluation")
	}
	var report struct {
		Model   string
		Results []recordedCase
	}
	if json.Unmarshal(raw, &report) != nil || len(report.Results) == 0 {
		t.Fatal("invalid saved evaluation")
	}
	provider := &recordedProvider{model: report.Model, raw: map[string]string{}}
	for _, c := range report.Results {
		provider.raw[c.Text] = c.Normalization.Raw
	}
	return report.Results, &llm.Semantic{Provider: provider}
}
func recordedContext(ctx context.Context, c recordedCase) context.Context {
	source := c.Source
	for _, tag := range c.Tags {
		if tag == "voice" || tag == "asr" {
			source = "voice"
		}
	}
	return llm.WithSource(ctx, source)
}

func TestGigaChatRecordedMandatoryExecutor(t *testing.T) {
	cases, model := recordedGiga(t)
	if len(cases) != 13 {
		t.Skip("mandatory executor requires the 13-case report")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			store, cal, _ := services(t)
			now := time.Date(2026, 10, 15, 12, 30, 17, 0, cal.Location)
			ctx := recordedContext(t.Context(), c)
			var targetID int64
			if c.Expected.Action == "update" || c.Expected.Action == "delete" || c.Expected.Action == "complete" {
				kind, title, date := domain.EventMeeting, c.Expected.Query, domain.Day(now)
				if c.Expected.Action == "delete" {
					date = date.AddDays(1)
				}
				if c.Expected.Action == "complete" {
					kind = domain.EventTask
					title = "Подготовить отчет"
				}
				if err := store.Write(ctx, func(tx repository.Tx) error {
					event, err := tx.Create(ctx, domain.Event{Kind: kind, Title: title, Date: date, StartTime: &domain.TimeOfDay{Hour: 9}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
					if err == nil {
						targetID = event.ID
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			service := assistant.New(store, model, cal)
			response, err := service.Respond(ctx, c.Text, "recorded", c.ID, now)
			if c.Expected.Action == "delete" {
				if err != nil || response.Action != domain.ActionClarify || !response.Pending {
					t.Fatal("deletion skipped confirmation")
				}
				response, err = service.Respond(ctx, "да", "recorded", c.ID+"-confirm", now)
			}
			if err != nil || response.Action.String() != c.Expected.Action {
				t.Fatalf("executor action=%s expected=%s error=%v", response.Action, c.Expected.Action, err)
			}
			if c.Expected.Action == "create" || c.Expected.Action == "update" || c.Expected.Action == "complete" {
				if len(response.Events) != 1 {
					t.Fatal("expected one event")
				}
				event := response.Events[0]
				if c.Expected.Title != "" && event.Title != c.Expected.Title {
					t.Fatal("wrong title")
				}
				if c.Expected.Date != nil && (event.Date == nil || event.Date.String() != *c.Expected.Date) {
					t.Fatal("wrong date")
				}
				if c.Expected.Time != nil && (event.StartTime == nil || event.StartTime.String() != *c.Expected.Time) {
					t.Fatal("wrong time")
				}
				if event.StartSecond != c.Expected.StartSecond || targetID != 0 && event.ID != targetID {
					t.Fatal("wrong seconds or target")
				}
			}
			if c.Expected.Action == "delete" {
				if err := store.Read(ctx, func(tx repository.Tx) error {
					event, err := tx.Get(ctx, targetID)
					if event != nil {
						return errors.New("record not deleted")
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestGigaChatRecordedReadOnlyCannotMutate(t *testing.T) {
	cases, model := recordedGiga(t)
	store, cal, _ := services(t)
	now := time.Date(2026, 10, 15, 12, 30, 17, 0, cal.Location)
	if err := store.Write(t.Context(), func(tx repository.Tx) error {
		_, err := tx.Create(t.Context(), domain.Event{Kind: domain.EventTask, Title: "Исходная контрольная запись", Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var events []domain.Event
		if err := store.Read(t.Context(), func(tx repository.Tx) error {
			var err error
			events, err = tx.Events(t.Context(), domain.Filter{})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(events)
		return string(raw)
	}
	before := snapshot()
	count := 0
	service := assistant.New(store, model, cal)
	for _, c := range cases {
		if c.Expected.Action != "help" && c.Expected.Action != "list" && c.Expected.Action != "search" {
			continue
		}
		response, err := service.Respond(recordedContext(t.Context(), c), c.Text, c.ID, "safety", now)
		if err != nil {
			t.Fatal(err)
		}
		switch response.Action {
		case domain.ActionCreate, domain.ActionUpdate, domain.ActionDelete, domain.ActionComplete:
			t.Fatal("read-only case reached mutation")
		}
		count++
	}
	if snapshot() != before {
		t.Fatal("saved outputs changed database events")
	}
	t.Logf("saved-output DB safety checks=%d changed_events=0", count)
}
