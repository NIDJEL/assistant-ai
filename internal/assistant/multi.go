package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/google/uuid"
)

func (s *Service) applySimpleActions(ctx context.Context, tx repository.Tx, state *domain.State, op nlu.Operation, message string, now time.Time) (domain.Command, domain.Response, error) {
	ctx = context.WithValue(ctx, actionMessageKey{}, message)
	c := neutral("simple_llm")
	if state.Actions == nil && op.Intent != "multi_action" {
		if op.Intent == "resume_action" {
			return c, answer("Сейчас нет незавершённого списка действий.", domain.ActionClarify), nil
		}
		return s.applySimple(ctx, tx, state, op, message, now)
	}
	if op.Confidence == "low" {
		return c, answer("Уточните запрос. Сохранённые действия не изменены.", domain.ActionClarify), nil
	}
	if state.Actions == nil {
		args, err := op.Args()
		if err != nil {
			return c, answer("Уточните список действий. Ничего не изменено.", domain.ActionClarify), nil
		}
		m := args.(*nlu.MultiAction)
		// Check all source spans before executing anything. One step must not
		// inherit the dates, invitations or negation of another source clause.
		for _, step := range m.Steps {
			if !strings.Contains(message, step.Source) {
				return c, answer("Не удалось сопоставить действия с запросом. Повторите список команд.", domain.ActionClarify), nil
			}
		}
		base, _ := json.Marshal(state)
		var saved domain.State
		_ = json.Unmarshal(base, &saved)
		plan := &domain.ActionPlan{Token: uuid.NewString(), At: now, BaseLastIDs: saved.LastIDs, BaseDialogue: saved.Dialogue, Groups: map[string]domain.AtomicGroup{}}
		for _, step := range m.Steps {
			raw, _ := json.Marshal(freezeAction(step.Operation, now))
			plan.Steps = append(plan.Steps, domain.ActionStep{AtomicGroup: step.AtomicGroup, Key: step.Key, Source: step.Source, DependsOn: step.DependsOn, Operation: raw, Status: "ready"})
		}
		state.Actions, state.Pending = plan, nil
		return s.runActions(ctx, tx, state, nil, "", now)
	}
	plan := state.Actions
	if op.Intent == "cancel" {
		for i := range plan.Steps {
			step := &plan.Steps[i]
			if step.Status != "done" {
				step.Status, step.Reply, step.Pending = "cancelled", "Отменено.", nil
			}
		}
		r := actionSummary(plan, answer("Незавершённые действия отменены. Уже выполненные действия сохранены.", domain.ActionCancel))
		state.Actions, state.Pending = nil, nil
		return c, r, nil
	}
	key := plan.Active
	keyed := op.Intent == "resume_action"
	if op.Intent == "resume_action" {
		args, err := op.Args()
		if err != nil {
			return c, actionSummary(plan, answer("Уточните действие для исправления.", domain.ActionClarify)), nil
		}
		a := args.(*nlu.ResumeAction)
		key, op = a.Key, a.Operation
	}
	if op.Intent == "focus_action" {
		args, _ := op.Args()
		key = args.(*nlu.ActionFocus).Key
	}
	var selected *domain.ActionStep
	for i := range plan.Steps {
		if plan.Steps[i].Key == key {
			selected = &plan.Steps[i]
		}
	}
	if selected == nil || selected.Status == "done" || selected.Status == "cancelled" {
		return c, actionSummary(plan, answer("Выберите незавершённое действие. Выполненные шаги повторно не запускаются.", domain.ActionClarify)), nil
	}
	if op.Intent == "focus_action" {
		args, _ := op.Args()
		a := args.(*nlu.ActionFocus)
		for _, step := range plan.Steps {
			if step.Key == a.Key && !in(step.Status, "done", "cancelled") {
				plan.Active = step.Key
				state.Pending = step.Pending
				plan.Token = uuid.NewString()
				r := actionSummary(plan, answer("", domain.ActionClarify))
				if step.Preview != nil {
					r.Scheduling, r.Suggestions, r.Candidates = step.Preview.Scheduling, step.Preview.Suggestions, step.Preview.Candidates
				}
				return c, r, nil
			}
		}
		return c, actionSummary(plan, answer("Действие уже завершено.", domain.ActionClarify)), nil
	}
	if keyed && op.Intent == "cancel" {
		if selected.AtomicGroup != "" {
			for i := range plan.Steps {
				if plan.Steps[i].AtomicGroup == selected.AtomicGroup {
					plan.Steps[i].Status, plan.Steps[i].Reply, plan.Steps[i].Pending = "cancelled", "Атомарная группа отменена.", nil
				}
			}
		}
		selected.Status, selected.Reply, selected.Pending, selected.Preview = "cancelled", "Отменено.", nil, nil
		return s.runActions(ctx, tx, state, nil, "", now)
	}
	if op.Intent == "clarify" || op.Intent == "chat" {
		return c, actionSummary(plan, answer("Ответьте на уточнение выбранного действия или отмените незавершённые действия.", domain.ActionClarify)), nil
	}
	if selected.AtomicGroup != "" {
		return s.runActions(ctx, tx, state, &op, key, now)
	}
	if selected.Status == "waiting" {
		var original nlu.Operation
		_ = json.Unmarshal(selected.Operation, &original)
		if !in(op.Intent, "continue", "select", "confirm", "choose_time", "cancel") && !(selected.Pending != nil && len(selected.Pending.SocialOperation) > 0 && original.Intent == op.Intent) {
			// A replacement is allowed only through an explicit keyed correction.
			if !keyed {
				return c, actionSummary(plan, answer("Сначала завершите уточнение. Для исправления назовите нужное действие и новые параметры.", domain.ActionClarify)), nil
			}
			selected.Operation, _ = json.Marshal(freezeAction(op, now))
			selected.Source, selected.Status, selected.Pending, selected.Preview = message, "ready", nil, nil
			return s.runActions(ctx, tx, state, nil, "", now)
		}
		return s.runActions(ctx, tx, state, &op, key, now)
	}
	if !nlu.StepAllowed(op.Intent) {
		return c, actionSummary(plan, answer("Повторите исправленное действие с параметрами.", domain.ActionClarify)), nil
	}
	selected.Operation, _ = json.Marshal(freezeAction(op, now))
	selected.Source, selected.Status, selected.Pending = message, "ready", nil
	return s.runActions(ctx, tx, state, nil, "", now)
}

