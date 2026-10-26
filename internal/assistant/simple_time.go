package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

var bareHour = regexp.MustCompile(`(?:^|[\s,;])(?:в|на)\s+(одиннадцать|двенадцать|восемь|девять|десять|четыре|три|два|пять|шесть|семь|час|1[012]|[1-9])(?:\s+час(?:а|ов)?)?(?:$|[\s,;.!?])`)
var clockQualifier = regexp.MustCompile(`^(?:\s+час(?:а|ов)?)?(?:\s+(?:пятнадцать|тридцать|сорок\s+пять|[0-5]?[0-9]))?\s+(?:утра|утром|дня|днём|днем|вечера|вечером|ночи|ночью)(?:$|[\s,.!?])`)
var clockPrefix = regexp.MustCompile(`(?:утром|днём|днем|вечером|ночью|с\s+утра)(?:\s|[\p{L}])*`)
var halfClock = regexp.MustCompile(`(?:^|[\s,;])(?:в\s+)?(?:половин[аеуы]\s+|пол)(первого|второго|третьего|четвертого|четвёртого|пятого|шестого|седьмого|восьмого|девятого|десятого|одиннадцатого|двенадцатого)(?:$|[\s,;.!?])`)
var explicitClockTail = regexp.MustCompile(`^[:.\-][0-5][0-9](?:$|[\s,;.!?])`)

const sourceHour = `(?:[01]?[0-9]|2[0-3]|час|один|два|три|четыре|пять|шесть|семь|восемь|девять|десять|одиннадцать|двенадцать|тринадцать|четырнадцать|пятнадцать|шестнадцать|семнадцать|восемнадцать|девятнадцать|двадцать(?:\s+(?:один|два|три))?|полдень|полночь)`

var recurringClockSource = regexp.MustCompile(`(?:^|[\s,;])(?:[012]?[0-9][:.][0-5][0-9]|(?:в|на|к|с)\s+` + sourceHour + `|` + sourceHour + `\s+(?:утра|вечера|ночи|дня))(?:$|[\s,;.!?])`)

// Withhold a model-supplied clock when a new calendar recurrence has no clock
// expression in its source. This guard can only ask a question, never invent a
// time. Elapsed timers require no wall clock.
func guardRecurringClock(args any, message string) {
	create, ok := args.(*nlu.Create)
	if !ok || create.Recurrence == nil || create.Recurrence.Frequency == "minute" || create.Recurrence.Frequency == "hour" || create.Time == nil {
		return
	}
	text := strings.ToLower(message)
	if !recurringClockSource.MatchString(text) && !halfClock.MatchString(text) {
		create.Time = nil
	}
}

// This is a conservative ambiguity guard, not an intent parser. It can only
// withhold an unqualified 12-hour clock; it cannot schedule or authorize work.
func ambiguousHours(message string) []string {
	low := strings.ToLower(message)
	indices := bareHour.FindStringSubmatchIndex(low)
	half := false
	if indices == nil {
		indices = halfClock.FindStringSubmatchIndex(low)
		half = indices != nil
	}
	if indices == nil {
		return nil
	}
	hour := low[indices[2]:indices[3]]
	full := low[indices[0]:indices[1]]
	tail := low[indices[3]:]
	prefix := low[:indices[0]]
	if split := strings.LastIndexAny(prefix, ",;"); split >= 0 {
		prefix = prefix[split+1:]
	}
	if explicitClockTail.MatchString(tail) || clockQualifier.MatchString(tail) || clockPrefix.MatchString(prefix) {
		return nil
	}
	clause := strings.TrimSpace(full)
	if strings.HasPrefix(clause, "на ") {
		tail := strings.TrimSpace(tail)
		if hour == "час" || strings.HasPrefix(tail, "минут") || strings.HasPrefix(tail, "секунд") || strings.HasPrefix(tail, "час") {
			return nil
		}
	}
	h, _ := strconv.Atoi(hour)
	if h == 0 {
		h = map[string]int{"час": 1, "два": 2, "три": 3, "четыре": 4, "пять": 5, "шесть": 6, "семь": 7, "восемь": 8, "девять": 9, "десять": 10, "одиннадцать": 11, "двенадцать": 12}[hour]
	}
	minute := 0
	if half {
		h = map[string]int{"первого": 1, "второго": 2, "третьего": 3, "четвертого": 4, "четвёртого": 4, "пятого": 5, "шестого": 6, "седьмого": 7, "восьмого": 8, "девятого": 9, "десятого": 10, "одиннадцатого": 11, "двенадцатого": 12}[hour] - 1
		minute = 30
	}
	return []string{fmt.Sprintf("%02d:%02d", h%12, minute), fmt.Sprintf("%02d:%02d", h%12+12, minute)}
}

func (s *Service) applySimple(ctx context.Context, tx repository.Tx, state *domain.State, op nlu.Operation, message string, now time.Time) (domain.Command, domain.Response, error) {
	args, _ := op.Args()
	options := ambiguousHours(message)
	var ambiguous bool
	mark := func(ch *nlu.Changes) {
		ambiguous = ch.TimeAmbiguous || ch.Time != nil && len(options) > 0
		if ambiguous {
			ch.Time, ch.TimeAmbiguous = nil, true
		}
	}
	switch a := args.(type) {
	case *nlu.Create:
		ambiguous = a.TimeAmbiguous || len(options) > 0 && (a.Date == nil || !in(a.Date.Type, "relative_delay", "absolute_time"))
		if ambiguous {
			a.Time, a.TimeAmbiguous = nil, true
		}
	case *nlu.Continue:
		mark(&a.Changes)
	case *nlu.Update:
		mark(&a.Changes)
	case *nlu.SchedulingReply:
		mark(&a.Changes)
	}
	if ambiguous {
		op.Arguments, _ = json.Marshal(args)
	}
	c, r, err := s.applySimpleOperation(ctx, tx, state, op, message, now)
	if ambiguous && err == nil && state.Pending != nil && state.Pending.RequiredSlot == "time" {
		r.Reply = "Укажите точное время в формате ЧЧ:ММ или назовите часть суток."
		if len(options) == 2 {
			r.Reply = fmt.Sprintf("Вы имеете в виду %s или %s?", options[0], options[1])
			r.Suggestions = options
		}
	}
	return c, r, err
}
