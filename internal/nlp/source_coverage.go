package nlp

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Source constraints are intentionally a closed lexical inventory. They do not
// identify commands, extract titles or run the legacy parser. Unsupported
// language stays with the model and its evidence contract.
type sourceConstraint struct{ field, span string }

var exclusionMarkers = []string{"кроме", "за исключением", "исключая", "не трогай", "оставь как есть"}

func sourceExclusionStart(source string) (int, int) {
	start, length := -1, 0
	for _, marker := range exclusionMarkers {
		for _, span := range sourceSpans(source, marker) {
			i := len(string([]rune(source)[:span[0]]))
			if start < 0 || i < start {
				start, length = i, len(marker)
			}
		}
	}
	return start, length
}

var coverageDay = regexp.MustCompile(`(?i)сегодня|послезавтра|завтра|позавчера|вчера`)
var coverageClock = regexp.MustCompile(`(?:[01]?[0-9]|2[0-3]):[0-5][0-9]`)
var coverageDelay = regexp.MustCompile(`(?i)через\s+[1-9][0-9]*\s+(?:минут(?:у|ы)?|час(?:а|ов)?|день|дня|дней)`)
var coverageShift = regexp.MustCompile(`(?i)на\s+(?:[1-9][0-9]*|один|одну|два|две|три|четыре|пять|полчаса|час)\s*(?:минут(?:у|ы)?|час(?:а|ов)?|день|дня|дней)?\s+(?:позже|раньше|вперед|назад)`)
var coverageStatus = regexp.MustCompile(`(?i)заверш[её]нные|заверш[её]нных|выполненные|выполненных|открытые|открытых|незаверш[её]нные|незаверш[её]нных|невыполненные|невыполненных`)
var coverageTimerAmount = regexp.MustCompile(`(?i)(?:[1-9][0-9]*\s+(?:минут(?:у|ы)?|час(?:а|ов)?)|полчаса)`)
var explicitNegation = regexp.MustCompile(`(?i)(?:^|[^\p{L}])не\s+(?:на\s+)?$`)
var quotedContent = regexp.MustCompile(`«[^»]*»|"[^"]*"`)

func positiveSpans(source string, re *regexp.Regexp) []string {
	var result []string
	for _, at := range re.FindAllStringIndex(source, -1) {
		span := source[at[0]:at[1]]
		// ExactSourceSpan protects tomorrow vs day-after-tomorrow and numbers.
		if !ExactSourceSpan(source, span) {
			continue
		}
		prefix := source[:at[0]]
		if explicitNegation.MatchString(prefix) {
			continue
		}
		result = append(result, span)
	}
	return result
}

func sourceConstraints(p SemanticPlan, source string) []sourceConstraint {
	op := semanticString(p.Operation)
	if p.Status != "ok" || op != "create" && !IsPlanMutation(op) {
		return nil
	}
	// Quoted titles are content, not scheduling instructions. Exclusions below
	// use the original source because their quoted names are constraints.
	text := quotedContent.ReplaceAllString(source, " ")
	main := text
	if i, _ := sourceExclusionStart(main); i >= 0 {
		main = main[:i]
	}
	var out []sourceConstraint
	add := func(field, span string) { out = append(out, sourceConstraint{field, span}) }
	for _, day := range positiveSpans(main, coverageDay) {
		field := "filters.period"
		if op == "create" {
			field = "schedule.date"
		} else if p.DialogueAct == "correction" || op == "update" || op == "batch_update" {
			// Only the explicit destination preposition changes the role for a
			// batch. An unmarked day on a time/shift update selects the source.
			if p.DialogueAct == "correction" || ExactSourceSpan(folded(main), "на "+folded(day)) || !strings.HasPrefix(op, "batch_") && (p.Filters == nil || p.Filters.Period == nil) {
				field = "changes.date"
			}
		}
		add(field, day)
	}
	for _, clock := range positiveSpans(main, coverageClock) {
		field := "changes.time"
		if op == "create" {
			field = "schedule.time"
		}
		if ExactSourceSpan(main, "после "+clock) {
			field = "filters.time_from"
		}
		if ExactSourceSpan(main, "до "+clock) {
			field = "filters.time_before"
		}
		add(field, clock)
	}
	for _, delay := range positiveSpans(main, coverageDelay) {
		field := "changes.delay"
		if op == "create" {
			field = "schedule.delay"
		}
		add(field, delay)
	}
	for _, shift := range positiveSpans(main, coverageShift) {
		add("changes.time_shift_minutes", shift)
	}
	for _, status := range positiveSpans(main, coverageStatus) {
		add("filters.status", status)
	}
	for n := 1; n <= 8; n++ {
		if HasOrdinalEvidence(main, n, 8) {
			add("target.ordinal", strconv.Itoa(n))
		}
	}
	if i, _ := sourceExclusionStart(text); i >= 0 {
		add("filters.exclude", source)
	}
	return out
}