func dependenciesReady(plan *domain.ActionPlan, step domain.ActionStep) bool {
	for _, key := range step.DependsOn {
		for _, dependency := range plan.Steps {
			if dependency.Key == key && dependency.Status != "done" {
				return false
			}
		}
	}
	return true
}

func actionTarget(op nlu.Operation) *nlu.Target {
	a, _ := op.Args()
	switch v := a.(type) {
	case *nlu.Update:
		return &v.Target
	case *nlu.Mutation:
		return &v.Target
	case *nlu.MeetingParticipants:
		return &v.Target
	}
	return nil
}

func (s *Service) runActions(ctx context.Context, tx repository.Tx, state *domain.State, continuation *nlu.Operation, key string, now time.Time) (domain.Command, domain.Response, error) {
	plan := state.Actions
	combined := answer("", domain.ActionClarify)
	last := neutral("simple_llm")
	applied := map[string]bool{}
	visitedGroups := map[string]bool{}
	for i := range plan.Steps {
		step := &plan.Steps[i]
		resume := continuation != nil && step.Key == key
		if step.AtomicGroup != "" && ctx.Value(atomicRunKey{}) != true {
			if visitedGroups[step.AtomicGroup] {
				continue
			}
			if resume || in(step.Status, "ready", "blocked") {
				visitedGroups[step.AtomicGroup] = true
				r, err := s.runAtomicGroup(ctx, tx, state, step.AtomicGroup, continuation, key, now)
				if err != nil {
					return last, r, err
				}
				combined.Events = append(combined.Events, r.Events...)
				if len(r.Events) > 0 {
					combined.Action = r.Action
				}
				for _, v := range r.Operations {
					if v.Applied {
						applied[v.Key] = true
					}
				}
			}
			continue
		}
		if !resume && !in(step.Status, "ready", "blocked") {
			continue
		}
		if !dependenciesReady(plan, *step) {
			step.Status, step.Reply = "blocked", "Ожидает выполнения предыдущего связанного действия."
			for _, dep := range step.DependsOn {
				for _, prior := range plan.Steps {
					if prior.Key == dep && prior.Status == "cancelled" {
						step.Status, step.Reply = "cancelled", "Связанное действие отменено."
					}
				}
			}
			continue
		}
		var op nlu.Operation
		var err error
		if resume {
			op = *continuation
		} else {
			op, err = nlu.Decode(step.Operation)
			if err == nil && !nlu.StepAllowed(op.Intent) {
				err = fmt.Errorf("unsupported plan operation")
			}
		}
		if err != nil {
			step.Status, step.Reply = "failed", "Аргументы не прошли проверку. Уточните это действие."
			continue
		}
		local := *state
		local.Actions, local.Pending = nil, step.Pending
		base, _ := json.Marshal(plan.BaseDialogue)
		local.Dialogue = nil
		_ = json.Unmarshal(base, &local.Dialogue)
		local.LastIDs = append([]int64{}, plan.BaseLastIDs...)
		if len(step.DependsOn) == 1 {
			for _, prior := range plan.Steps {
				if prior.Key == step.DependsOn[0] && prior.Status == "done" {
					local.LastIDs = append([]int64{}, prior.IDs...)
					local.Dialogue = &domain.DialogueState{ResultAt: now}
				}
			}
		}
		// Explicit step references are resolved only to a completed dependency.
		if target := actionTarget(op); target != nil && target.Type == "step" {
			var ids []int64
			for _, dep := range step.DependsOn {
				if dep == target.Value {
					for _, prior := range plan.Steps {
						if prior.Key == dep && prior.Status == "done" {
							ids = prior.IDs
						}
					}
				}
			}
			if len(ids) != 1 || target.Index != 0 || target.Filter != nil {
				step.Status, step.Reply = "failed", "Нужен один результат указанного зависимого действия."
				continue
			}
			// The LLM never sees or chooses this identifier.
			local.LastIDs = append([]int64{}, ids...)
			local.Dialogue = &domain.DialogueState{ResultAt: now}
			op = replaceStepTarget(op)
		}
		text := step.Source
		if resume {
			text = actionMessage(ctx)
		}
		cmd, r, err := s.applySimple(ctx, tx, &local, op, text, now)
		if err != nil {
			// Infrastructure failures roll back effects, state and request cache
			// together. Retrying the request cannot replay committed steps.
			return cmd, domain.Response{}, err
		}
		if p := local.Pending; p != nil {
			if in(p.Expectation, "batch", "delete", "delete_many", "conflict") {
				p.ConfirmationState = "pending_confirmation"
			}
			if p.Expectation == "conflict" && p.Command.TargetID != nil && p.TargetSnapshot == nil {
				p.TargetSnapshot, err = tx.Get(ctx, *p.Command.TargetID)
				if err != nil {
					return cmd, domain.Response{}, err
				}
			}
		}
		step.Pending, step.Reply, step.Preview = local.Pending, r.Reply, nil
		if r.Search != nil {
			combined.Search = r.Search
		}
		if local.Pending != nil {
			step.Status = "waiting"
			step.Preview = &r
		} else if r.Action == domain.ActionClarify || r.Action == domain.ActionHelp {
			step.Status = "failed"
		} else if r.Action == domain.ActionCancel {
			step.Status = "cancelled"
		} else {
			step.Status = "done"
			applied[step.Key] = true
			step.Action = r.Action
			for _, event := range r.Events {
				step.IDs = append(step.IDs, event.ID)
			}
			rememberSimple(&local, cmd, r, now)
			state.LastIDs, state.Dialogue = local.LastIDs, local.Dialogue
			for _, event := range r.Events {
				found := false
				for j, old := range combined.Events {
					if old.ID == event.ID {
						combined.Events[j], found = event, true
						break
					}
				}
				if !found {
					combined.Events = append(combined.Events, event)
				}
			}
			combined.Action, last = r.Action, cmd
		}
	}
	if ctx.Value(atomicRunKey{}) != true {
		plan.Token = uuid.NewString()
		for name, g := range plan.Groups {
			if g.Ready && g.PreviewAt.Equal(now) {
				snapshot, err := atomicSnapshot(ctx, tx)
				if err != nil {
					return last, domain.Response{}, err
				}
				g.Snapshot = snapshot
				plan.Groups[name] = g
			}
		}
	}
	plan.Active, state.Pending = "", nil
	for _, status := range []string{"waiting", "failed", "blocked"} {
		for i := range plan.Steps {
			step := &plan.Steps[i]
			if plan.Active == "" && step.Status == status {
				plan.Active, state.Pending = step.Key, step.Pending
			}
		}
	}
	if state.Pending != nil {
		for _, step := range plan.Steps {
			if step.Key == plan.Active && step.Preview != nil {
				p := step.Preview
				combined.Suggestions, combined.Candidates, combined.Conflicts = p.Suggestions, p.Candidates, p.Conflicts
				combined.Scheduling, combined.Social = p.Scheduling, p.Social
			}
		}
	}
	combined = actionSummary(plan, combined)
	for i := range combined.Operations {
		combined.Operations[i].Applied = applied[combined.Operations[i].Key]
	}
	if plan.Active == "" {
		state.Actions = nil
	}
	return last, combined, nil
}

