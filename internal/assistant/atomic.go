package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type atomicRunKey struct{}

var errAtomicPreview = errors.New("atomic preview only")

func atomicSnapshot(ctx context.Context, tx repository.Tx) (string, error) {
	items, err := tx.Events(ctx, domain.Filter{})
	if err != nil {
		return "", err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	raw, err := json.Marshal(items)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), err
}

func mergeAtomicContinuation(step *domain.ActionStep, op nlu.Operation, message string, now time.Time) error {
	if op.Intent != "continue" {
		if !nlu.StepAllowed(op.Intent) {
			return errors.New("Для группы укажите полную исправленную команду или ответьте на вопрос о полях")
		}
		step.Operation, _ = json.Marshal(freezeAction(op, now))
		step.Source = message
		return nil
	}
	a, err := op.Args()
	if err != nil {
		return err
	}
	change := a.(*nlu.Continue).Changes
	var original nlu.Operation
	if err = json.Unmarshal(step.Operation, &original); err != nil {
		return err
	}
	args, err := original.Args()
	if err != nil {
		return err
	}
	switch v := args.(type) {
	case *nlu.Create:
		if change.Title != nil {
			v.Title = *change.Title
		}
		if change.Date != nil {
			v.Date = change.Date
		}
		if change.Time != nil {
			v.Time = change.Time
			v.TimeAmbiguous = false
		}
		if change.TimeAmbiguous {
			v.Time = nil
			v.TimeAmbiguous = true
		}
		if change.DurationMinutes != nil {
			v.DurationMinutes = change.DurationMinutes
		}
		if change.ReminderMinutes != nil {
			v.ReminderMinutes = change.ReminderMinutes
		}
		if change.ReminderEnabled != nil {
			v.ReminderEnabled = change.ReminderEnabled
		}
		if change.Recurrence != nil {
			v.Recurrence = change.Recurrence
		}
		if change.ClearRecurrence {
			v.Recurrence = nil
		}
	case *nlu.Update:
		before, _ := json.Marshal(v.Changes)
		after, _ := json.Marshal(change)
		var x, y map[string]json.RawMessage
		_ = json.Unmarshal(before, &x)
		_ = json.Unmarshal(after, &y)
		for k, value := range y {
			x[k] = value
		}
		data, _ := json.Marshal(x)
		if err = json.Unmarshal(data, &v.Changes); err != nil {
			return err
		}
	default:
		return errors.New("Повторите полную исправленную команду для этой группы")
	}
	original.Arguments, _ = json.Marshal(args)
	step.Operation, _ = json.Marshal(freezeAction(original, now))
	step.Source = message
	return nil
}

