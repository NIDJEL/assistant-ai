package assistant

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

type RouteDecision struct {
	Path            string `json:"path"`
	FastPathMatched bool   `json:"fast_path_matched"`
	Reason          string `json:"reason"`
}
type planInterpreter interface {
	ParsePlan(context.Context, string, nlp.DialogueContext, time.Time) (nlp.SemanticPlan, error)
}

// A small, anchored whitelist. No extraction, partial match or intent inference.
var exactDelay = regexp.MustCompile(`^через ([1-9][0-9]{0,3}) (секунду|секунды|секунд|минуту|минуты|минут|час|часа|часов)$`)
var exactClock = regexp.MustCompile(`^в ([01]?[0-9]|2[0-3]):([0-5][0-9])$`)
var exactReminder = regexp.MustCompile(`^за ([1-9][0-9]{0,3}) (минуту|минуты|минут|час|часа|часов)$`)

func exactText(message string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(message), ".!?"))
}

func exactPlanFastPath(state domain.State, message string, now time.Time) (RouteDecision, *nlp.ContextualTurn) {
	miss := RouteDecision{Path: "semantic_llm", Reason: "no_exact_fast_path"}
	matched := func(reason string, t nlp.ContextualTurn) (RouteDecision, *nlp.ContextualTurn) {
		return RouteDecision{Path: "fast_path", FastPathMatched: true, Reason: reason}, &t
	}
	low := exactText(message)
	p := state.Pending
	if p != nil {
		confirmation := in(p.Expectation, "delete", "delete_many", "conflict", "batch")
		if confirmation && in(low, "да", "подтверждаю", "удаляй") {
			yes := true
			return matched("confirmation", nlp.ContextualTurn{Status: "ok", Type: "confirmation", Confirmation: &yes})
		}
		if in(low, "отмена", "стоп") || p.Expectation != "reminder" && in(low, "нет", "не надо") {
			return matched("cancellation", nlp.ContextualTurn{Status: "ok", Type: "cancellation"})
		}
		if confirmation && nlp.DeferredConfirmation(message) {
			return matched("deferred_confirmation", nlp.ContextualTurn{Status: "ok", Type: "chat"})
		}
		if p.Expectation == "choose" {
			if i := ordinal(low, min(len(p.Candidates), 8)); i >= 0 && i < min(len(p.Candidates), 5) {
				return matched("ordinal", nlp.ContextualTurn{Status: "ok", Type: "choice", Reference: &nlp.DialogueReference{Relation: []string{"first", "second", "third", "fourth", "fifth"}[i]}})
			}
		}
		if p.Expectation == "reminder" && (exactReminder.MatchString(low) || in(low, "нет", "не надо", "без уведомлений", "в момент события", "в момент начала", "за час")) {
			if _, _, ok, q := nlp.ReminderChoice(message); ok && q == "" {
				return matched("reminder", nlp.ContextualTurn{Status: "ok", Type: "slot_reply", Slots: &nlp.SlotUpdate{ReminderExpression: &message}})
			}
		}
		if p.Expectation == "fields" {
			expect := expectedSlot(p)
			isDate := in(low, "сегодня", "завтра", "послезавтра") && in(expect, "date", "date_or_time", "relative_time_or_date")
			isClock := exactClock.MatchString(low) && in(expect, "time", "date_or_time")
			isDelay := exactDelay.MatchString(low) && in(expect, "relative_time", "relative_time_or_date")
			if isDate || isClock || isDelay {
				if isDelay {
					m := exactDelay.FindStringSubmatch(low)
					n, _ := strconv.Atoi(m[1])
					if n > 1440 {
						return miss, nil
					}
				}
				return matched("slot_"+expect, nlp.ContextualTurn{Status: "ok", Type: "slot_reply", Slots: &nlp.SlotUpdate{TemporalExpression: &message}})
			}
		}
	}
	if p == nil && in(low, "да", "подтверждаю", "удаляй", "нет", "отмена", "не надо", "стоп") {
		reply := "Сейчас нечего подтверждать."
		if in(low, "отмена", "стоп") && state.Dialogue != nil {
			return matched("context_cancellation", nlp.ContextualTurn{Status: "ok", Type: "chat", ChatResponse: &reply})
		}
		return matched("orphan_confirmation", nlp.ContextualTurn{Status: "ok", Type: "chat", ChatResponse: &reply})
	}
	if reply := chatReply(message); reply != "" {
		return matched("chat", nlp.ContextualTurn{Status: "ok", Type: "chat", ChatResponse: &reply})
	}
	return miss, nil
}

// Route is observable without running a provider or a repository mutation.
func Route(state domain.State, message string, now time.Time) RouteDecision {
	expireDialogue(&state, now)
	r, _ := exactPlanFastPath(state, message, now)
	return r
}