type actionMessageKey struct{}

func actionMessage(ctx context.Context) string {
	v, _ := ctx.Value(actionMessageKey{}).(string)
	return v
}

func actionSummary(plan *domain.ActionPlan, r domain.Response) domain.Response {
	lines := []string{}
	if r.Reply != "" {
		lines = append(lines, r.Reply)
	}
	for i, step := range plan.Steps {
		label := map[string]string{"done": "Выполнено", "waiting": "Нужно уточнение", "failed": "Не выполнено", "blocked": "Ожидает", "cancelled": "Отменено"}[step.Status]
		lines = append(lines, fmt.Sprintf("%d. %s: %s", i+1, label, step.Reply))
		r.Operations = append(r.Operations, domain.ActionResult{AtomicGroup: step.AtomicGroup, ControlToken: plan.Token, CanConfirm: step.Pending != nil && in(step.Pending.Expectation, "delete", "delete_many", "batch", "conflict", "confirm_meeting", "atomic_group"), Active: step.Key == plan.Active, Key: step.Key, Status: step.Status, Reply: step.Reply, Action: step.Action, EventIDs: step.IDs})
	}
	if plan.Active != "" {
		for i, step := range plan.Steps {
			if step.Key == plan.Active {
				lines = append(lines, fmt.Sprintf("Продолжим действие %d: %s", i+1, step.Reply))
			}
		}
	}
	r.Reply = strings.Join(lines, "\n")
	return r
}