func (s *Service) runAtomicGroup(ctx context.Context, tx repository.Tx, state *domain.State, name string, continuation *nlu.Operation, key string, now time.Time) (domain.Response, error) {
	plan := state.Actions
	checkpoint, ok := tx.(repository.Checkpoint)
	if !ok {
		return domain.Response{}, errors.New("atomic groups require transactional savepoints")
	}
	for _, step := range plan.Steps {
		if step.AtomicGroup == name {
			var operation nlu.Operation
			_ = json.Unmarshal(step.Operation, &operation)
			args, e := operation.Args()
			allowed := in(operation.Intent, "create_task", "create_meeting", "create_reminder", "update", "complete", "delete")
			if c, ok := args.(*nlu.Create); ok && (len(c.Friends) > 0 || c.FriendTag != "" || c.Invite) {
				allowed = false
			}
			if e != nil || !allowed {
				for i := range plan.Steps {
					if plan.Steps[i].AtomicGroup == name {
						plan.Steps[i].Status = "failed"
						plan.Steps[i].Reply = "В атомарной группе доступны личные создания и одиночные изменения/удаления. Исправьте команду или отмените группу."
					}
				}
				return answer("", domain.ActionClarify), nil
			}
		}
	}
	group := plan.Groups[name]
	if group.EvaluatedAt.IsZero() {
		group.EvaluatedAt = now
	}
	group.EvaluatedAt = group.EvaluatedAt.In(now.Location())
	commit := continuation != nil && continuation.Intent == "confirm" && group.Ready
	if commit {
		current, err := atomicSnapshot(ctx, tx)
		if err != nil {
			return domain.Response{}, err
		}
		if current != group.Snapshot {
			commit = false
			group.Ready = false
		}
	} else if continuation != nil {
		for i := range plan.Steps {
			if plan.Steps[i].Key == key {
				if err := mergeAtomicContinuation(&plan.Steps[i], *continuation, actionMessage(ctx), now); err != nil {
					plan.Steps[i].Reply = err.Error()
					return answer(err.Error(), domain.ActionClarify), nil
				}
			}
		}
		group.Ready = false
		group.EvaluatedAt = now
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return domain.Response{}, err
	}
	var local domain.State
	if err = json.Unmarshal(raw, &local); err != nil {
		return domain.Response{}, err
	}
	child := local.Actions
	child.Steps = nil
	for _, step := range plan.Steps {
		if step.AtomicGroup == name {
			step.Status = "ready"
			step.Pending = nil
			step.Preview = nil
			step.IDs = nil
			child.Steps = append(child.Steps, step)
		} else if step.Status == "done" {
			child.Steps = append(child.Steps, step)
		}
	}
	var preview domain.Response
	allDone := false
	err = checkpoint.WithinSavepoint(ctx, func() error {
		_, preview, err = s.runActions(context.WithValue(ctx, atomicRunKey{}, true), tx, &local, nil, "", group.EvaluatedAt)
		if err != nil {
			return err
		}
		allDone = true
		for _, step := range child.Steps {
			if step.AtomicGroup == name && step.Status != "done" {
				allDone = false
			}
		}
		if allDone {
			for _, event := range preview.Events {
				current, e := tx.Get(ctx, event.ID)
				if e != nil {
					return e
				}
				if current == nil {
					continue
				}
				conflicts, e := s.Calendar.Conflicts(ctx, tx, *current, current.ID)
				if e != nil {
					return e
				}
				if len(conflicts) > 0 {
					allDone = false
					for i := range child.Steps {
						if child.Steps[i].AtomicGroup == name {
							child.Steps[i].Status = "failed"
							child.Steps[i].Reply = "В итоговом состоянии группы есть конфликт встреч. Уточните время; группа не сохранена."
							break
						}
					}
					break
				}
			}
		}
		if !commit || !allDone {
			return errAtomicPreview
		}
		return nil
	})
	if err != nil && !errors.Is(err, errAtomicPreview) {
		return domain.Response{}, err
	}
	if commit && allDone {
		for i := range plan.Steps {
			for _, done := range child.Steps {
				if plan.Steps[i].Key == done.Key && done.AtomicGroup == name {
					plan.Steps[i] = done
				}
			}
		}
		state.LastIDs, state.Dialogue = local.LastIDs, local.Dialogue
		delete(plan.Groups, name)
		return preview, nil
	}
	group.Ready, group.PreviewAt = allDone, now
	if plan.Groups == nil {
		plan.Groups = map[string]domain.AtomicGroup{}
	}
	plan.Groups[name] = group
	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.AtomicGroup != name {
			continue
		}
		for _, staged := range child.Steps {
			if staged.Key != step.Key {
				continue
			}
			step.IDs = nil
			step.Pending, step.Preview = nil, nil
			if allDone {
				summary := step.Source
				for _, event := range preview.Events {
					for _, id := range staged.IDs {
						if event.ID == id {
							summary = fmt.Sprintf("«%s» (%s)", event.Title, event.Kind)
							if event.Date != nil {
								summary += " " + event.Date.String()
							}
							if event.StartTime != nil {
								summary += " " + event.StartTime.String()
							}
						}
					}
				}
				step.Status = "waiting"
				step.Reply = fmt.Sprintf("Группа %s готова: %s. Все изменения будут сохранены вместе после подтверждения.", name, summary)
				step.Pending = &domain.Pending{At: now, Expectation: "atomic_group", Command: neutral("simple_llm")}
			} else if in(staged.Status, "waiting", "failed") {
				step.Status, step.Reply = staged.Status, staged.Reply
				step.Pending = staged.Pending
				// A staged ID cannot escape a rolled-back checkpoint.
				if step.Pending != nil {
					step.Pending.Command.TargetID = nil
					step.Pending.TargetSnapshot = nil
					step.Pending.Candidates = nil
				}
			} else {
				step.Status = "blocked"
				step.Reply = "Группа не сохранена: нужно уточнить остальные действия."
			}
		}
	}
	return answer("", domain.ActionClarify), nil
}

func actionChoiceOperation(state domain.State, choice *domain.ChatChoice) (nlu.Operation, error) {
	bad := domain.Validation("Этот список действий изменился. Используйте актуальный ответ.")
	if state.Actions == nil || state.Actions.Token != choice.Token {
		return nlu.Operation{}, bad
	}
	var step *domain.ActionStep
	for i := range state.Actions.Steps {
		if state.Actions.Steps[i].Key == choice.Key {
			step = &state.Actions.Steps[i]
		}
	}
	if step == nil || in(step.Status, "done", "cancelled") {
		return nlu.Operation{}, bad
	}
	op := nlu.Operation{Intent: "focus_action", Confidence: "high"}
	if choice.Action == "action_focus" {
		op.Arguments, _ = json.Marshal(nlu.ActionFocus{Key: step.Key})
		return op, nil
	}
	intent := "cancel"
	if choice.Action == "action_confirm" {
		if step.Pending == nil || !in(step.Pending.Expectation, "atomic_group", "delete", "delete_many", "batch", "conflict", "confirm_meeting") {
			return nlu.Operation{}, bad
		}
		intent = "confirm"
	} else if choice.Action != "action_cancel" {
		return nlu.Operation{}, bad
	}
	op.Intent = "resume_action"
	op.Arguments, _ = json.Marshal(nlu.ResumeAction{Key: step.Key, Operation: nlu.Operation{Intent: intent, Confidence: "high", Arguments: json.RawMessage(`{}`)}})
	return op, nil
}
