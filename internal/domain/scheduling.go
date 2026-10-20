package domain

import (
	"strings"
	"time"
)

type PlanningSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type ChatChoice struct {
	Token  string        `json:"token"`
	Action string        `json:"action"`
	Index  int           `json:"index,omitempty"`
	Slot   *PlanningSlot `json:"slot,omitempty"`
}
type SchedulingPending struct {
	Preview      bool           `json:"preview,omitempty"`
	SlotChosen   bool           `json:"slot_chosen,omitempty"`
	SearchEnd    *time.Time     `json:"search_end,omitempty"`
	ConfirmedNew bool           `json:"confirmed_new,omitempty"`
	Token        string         `json:"token"`
	Slots        []PlanningSlot `json:"slots,omitempty"`
	Requested    *PlanningSlot  `json:"requested,omitempty"`
	After        *time.Time     `json:"after,omitempty"`
	Meetings     []Event        `json:"meetings,omitempty"`
	Selected     *Event         `json:"selected,omitempty"`
}
type SchedulingCard struct {
	Token     string         `json:"token"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title"`
	Requested *PlanningSlot  `json:"requested,omitempty"`
	Slots     []PlanningSlot `json:"slots,omitempty"`
	Meetings  []Event        `json:"meetings,omitempty"`
	Complete  bool           `json:"complete"`
	Timezone  string         `json:"timezone"`
	Today     string         `json:"today"`
}

func GenericMeetingTitle(title string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(title), ".!")) {
	case "", "встреча", "созвон", "zoom", "zoom-встреча", "встреча в zoom", "встреча с друзьями":
		return true
	}
	return false
}

// InstrumentalName formats Russian display names without involving NLU or IDs.
// Unrecognized and non-Russian names retain their original spelling.
func InstrumentalName(name string) string {
	parts := strings.Fields(name)
	for i, word := range parts {
		lower := strings.ToLower(word)
		if !strings.ContainsAny(lower, "абвгдежзийклмнопрстуфхцчшщыьэюяё") {
			continue
		}
		r := []rune(word)
		if len(r) < 2 {
			continue
		}
		switch {
		case strings.HasSuffix(lower, "ова"), strings.HasSuffix(lower, "ева"), strings.HasSuffix(lower, "ина"):
			word = string(r[:len(r)-1]) + "ой"
		case strings.HasSuffix(lower, "ский"), strings.HasSuffix(lower, "цкий"), strings.HasSuffix(lower, "ый"), strings.HasSuffix(lower, "ой"):
			word = string(r[:len(r)-2]) + "ым"
		case strings.HasSuffix(lower, "ий"):
			word = string(r[:len(r)-2]) + "ием"
		case strings.HasSuffix(lower, "ей"), strings.HasSuffix(lower, "ь"):
			word = string(r[:len(r)-1]) + "ем"
		case strings.HasSuffix(lower, "а"):
			word = string(r[:len(r)-1]) + "ой"
		case strings.HasSuffix(lower, "я"):
			word = string(r[:len(r)-1]) + "ей"
		case strings.HasSuffix(lower, "ов"), strings.HasSuffix(lower, "ев"), strings.HasSuffix(lower, "ин"):
			word += "ым"
		case strings.ContainsRune("бвгджзклмнпрстфхцчшщ", r[len(r)-1]):
			word += "ом"
		}
		parts[i] = word
	}
	return strings.Join(parts, " ")
}
func MeetingDisplayTitle(title string, names []string) string {
	if !GenericMeetingTitle(title) || len(names) == 0 {
		return title
	}
	labels := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" {
			labels = append(labels, InstrumentalName(name))
		}
	}
	if len(labels) == 0 {
		return title
	}
	return "Встреча с " + strings.Join(labels, ", ")
}
func (e Event) Label() string {
	if e.DisplayTitle != "" {
		return e.DisplayTitle
	}
	return e.Title
}