func replaceStepTarget(op nlu.Operation) nlu.Operation {
	a, _ := op.Args()
	target := nlu.Target{Type: "last"}
	switch v := a.(type) {
	case *nlu.Update:
		v.Target = target
	case *nlu.Mutation:
		v.Target = target
	case *nlu.MeetingParticipants:
		v.Target = target
	}
	op.Arguments, _ = json.Marshal(a)
	return op
}

// Resolve relative calendar expressions when the request is accepted, so a
// clarification across midnight cannot silently move another step to a new day.
func freezeAction(op nlu.Operation, now time.Time) nlu.Operation {
	a, err := op.Args()
	if err != nil {
		return op
	}
	freeze := func(t **nlu.Temporal, clock **string) {
		if *t == nil {
			return
		}
		start, end, exact, err := (*t).Resolve(now)
		if err != nil || start == nil {
			return
		}
		if exact != nil && clock != nil && *clock == nil {
			*t = &nlu.Temporal{Type: "absolute_time", Date: exact.Format(time.RFC3339Nano)}
			return
		}
		if end != nil && exact == nil && (*t).Type != "relative_day" && (*t).Type != "absolute" && (*t).Type != "weekday" && (*t).Type != "day_of_month" {
			*t = &nlu.Temporal{Type: "date_range", Date: start.String(), EndDate: end.String()}
		} else {
			*t = &nlu.Temporal{Type: "absolute", Date: start.String()}
		}
	}
	filter := func(f *nlu.Filter) { freeze(&f.Date, nil); freeze(&f.Period, nil) }
	target := func(t *nlu.Target) {
		if t.Filter != nil {
			filter(t.Filter)
		}
	}
	switch v := a.(type) {
	case *nlu.Create:
		freeze(&v.Date, &v.Time)
		if v.Recurrence != nil {
			freeze(&v.Recurrence.Until, nil)
		}
	case *nlu.Update:
		freeze(&v.Changes.Occurrence, nil)
		freeze(&v.Changes.Date, &v.Changes.Time)
		if v.Changes.Recurrence != nil {
			freeze(&v.Changes.Recurrence.Until, nil)
		}
		target(&v.Target)
	case *nlu.Mutation:
		target(&v.Target)
	case *nlu.Read:
		filter(&v.Filter)
	case *nlu.Batch:
		filter(&v.Filter)
		for i := range v.Exclude {
			filter(&v.Exclude[i])
		}
	case *nlu.BatchUpdate:
		filter(&v.Filter)
		for i := range v.Exclude {
			filter(&v.Exclude[i])
		}
		freeze(&v.Changes.Date, &v.Changes.Time)
	case *nlu.Social:
		freeze(&v.Date, &v.Time)
		freeze(&v.Period, nil)
		for i := range v.Options {
			freeze(&v.Options[i].Date, &v.Options[i].Time)
		}
	case *nlu.MeetingParticipants:
		target(&v.Target)
	}
	op.Arguments, _ = json.Marshal(a)
	return op
}