func planValue(p SemanticPlan, field string) any {
	raw, _ := json.Marshal(p)
	var v any
	_ = json.Unmarshal(raw, &v)
	for _, part := range strings.Split(field, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

// ValidateSourceCoverage checks SOURCE -> PLAN, including create schedules.
// A missing constraint is never filled in: the entire turn fails closed.
func ValidateSourceCoverage(p SemanticPlan, source string) error {
	if p.Status == "ok" && semanticString(p.Operation) == "create" && semanticString(p.Kind) == "reminder" && explicitTimerMarker(source) {
		for _, span := range positiveSpans(quotedContent.ReplaceAllString(source, " "), coverageTimerAmount) {
			n, u, ok := explicitAmount(span)
			if folded(span) == "полчаса" {
				n, u, ok = 30, "minute", true
			}
			if !ok {
				continue
			}
			if p.Schedule == nil || p.Schedule.Delay == nil {
				return evidenceFailure("schedule.delay", "source_constraint_omitted")
			}
			seconds := func(n int, u string) int {
				switch u {
				case "second":
					return n
				case "minute":
					return n * 60
				case "hour":
					return n * 3600
				}
				return -1
			}
			d := p.Schedule.Delay
			if d.Type != "relative_delay" || d.Value == nil || seconds(n, u) != seconds(*d.Value, semanticString(d.Unit)) {
				return evidenceFailure("schedule.delay", "value_mismatch")
			}
		}
	}
	for _, c := range sourceConstraints(p, source) {
		v := planValue(p, c.field)
		if v == nil || v == "any" {
			return evidenceFailure(c.field, "source_constraint_omitted")
		}
		if a, ok := v.([]any); ok && len(a) == 0 {
			return evidenceFailure(c.field, "source_constraint_omitted")
		}
		if c.field == "target.ordinal" {
			n, _ := strconv.Atoi(c.span)
			if int(v.(float64)) != n || p.Target == nil || semanticString(p.Target.Reference) != "ordinal" {
				return evidenceFailure(c.field, "value_mismatch")
			}
		} else if strings.HasSuffix(c.field, ".date") || c.field == "filters.period" || strings.HasSuffix(c.field, ".delay") {
			b, _ := json.Marshal(v)
			var t TemporalMeaning
			_ = json.Unmarshal(b, &t)
			if err := checkTemporalEvidence(c.field, t, c.span); err != nil {
				return err
			}
		} else if strings.HasSuffix(c.field, ".time") || strings.HasSuffix(c.field, ".time_from") || strings.HasSuffix(c.field, ".time_before") {
			b, _ := json.Marshal(v)
			var t ClockMeaning
			_ = json.Unmarshal(b, &t)
			if err := checkClockEvidence(c.field, t.Hour, t.Minute, c.span); err != nil {
				return err
			}
		} else if c.field == "changes.time_shift_minutes" {
			n, ok := explicitShift(c.span)
			if !ok || n != int(v.(float64)) {
				return evidenceFailure(c.field, "value_mismatch")
			}
		} else if c.field == "filters.status" {
			wanted := "done"
			if strings.HasPrefix(folded(c.span), "открыт") || strings.HasPrefix(folded(c.span), "не") {
				wanted = "open"
			}
			if v != wanted {
				return evidenceFailure(c.field, "value_mismatch")
			}
		}
	}
	return nil
}

// GoSourceAnchor supplies proof only when a populated value has an obvious
// current-source anchor. Model-supplied proofs are still validated independently.
func GoSourceAnchor(p SemanticPlan, field, source string) string {
	for _, c := range sourceConstraints(p, source) {
		if c.field == field && field != "target.ordinal" && field != "filters.exclude" {
			return c.span
		}
	}
	value := planValue(p, field)
	if s, ok := value.(string); ok && (strings.HasSuffix(field, ".query") || field == "query" || field == "changes.title") {
		text := source
		if strings.HasPrefix(field, "filters.exclude.") {
			if i, n := sourceExclusionStart(source); i >= 0 {
				text = source[i+n:]
			} else {
				return ""
			}
		}
		for _, span := range sourceSpans(text, s) {
			return string([]rune(text)[span[0]:span[1]])
		}
	}
	if (field == "target.reference" || field == "target.ordinal") && p.Target != nil && p.Target.Ordinal != nil && HasOrdinalEvidence(source, *p.Target.Ordinal, 8) {
		return source
	}
	return ""
}
