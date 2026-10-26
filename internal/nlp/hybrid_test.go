package nlp_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

type model struct {
	calls atomic.Int32
	fail  bool
}

func (m *model) Interpret(ctx context.Context, text string, now time.Time) (domain.Command, error) {
	m.calls.Add(1)
	if m.fail {
		return domain.Command{}, errors.New("offline")
	}
	return domain.Command{Action: domain.ActionCreate, Kind: domain.EventTask, Title: "Проверить почту", Duration: time.Hour}, nil
}
func TestConfidenceGateAndFallback(t *testing.T) {
	m := &model{}
	h := nlp.Hybrid{Fallback: m}
	now := time.Now()
	for _, text := range []string{"Напомни завтра в 10 утра позвонить Ивану", "Добавь встречу с Иваном завтра", "в 15:00", "на завтра", "в 20 октября", "да", "да все равно", "сохранить", "#42", "нет", "отмена", "Добавь встречу завтра в 25:00", "Не добавляй встречу завтра", "Оставь встречу как есть", "за час", "полчаса", "без уведомлений"} {
		if _, err := h.Interpret(context.Background(), text, now); err != nil {
			t.Fatal(err)
		}
	}
	if m.calls.Load() != 0 {
		t.Fatal("high-confidence commands reached the LLM")
	}
	cmd, err := h.Interpret(context.Background(), "Помоги организовать почту", now)
	if err != nil || cmd.Provider != "model" || m.calls.Load() != 1 {
		t.Fatalf("fallback not used: %+v %v", cmd, err)
	}
	cmd, err = h.Interpret(context.Background(), "Организуй мою почту завтра", now)
	if err != nil || m.calls.Load() != 2 || cmd.Date == nil || cmd.Date.String() != domain.Day(now).AddDays(1).String() {
		t.Fatalf("a date alone must not make unknown intent high-confidence: %+v %v", cmd, err)
	}
	m.fail = true
	cmd, err = h.Interpret(context.Background(), "Непонятная просьба", now)
	if err != nil || cmd.Provider != "rules_fallback" || cmd.Action != domain.ActionHelp {
		t.Fatalf("unsafe failure: %+v %v", cmd, err)
	}
}
func TestCanceledInterpretation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (nlp.Hybrid{}).Interpret(ctx, "Поставь задачу", time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
