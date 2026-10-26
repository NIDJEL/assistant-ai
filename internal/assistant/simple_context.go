package assistant

import (
	"context"
	"encoding/json"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func operationName(c domain.Command) string {
	if c.Action == domain.ActionCreate {
		return "create_" + c.Kind.String()
	}
	if c.Batch {
		return "batch_" + c.Action.String()
	}
	return c.Action.String()
}
func compactItem(e domain.Event, index int) nlu.Item {
	v := nlu.Item{Index: index, Kind: e.Kind.String(), Title: bounded(e.Label(), 200), Status: e.Status.String()}
	v.Recurrence = nlu.DisplayRecurrence(e.Recurrence)
	if e.Date != nil {
		v.Date = e.Date.String()
	}
	if e.StartTime != nil {
		v.Time = e.StartTime.String()
	}
	return v
}
func buildSimpleContext(ctx context.Context, tx repository.Tx, state domain.State, now time.Time) (nlu.Context, error) {
	d := nlu.Context{Now: now.Format(time.RFC3339), Timezone: now.Location().String()}
	if plan := state.Actions; plan != nil {
		d.ActiveAction = plan.Active
		for _, step := range plan.Steps {
			var op nlu.Operation
			_ = json.Unmarshal(step.Operation, &op)
			v := nlu.ActionContext{AtomicGroup: step.AtomicGroup, Key: step.Key, Status: step.Status, Source: step.Source, Intent: op.Intent, DependsOn: step.DependsOn}
			if _, err := op.Args(); err == nil && nlu.StepAllowed(op.Intent) && step.Status != "done" {
				v.Operation = &op
			}
			d.Actions = append(d.Actions, v)
		}
	}
	for i, id := range state.LastIDs {
		if i >= 8 {
			break
		}
		e, err := tx.Get(ctx, id)
		if err != nil {
			return d, err
		}
		if e != nil {
			d.LastResults = append(d.LastResults, compactItem(*e, i+1))
		} else {
			d.LastResults = append(d.LastResults, nlu.Item{Index: i + 1, Title: "Запись недоступна"})
		}
	}
	if v := state.Dialogue; v != nil {
		d.LastAction = v.LastAction.String()
		if len(v.AffectedIDs) == 1 {
			e, err := tx.Get(ctx, v.AffectedIDs[0])
			if err != nil {
				return d, err
			}
			if e != nil {
				item := compactItem(*e, 0)
				d.LastAffected = &item
			}
		}
		if q := v.Search; q != nil {
			d.LastSearch = &nlu.SearchSummary{Kind: q.Kind.String(), Query: q.Query}
			if q.Filter != nil {
				d.LastSearch.Conference = q.Filter.Conference
			}
			if q.Start != nil {
				d.LastSearch.Start = q.Start.String()
			}
			if q.End != nil {
				d.LastSearch.End = q.End.String()
			}
			if q.Filter != nil && q.Filter.Status != nil {
				d.LastSearch.Status = q.Filter.Status.String()
				if d.LastSearch.Status == "" {
					d.LastSearch.Status = "any"
				}
			}
		}
	}
	if p := state.Pending; p != nil {
		v := &nlu.Pending{Intent: operationName(p.Command), Expected: p.Expectation, Title: bounded(p.Command.Title, 200), Confirmation: in(p.Expectation, "batch", "delete", "delete_many", "conflict")}
		v.Recurrence = nlu.DisplayRecurrence(p.Command.Recurrence)
		if p.Expectation == "fields" && p.Command.Action == domain.ActionCreate {
			if p.Command.Title == "" {
				v.Missing = append(v.Missing, "title")
			}
			if p.Command.Date == nil && (p.Command.Kind != domain.EventTask || p.Command.Time != nil) {
				v.Missing = append(v.Missing, "date")
			}
			if p.Command.Time == nil && p.Command.Kind != domain.EventTask {
				v.Missing = append(v.Missing, "time")
			}
		}
		if p.RequiredSlot != "" {
			v.Expected = p.RequiredSlot
			v.Missing = []string{p.RequiredSlot}
		}
		if p.Command.Date != nil {
			v.Date = p.Command.Date.String()
		}
		if p.Command.Time != nil {
			v.Time = p.Command.Time.String()
		}
		if p.Command.Action == domain.ActionUpdate {
			v.Changes = displayChanges(p.Command)
		}
		if p.Command.Batch {
			v.Filter = displayFilter(p.Command.Filter)
			v.Changes = displayChanges(p.Command)
			if p.Command.Filter != nil {
				for _, x := range p.Command.Filter.Exclude {
					v.Exclude = append(v.Exclude, *displayFilter(&x))
				}
			}
		}
		for i, e := range p.Candidates {
			if i >= 8 {
				break
			}
			v.Candidates = append(v.Candidates, compactItem(e, i+1))
		}
		d.Pending = v
	}
	return d, nil
}

// Retain only successful actions, IDs and search filters; no source/provenance,
// lexical metadata or complete dialogue history is persisted by this runtime.
func rememberSimple(state *domain.State, c domain.Command, r domain.Response, now time.Time) {
	if r.Action == domain.ActionDelete {
		state.LastIDs, state.Dialogue = nil, nil
		return
	}
	if r.Action == domain.ActionCancel {
		return
	}
	if !in(r.Action.String(), "create", "update", "complete", "search", "list") {
		return
	}
	d := state.Dialogue
	if d == nil {
		d = &domain.DialogueState{}
		state.Dialogue = d
	}
	d.LastAction, d.LastKind = r.Action, c.Kind
	d.LastUserMessage, d.LastAssistantMessage = "", ""
	if r.Action == domain.ActionList || r.Action == domain.ActionSearch {
		d.Search = &domain.DialogueSearch{Action: r.Action, Kind: c.Kind, Query: c.Query, Filter: c.Filter, Start: c.PeriodStart, End: c.PeriodEnd, IDs: append([]int64{}, state.LastIDs...), At: now}
		d.Query, d.Filter, d.PeriodStart, d.PeriodEnd = c.Query, c.Filter, c.PeriodStart, c.PeriodEnd
		d.ResultAt = now
	} else {
		d.AffectedIDs = nil
		for _, e := range r.Events {
			d.AffectedIDs = append(d.AffectedIDs, e.ID)
		}
		d.AffectedAt = now
		// A correction must not renumber the preceding displayed search list.
		if d.Search != nil {
			state.LastIDs = append([]int64{}, d.Search.IDs...)
			d.ResultAt = d.Search.At
		} else {
			d.ResultAt = now
		}
		if len(r.Events) == 1 {
			d.LastKind = r.Events[0].Kind
		}
	}
}
