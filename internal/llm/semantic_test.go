package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

type fixtureProvider struct {
	raw, name, model string
	calls            int
	err              error
}

func (p *fixtureProvider) Name() string    { return p.name }
func (p *fixtureProvider) ModelID() string { return p.model }
func (p *fixtureProvider) Parse(ctx context.Context, req SemanticRequest) (nlp.SemanticCommand, ProviderMeta, error) {
	p.calls++
	if p.err != nil {
		return nlp.SemanticCommand{}, ProviderMeta{}, p.err
	}
	s, e := nlp.DecodeSemantic([]byte(p.raw))
	return s, ProviderMeta{Raw: p.raw, TotalTokens: 140}, e
}

func TestSemanticCacheProviderModelSourceAndClock(t *testing.T) {
	raw := semanticFixture(t, `{"status":"ok","action":"create","kind":"task","title":"Отправить отчет","date_expression":"завтра"}`)
	provider := &fixtureProvider{raw: raw, name: "gigachat", model: "model"}
	adapter := &Semantic{Provider: provider, CacheDir: t.TempDir()}
	now := time.Date(2026, 10, 15, 12, 30, 17, 0, time.UTC)
	first, err := adapter.Normalize(t.Context(), "нужно отправить отчет завтра", now)
	if err != nil || first.CacheHit {
		t.Fatal(err)
	}
	later, err := adapter.Normalize(t.Context(), "нужно отправить отчет завтра", now.AddDate(0, 0, 2))
	if err != nil || !later.CacheHit || first.Command.Date.String() == later.Command.Date.String() || provider.calls != 1 {
		t.Fatal("cache coupled to Go clock")
	}
	for _, change := range []func(){func() { provider.model = "other" }, func() { provider.name = "ollama" }} {
		change()
		adapter.CacheOnly = true
		if _, err := adapter.Extract(t.Context(), "нужно отправить отчет завтра"); err == nil || provider.calls != 1 {
			t.Fatal("cache identities collide or offline invoked model")
		}
	}
	provider.name, provider.model = "gigachat", "model"
	if _, err := adapter.Extract(WithSource(t.Context(), "voice"), "нужно отправить отчет завтра"); err == nil {
		t.Fatal("source missing from cache key")
	}
	path := filepath.Join(adapter.CacheDir, "gigachat", first.CacheKey+".json")
	meta := ProviderMeta{Raw: strings.TrimSuffix(raw, "}") + `,"id":42}`}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Extract(t.Context(), "нужно отправить отчет завтра"); err == nil {
		t.Fatal("cache bypassed strict decoding")
	}
}

func TestSemanticFailureNeverFallsBackToMutation(t *testing.T) {
	now := time.Now()
	for _, raw := range []string{"{", semanticFixture(t, `{"status":"ok","action":"create","kind":"task","title":"Отправить отчет"}`)} {
		p := &fixtureProvider{name: "gigachat", model: "test", raw: raw}
		if raw != "{" {
			p.err = errors.New("provider unavailable")
		}
		command, err := (&Semantic{Provider: p}).Interpret(t.Context(), "ну поставь отчет завтра", now)
		if err != nil || command.Action != domain.ActionHelp || command.Error == "" {
			t.Fatal("failed semantic call accepted a mutation")
		}
	}
}

func TestSemanticGroundingAndMutationGuards(t *testing.T) {
	for _, tc := range []struct{ text, raw string }{
		{"Добавь встречу с Алисой завтра в 9", `{"status":"ok","action":"create","kind":"meeting","title":"Встреча с Никитой","date_expression":"завтра","time_expression":"9"}`},
		{"Не добавляй задачу отправить отчет", `{"status":"ok","action":"create","kind":"task","title":"Отправить отчет"}`},
		{"Измени на завтра встречу с Никитой на встречу с Геной", `{"status":"ok","action":"update","kind":"meeting","target":{"query":"Встреча с Никитой","all":false},"changes":{"date_expression":"завтра"}}`},
		{"Удали встречу с Алисой", `{"status":"ok","action":"delete","kind":"meeting","target":{"query":"Встреча с Алисой","all":true}}`},
	} {
		p := &fixtureProvider{name: "gigachat", model: "test", raw: semanticFixture(t, tc.raw)}
		result, err := (&Semantic{Provider: p}).Normalize(t.Context(), tc.text, time.Now())
		if err != nil || result.Status != "rejected" || result.Command.Action != domain.ActionHelp || !result.UnsafeProposal {
			t.Fatalf("unsafe output accepted, reason=%s error=%v", result.GuardReason, err)
		}
	}
}

func TestSemanticDialogueAndFastPath(t *testing.T) {
	p := &fixtureProvider{name: "gigachat", model: "test", err: errors.New("must not call")}
	adapter := &Semantic{Provider: p, FastPath: true}
	for _, text := range []string{"да", "нет", "#15", "за 15 минут", "что у меня завтра"} {
		if _, err := adapter.Interpret(t.Context(), text, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if p.calls != 0 {
		t.Fatal("dialogue/list fast path called model")
	}
}

func TestSemanticFastPathScheduledMeetingDeclaration(t *testing.T) {
	p := &fixtureProvider{name: "gigachat", model: "test", err: errors.New("must not call")}
	now := time.Date(2026, 10, 15, 12, 30, 17, 0, time.UTC)
	command, err := (&Semantic{Provider: p, FastPath: true}).Interpret(t.Context(), "В четверг в 11 у меня встреча с дизайнером", now)
	if err != nil || command.Action != domain.ActionCreate || command.Kind != domain.EventMeeting || command.Title != "Встреча с дизайнером" || command.Date == nil || command.Time == nil || command.Time.String() != "11:00" || p.calls != 0 {
		t.Fatalf("scheduled declaration did not use the conservative rules path: action=%s kind=%s model_calls=%d", command.Action, command.Kind, p.calls)
	}
	for _, text := range []string{"В четверг у меня встреча с дизайнером", "В четверг в 11 у меня нет встречи с дизайнером", "В четверг в 11 у меня встреча с дизайнером?"} {
		p.calls = 0
		command, err = (&Semantic{Provider: p, FastPath: true}).Interpret(t.Context(), text, now)
		if err != nil || command.Action == domain.ActionCreate || p.calls != 1 {
			t.Fatal("incomplete, negated or question input bypassed the model guard")
		}
	}
}

func TestSemanticContractExportMatchesSource(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/semantic/structured_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Schema map[string]any `json:"schema"`
		Prompt string         `json:"system_prompt"`
	}
	if json.Unmarshal(raw, &contract) != nil {
		t.Fatal("invalid contract")
	}
	source, _ := json.Marshal(nlp.SemanticSchema())
	exported, _ := json.Marshal(contract.Schema)
	if string(source) != string(exported) || contract.Prompt != SemanticPrompt() {
		t.Fatal("provider-neutral contract drift")
	}
}
