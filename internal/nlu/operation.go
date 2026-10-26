// Package nlu defines the small LLM operation contract. It contains no language parser.
package nlu

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type Operation struct {
	Intent        string          `json:"intent"`
	Arguments     json.RawMessage `json:"arguments"`
	Confidence    string          `json:"confidence"`
	Clarification *string         `json:"clarification"`
}
type Create struct {
	Recurrence      *Recurrence `json:"recurrence,omitempty"`
	TimeAmbiguous   bool        `json:"time_ambiguous,omitempty"`
	Personal        bool        `json:"personal,omitempty"`
	Invite          bool        `json:"invite,omitempty"`
	AllWithTag      bool        `json:"all_with_tag,omitempty"`
	Friends         []string    `json:"friends,omitempty"`
	FriendTag       string      `json:"friend_tag,omitempty"`
	Calendar        string      `json:"calendar,omitempty"`
	Conference      string      `json:"conference,omitempty"`
	Title           string      `json:"title"`
	Date            *Temporal   `json:"date,omitempty"`
	Time            *string     `json:"time,omitempty"`
	DurationMinutes *int        `json:"duration_minutes,omitempty"`
	ReminderMinutes *int        `json:"reminder_minutes,omitempty"`
	ReminderEnabled *bool       `json:"reminder_enabled,omitempty"`
}
type Filter struct {
	Conference string    `json:"conference,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Query      string    `json:"query,omitempty"`
	Date       *Temporal `json:"date,omitempty"`
	Period     *Temporal `json:"period,omitempty"`
	Status     *string   `json:"status,omitempty"`
}
type Read struct {
	Filter
	StrictKind bool `json:"strict_kind,omitempty"`
	Next       bool `json:"next,omitempty"`
	Inherit    bool `json:"inherit,omitempty"`
}
type Target struct {
	Type   string  `json:"type"`
	Value  string  `json:"value,omitempty"`
	Index  int     `json:"index,omitempty"`
	Filter *Filter `json:"filter,omitempty"`
}
type Changes struct {
	Occurrence      *Temporal   `json:"occurrence,omitempty"`
	Scope           string      `json:"scope,omitempty"`
	Recurrence      *Recurrence `json:"recurrence,omitempty"`
	ClearRecurrence bool        `json:"clear_recurrence,omitempty"`
	TimeAmbiguous   bool        `json:"time_ambiguous,omitempty"`
	DurationMinutes *int        `json:"duration_minutes,omitempty"`
	Title           *string     `json:"title,omitempty"`
	Date            *Temporal   `json:"date,omitempty"`
	Time            *string     `json:"time,omitempty"`
	ShiftMinutes    *int        `json:"shift_minutes,omitempty"`
	ReminderMinutes *int        `json:"reminder_minutes,omitempty"`
	ReminderEnabled *bool       `json:"reminder_enabled,omitempty"`
	Status          *string     `json:"status,omitempty"`
}
type Mutation struct {
	Target Target `json:"target"`
}
type Update struct {
	Target  Target  `json:"target"`
	Changes Changes `json:"changes"`
}
type MeetingParticipants struct {
	Target    Target   `json:"target"`
	Friends   []string `json:"friends"`
	FriendTag string   `json:"friend_tag,omitempty"`
}
type Batch struct {
	Filter  Filter   `json:"filter"`
	Exclude []Filter `json:"exclude,omitempty"`
}
type BatchUpdate struct {
	Filter  Filter   `json:"filter"`
	Exclude []Filter `json:"exclude,omitempty"`
	Changes Changes  `json:"changes"`
}
type Continue struct {
	Changes Changes `json:"changes"`
}
type SchedulingReply struct {
	Option     int     `json:"option,omitempty"`
	More       bool    `json:"more,omitempty"`
	NewMeeting bool    `json:"new_meeting,omitempty"`
	Confirm    bool    `json:"confirm,omitempty"`
	Changes    Changes `json:"changes,omitempty"`
}
type Chat struct {
	Reply string `json:"reply"`
}

// Social is an operation-specific contract. Names/tags are resolved only
// against the authenticated user's confirmed friends by Go.
type Social struct {
	Code            string       `json:"code,omitempty"`
	Friends         []string     `json:"friends,omitempty"`
	Tag             string       `json:"tag,omitempty"`
	Query           string       `json:"query,omitempty"`
	Date            *Temporal    `json:"date,omitempty"`
	Period          *Temporal    `json:"period,omitempty"`
	Time            *string      `json:"time,omitempty"`
	DurationMinutes *int         `json:"duration_minutes,omitempty"`
	Status          string       `json:"status,omitempty"`
	Options         []SocialTime `json:"options,omitempty"`
	Option          int          `json:"option,omitempty"`
}
type SocialTime struct {
	Date *Temporal `json:"date"`
	Time *string   `json:"time"`
}
type Friend struct {
	Name string   `json:"name"`
	Tags []string `json:"tags,omitempty"`
}

// Strict rejects extra fields recursively, including id/target_id at any depth.
func Strict(raw []byte, value any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("missing JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func Decode(raw []byte) (Operation, error) {
	var op Operation
	if len(raw) > 65536 {
		return op, errors.New("operation too large")
	}
	if err := Strict(raw, &op); err != nil {
		return op, err
	}
	_, err := op.Args()
	return op, err
}
func (o Operation) Args() (any, error) {
	if o.Confidence != "high" && o.Confidence != "medium" && o.Confidence != "low" {
		return nil, errors.New("invalid confidence")
	}
	if o.Clarification != nil && len([]rune(*o.Clarification)) > 500 {
		return nil, errors.New("clarification too long")
	}
	var v any
	switch o.Intent {
	case "focus_action":
		v = &ActionFocus{}
	case "multi_action":
		v = &MultiAction{}
	case "resume_action":
		v = &ResumeAction{}
	case "friend_request", "friend_requests", "friend_availability", "invite_friends", "respond_invitation", "propose_reschedule", "resolve_reschedule":
		v = &Social{}
	case "choose_time":
		v = &SchedulingReply{}
	case "meeting_zoom":
		v = &Social{}
	case "set_meeting_participants":
		v = &MeetingParticipants{}
	case "create_task", "create_meeting", "create_reminder":
		v = &Create{}
	case "search", "list":
		v = &Read{}
	case "update":
		v = &Update{}
	case "complete", "delete", "select":
		v = &Mutation{}
	case "batch_update":
		v = &BatchUpdate{}
	case "batch_complete", "batch_delete":
		v = &Batch{}
	case "continue":
		v = &Continue{}
	case "chat":
		v = &Chat{}
	case "clarify", "confirm", "cancel":
		v = &struct{}{}
	default:
		return nil, errors.New("unknown intent")
	}
	if err := Strict(o.Arguments, v); err != nil {
		return nil, err
	}
	if m, ok := v.(*MultiAction); ok {
		if err := m.Validate(); err != nil {
			return nil, err
		}
	}
	if r, ok := v.(*ResumeAction); ok {
		if !actionKey.MatchString(r.Key) || r.Operation.Intent == "multi_action" || r.Operation.Intent == "resume_action" {
			return nil, errors.New("invalid action continuation")
		}
		if _, err := r.Operation.Args(); err != nil {
			return nil, err
		}
	}
	if chat, ok := v.(*Chat); ok && (strings.TrimSpace(chat.Reply) == "" || len([]rune(chat.Reply)) > 1000) {
		return nil, errors.New("invalid chat reply")
	}
	return v, nil
}

type ActionFocus struct {
	Key string `json:"key"`
}

// Item exposes display indices, never database identifiers.
type Item struct {
	Recurrence *Recurrence `json:"recurrence,omitempty"`
	Index      int         `json:"index,omitempty"`
	Kind       string      `json:"kind"`
	Title      string      `json:"title"`
	Date       string      `json:"date,omitempty"`
	Time       string      `json:"time,omitempty"`
	Status     string      `json:"status"`
}
type Pending struct {
	Recurrence   *Recurrence `json:"recurrence,omitempty"`
	Intent       string      `json:"intent"`
	Expected     string      `json:"expected"`
	Missing      []string    `json:"missing,omitempty"`
	Title        string      `json:"title,omitempty"`
	Date         string      `json:"date,omitempty"`
	Time         string      `json:"time,omitempty"`
	Candidates   []Item      `json:"candidates,omitempty"`
	Confirmation bool        `json:"confirmation"`
	Filter       *Filter     `json:"filter,omitempty"`
	Changes      *Changes    `json:"changes,omitempty"`
	Exclude      []Filter    `json:"exclude,omitempty"`
}
type SearchSummary struct {
	Conference string `json:"conference,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Query      string `json:"query,omitempty"`
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
	Status     string `json:"status,omitempty"`
}
type Context struct {
	Actions        []ActionContext `json:"actions,omitempty"`
	ActiveAction   string          `json:"active_action,omitempty"`
	OfferedSlots   []SocialTime    `json:"offered_slots,omitempty"`
	MeetingChoices []Item          `json:"meeting_choices,omitempty"`
	Friends        []Friend        `json:"friends,omitempty"`
	SocialPending  *Operation      `json:"social_pending,omitempty"`
	Now            string          `json:"now"`
	Timezone       string          `json:"timezone"`
	LastAction     string          `json:"last_action,omitempty"`
	LastResults    []Item          `json:"last_results,omitempty"`
	LastAffected   *Item           `json:"last_affected,omitempty"`
	LastSearch     *SearchSummary  `json:"last_search,omitempty"`
	Pending        *Pending        `json:"pending,omitempty"`
}

type ActionContext struct {
	AtomicGroup string     `json:"atomic_group,omitempty"`
	Operation   *Operation `json:"operation,omitempty"`
	Key         string     `json:"key"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Intent      string     `json:"intent"`
	DependsOn   []string   `json:"depends_on,omitempty"`
}
