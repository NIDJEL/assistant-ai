package llm

import (
	"context"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

type repairProvider struct {
	raw, repaired  string
	calls, repairs int
	issue          nlu.ValidationIssue
	delay          bool
}

func (p *repairProvider) ParseOperation(ctx context.Context, _ SemanticRequest, _ nlu.Context) (nlu.Operation, ProviderMeta, error) {
	p.calls++
	if p.delay {
		<-ctx.Done()
		return nlu.Operation{}, ProviderMeta{}, ctx.Err()
	}
	return nlu.Operation{}, ProviderMeta{Raw: p.raw}, nil
}
func (p *repairProvider) RepairOperation(_ context.Context, _ SemanticRequest, _ nlu.Context, previous string, issue nlu.ValidationIssue) (nlu.Operation, ProviderMeta, error) {
	p.repairs++
	p.issue = issue
	return nlu.Operation{}, ProviderMeta{Raw: p.repaired}, nil
}

func TestSimpleOneRepairForSyntaxAndBusinessFieldValidation(t *testing.T) {
	for _, bad := range []string{`{broken`, `{"intent":"create_task","confidence":"high","arguments":{"title":"Отчёт","time":"25:00"}}`, `{"intent":"delete","confidence":"high","arguments":{"target":{"type":"last","id":77}}}`} {
		p := &repairProvider{raw: bad, repaired: `{"intent":"create_task","confidence":"high","arguments":{"title":"Отчёт"}}`}
		ctx, m := nlu.WithMetrics(t.Context())
		s := &Simple{Provider: p}
		op, err := s.InterpretOperation(ctx, "Создай Отчёт", nlu.Context{Now: "2026-10-26T10:00:00+03:00"})
		if err != nil || op.Intent != "create_task" || p.calls != 1 || p.repairs != 1 || m.ModelCalls != 2 || m.Repairs != 1 {
			t.Fatal(op, err, p, m)
		}
	}
}

func TestRecurrenceValidationRetainsIANATimezoneFromCompactContext(t *testing.T) {
	p := &repairProvider{raw: `{"intent":"create_reminder","confidence":"high","arguments":{"title":"Витамины","time":"09:00","recurrence":{"frequency":"day","interval":1,"count":3}}}`}
	op, err := (&Simple{Provider: p}).InterpretOperation(t.Context(), "Витамины ежедневно в 09:00 три раза", nlu.Context{Now: "2026-10-26T04:53:00+03:00", Timezone: "Europe/Moscow"})
	if err != nil || op.Intent != "create_reminder" || p.repairs != 0 {
		t.Fatal(op, err, p.repairs)
	}
}

func TestSimpleRepairIsBoundedAndTimeoutIsNotRegenerated(t *testing.T) {
	p := &repairProvider{raw: `{broken`, repaired: `{still broken`}
	s := &Simple{Provider: p}
	if _, err := s.InterpretOperation(t.Context(), "Отчёт", nlu.Context{}); err == nil || p.repairs != 1 {
		t.Fatal(err, p)
	}
	p = &repairProvider{delay: true}
	s = &Simple{Provider: p, Budget: 5 * time.Millisecond}
	if _, err := s.InterpretOperation(t.Context(), "Отчёт", nlu.Context{}); err == nil || p.repairs != 0 {
		t.Fatal(err, p)
	}
}
