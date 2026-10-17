package nlp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// EvidenceError is safe structured instrumentation: it contains no user text.
type EvidenceError struct{ Field, Reason string }

func (e *EvidenceError) Error() string { return "semantic evidence: " + e.Field + ": " + e.Reason }

func evidenceFailure(field, reason string) error { return &EvidenceError{field, reason} }

var negatedDayProof = regexp.MustCompile(`(?i)^\s*не\s+(?:на\s+)?(?:сегодня|завтра|послезавтра|вчера|позавчера)\s*[.!?,;]?\s*$`)

func folded(s string) string { return strings.ReplaceAll(strings.ToLower(s), "ё", "е") }
func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

// ExactSourceSpan matches Unicode case-insensitively while preserving complete
// lexical boundaries and exact whitespace. Rune indices avoid byte offsets after
// case folding (for example Kelvin sign / K).
func sourceSpans(source, span string) [][2]int {
	text, proof := []rune(source), []rune(span)
	var spans [][2]int
	if strings.TrimSpace(span) == "" {
		return spans
	}
	for i := 0; i+len(proof) <= len(text); i++ {
		end := i + len(proof)
		if !strings.EqualFold(string(text[i:end]), span) {
			continue
		}
		if i > 0 && wordRune(proof[0]) && wordRune(text[i-1]) {
			continue
		}
		if end < len(text) && wordRune(proof[len(proof)-1]) && wordRune(text[end]) {
			continue
		}
		spans = append(spans, [2]int{i, end})
	}
	return spans
}
func ExactSourceSpan(source, span string) bool { return len(sourceSpans(source, span)) > 0 }

// ValidatePlanEvidence is a bounded entailment guard, not an intent parser.
// Only explicit lexical invariants are interpreted here; all other language
// remains with ParsePlan. Contradictions never get silently repaired into a
// write. The caller must clarify before grounding or producing a preview.
func ValidatePlanEvidence(p SemanticPlan, source string) error {
	if p.Status != "ok" {
		return nil
	}
	op := semanticString(p.Operation)
	if err := ValidateSourceCoverage(p, source); err != nil {
		return err
	}
	raw, _ := json.Marshal(p)
	var tree map[string]any
	_ = json.Unmarshal(raw, &tree)
	valueAt := func(field string) any {
		if field == "batch.scope" && strings.HasPrefix(op, "batch_") {
			return true
		}
		var value any = tree
		for _, part := range strings.Split(field, ".") {
			switch x := value.(type) {
			case map[string]any:
				value = x[part]
			case []any:
				i, err := strconv.Atoi(part)
				if err != nil || i < 0 || i >= len(x) {
					return nil
				}
				value = x[i]
			default:
				return nil
			}
		}
		return value
	}
	evidence := map[string]string{}
	for _, e := range p.Evidence {
		if EvidencePolicy(e.Field, op, valueAt(e.Field)) != Required {
			continue
		}
		// A quoted negative alternative is not a second supporting proof.
		// Coverage has already checked the selected positive day against p.
		if oneOf(e.Field, "changes.date", "filters.period") && negatedDayProof.MatchString(e.Text) && ExactSourceSpan(source, e.Text) {
			continue
		}
		if _, exists := evidence[e.Field]; exists {
			return evidenceFailure(e.Field, "duplicate")
		}
		if !ExactSourceSpan(source, e.Text) {
			return evidenceFailure(e.Field, "not_in_original")
		}
		evidence[e.Field] = e.Text
	}
	if !IsPlanMutation(op) {
		return nil
	}
	require := func(field string) (string, error) {
		x, ok := evidence[field]
		if !ok {
			// A proof attached to a nonexistent branch is not a substitute
			// for proof of a populated constraint (field-substitution attack).
			leaf := field[strings.LastIndex(field, ".")+1:]
			for _, e := range p.Evidence {
				if strings.HasSuffix(e.Field, "."+leaf) && valueAt(e.Field) == nil {
					return "", evidenceFailure(field, "missing")
				}
			}
			if anchor := GoSourceAnchor(p, field, source); anchor != "" {
				return anchor, nil
			}
			return "", evidenceFailure(field, "missing")
		}
		return x, nil
	}
	// Walk populated leaves so exclusions, clock bounds, priorities and reminder
	// changes cannot bypass provenance by being placed in a different DTO branch.
	var walk func(string, any) error
	walk = func(path string, v any) error {
		if v == nil {
			return nil
		}
		if path == "target" {
			t := p.Target
			if semanticString(t.Reference) == "query" {
				span, err := require("target.query")
				if err != nil {
					return err
				}
				if !ExactSourceSpan(folded(span), folded(*t.Query)) {
					return evidenceFailure("target.query", "value_mismatch")
				}
				return nil
			}
			proofField := "target.reference"
			if t.Ordinal != nil && evidence[proofField] == "" {
				proofField = "target.ordinal"
			}
			span, err := require(proofField)
			if err != nil {
				return err
			}
			if t.Ordinal != nil && !HasOrdinalEvidence(span, *t.Ordinal, 8) {
				// Genitive ordinal citations need their current-source preposition.
				withPreposition := "у " + span
				if !ExactSourceSpan(source, withPreposition) || !HasOrdinalEvidence(withPreposition, *t.Ordinal, 8) || !HasOrdinalEvidence(source, *t.Ordinal, 8) {
					return evidenceFailure("target.ordinal", "value_mismatch")
				}
			}
			if semanticString(t.Reference) == "last" {
				ok := false
				for _, w := range strings.Fields("её ее его эту этот это той ту последнее последнюю последний созданную созданный найденную найденный неё нее него ей ему лучше") {
					if ExactSourceSpan(folded(span), w) {
						ok = true
					}
				}
				if !ok {
					return evidenceFailure("target.reference", "unsupported_reference")
				}
			}
			return nil
		}
		if m, ok := v.(map[string]any); ok {
			if _, temporal := m["type"]; temporal {
				span, err := require(path)
				if err != nil {
					return err
				}
				b, _ := json.Marshal(m)
				var t TemporalMeaning
				_ = json.Unmarshal(b, &t)
				return checkTemporalEvidence(path, t, span)
			}
			if _, clock := m["hour"]; clock {
				span, err := require(path)
				if err != nil {
					return err
				}
				return checkClockEvidence(path, int(m["hour"].(float64)), int(m["minute"].(float64)), span)
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if err := walk(path+"."+k, m[k]); err != nil {
					return err
				}
			}
			return nil
		}
		if a, ok := v.([]any); ok {
			for i, x := range a {
				if err := walk(fmt.Sprintf("%s.%d", path, i), x); err != nil {
					return err
				}
			}
			return nil
		}
		if EvidencePolicy(path, op, v) != Required {
			return nil
		}
		span, err := require(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".status") || strings.HasSuffix(path, ".priority") || strings.HasSuffix(path, ".kind") {
			groups := map[string][]string{
				"open": {"открытые", "открытых", "незавершенные", "незавершенных", "невыполненные", "невыполненных"},
				"done": {"завершенные", "завершенных", "выполненные", "выполненных", "закрытые", "закрытых"},
				"high": {"важные", "важных", "высокого", "высокий"}, "low": {"низкого", "низкий"}, "normal": {"обычного", "обычный", "нормального"},
				"meeting": {"встреча", "встречу", "встречи", "встреч"}, "task": {"задача", "задачу", "задачи", "задач"}, "reminder": {"напоминание", "напоминания", "напоминаний"},
			}
			domainValues := map[string][]string{"status": {"open", "done"}, "priority": {"high", "normal", "low"}, "kind": {"task", "meeting", "reminder"}}
			parts := strings.Split(path, ".")
			for normalized, words := range groups {
				if !oneOf(normalized, domainValues[parts[len(parts)-1]]...) {
					continue
				}
				for _, word := range words {
					if ExactSourceSpan(folded(span), word) && v.(string) != normalized {
						return evidenceFailure(path, "value_mismatch")
					}
				}
			}
		}
		if path == "changes.time_shift_minutes" {
			n, ok := explicitShift(span)
			if !ok {
				return evidenceFailure(path, "unsupported_shift")
			}
			if n != int(v.(float64)) {
				return evidenceFailure(path, "value_mismatch")
			}
		}
		if strings.HasSuffix(path, ".query") || path == "query" || path == "changes.title" {
			if !ExactSourceSpan(folded(span), folded(v.(string))) {
				return evidenceFailure(path, "value_mismatch")
			}
		}
		return nil
	}
	for _, path := range []string{"target", "filters", "changes", "query"} {
		if err := walk(path, tree[path]); err != nil {
			return err
		}
	}
	if strings.HasPrefix(op, "batch_") {
		span, err := require("batch.scope")
		if err != nil {
			return err
		}
		plural := regexp.MustCompile(`(?i)(^|[^\p{L}])(все|всех|всем|встречи|встреч|встречам|задачи|задач|задачам|напоминания|напоминаний|массово)([^\p{L}]|$)`)
		if !plural.MatchString(span) {
			return evidenceFailure("batch.scope", "unsupported_scope")
		}
		if p.Kind != nil && *p.Kind != "any" {
			for _, k := range []struct{ stem, kind string }{{"встреч", "meeting"}, {"задач", "task"}, {"напоминан", "reminder"}} {
				if strings.Contains(folded(span), k.stem) && *p.Kind != k.kind {
					return evidenceFailure("batch.scope", "value_mismatch")
				}
			}
		}
		text := folded(source)
		if strings.Contains(text, "кроме") || strings.Contains(text, "за исключением") || strings.Contains(text, "не трогай") || strings.Contains(text, "оставь как есть") {
			if p.Filters == nil || len(p.Filters.Exclude) == 0 {
				return evidenceFailure("filters.exclude", "missing_exclusion")
			}
		}
		if p.Filters != nil && p.Filters.Period != nil && p.Changes != nil && p.Changes.Date != nil {
			a, b := evidence["filters.period"], evidence["changes.date"]
			if !disjointEvidence(source, a, b) {
				return evidenceFailure("changes.date", "source_destination_overlap")
			}
		}
	}
	return nil
}

func disjointEvidence(source, a, b string) bool {
	for _, left := range sourceSpans(source, a) {
		for _, right := range sourceSpans(source, b) {
			if left[1] <= right[0] || right[1] <= left[0] {
				return true
			}
		}
	}
	return false
}

var dayEvidence = map[string]int{"сегодня": 0, "завтра": 1, "послезавтра": 2, "вчера": -1, "позавчера": -2}
var weekdayEvidence = map[string]string{"понедельник": "monday", "вторник": "tuesday", "среду": "wednesday", "среда": "wednesday", "четверг": "thursday", "пятницу": "friday", "пятница": "friday", "субботу": "saturday", "суббота": "saturday", "воскресенье": "sunday"}

func checkTemporalEvidence(field string, t TemporalMeaning, span string) error {
	s := folded(span)
	found := 0
	for _, lex := range positiveSpans(s, coverageDay) {
		n := dayEvidence[lex]
		found++
		if t.Type != "relative_day" || t.Offset == nil || *t.Offset != n {
			return evidenceFailure(field, "value_mismatch")
		}
	}
	if found > 1 {
		return evidenceFailure(field, "ambiguous_span")
	}
	if found == 1 {
		return nil
	}
	if t.Type == "relative_day" {
		if _, ok := explicitShift(span); ok {
			return evidenceFailure(field, "value_mismatch")
		}
		return evidenceFailure(field, "unsupported_relative_day")
	}
	for lex, day := range weekdayEvidence {
		if ExactSourceSpan(s, lex) {
			direction := "next_or_current"
			if strings.Contains(s, "следующ") {
				direction = "next_week"
			}
			if strings.Contains(s, "прошл") || strings.Contains(s, "предыдущ") {
				direction = "previous_week"
			}
			actual := "next_or_current"
			if t.Direction != nil {
				actual = *t.Direction
			}
			if t.Type != "weekday" || semanticString(t.Weekday) != day || actual != direction {
				return evidenceFailure(field, "value_mismatch")
			}
			return nil
		}
	}
	if strings.Contains(s, "через") {
		n, u, ok := explicitAmount(s)
		if ok && (t.Type != "relative_delay" || semanticString(t.Unit) != u || t.Value == nil || *t.Value != n) {
			return evidenceFailure(field, "value_mismatch")
		}
		if ok {
			return nil
		}
	}
	if t.Type == "relative_delay" {
		return evidenceFailure(field, "unsupported_delay")
	}
	// Unsupported phrases retain the model's interpretation but must still cite
	// their exact source. We deliberately do not parse arbitrary Russian dates.
	return nil
}

var amountRE = regexp.MustCompile(`^(?:через |на |за )?(\d+|один|одну|одного|одна|два|две|двух|три|трех|треx|четыре|пять|шесть|семь|восемь|девять|десять|двадцать|тридцать)?\s*(минут(?:у|ы)?|час(?:а|ов)?|день|дня|дней)(?: (позже|раньше|вперед|назад))?$`)

func explicitAmount(span string) (int, string, bool) {
	s := strings.Join(strings.Fields(folded(span)), " ")
	s = strings.Trim(s, ".,!?")
	if oneOf(s, "через полчаса", "на полчаса", "на полчаса позже", "на полчаса раньше") {
		return 30, "minute", true
	}
	m := amountRE.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	words := map[string]int{"": 1, "один": 1, "одну": 1, "одного": 1, "одна": 1, "два": 2, "две": 2, "двух": 2, "три": 3, "трех": 3, "четыре": 4, "пять": 5, "шесть": 6, "семь": 7, "восемь": 8, "девять": 9, "десять": 10, "двадцать": 20, "тридцать": 30}
	n, ok := words[m[1]]
	if !ok {
		var err error
		n, err = strconv.Atoi(m[1])
		if err != nil {
			return 0, "", false
		}
	}
	u := "day"
	if strings.HasPrefix(m[2], "минут") {
		u = "minute"
	}
	if strings.HasPrefix(m[2], "час") {
		u = "hour"
	}
	return n, u, n > 0 && n <= 525600
}
func explicitShift(span string) (int, bool) {
	s := folded(span)
	if !strings.HasPrefix(s, "на ") {
		return 0, false
	}
	n, u, ok := explicitAmount(s)
	if !ok {
		return 0, false
	}
	if u == "hour" {
		n *= 60
	}
	if u == "day" {
		n *= 1440
	}
	if strings.Contains(s, "раньше") || strings.Contains(s, "назад") {
		n = -n
	}
	return n, true
}

var clockEvidenceRE = regexp.MustCompile(`^(?:(?:в|на|после|до) )?(\d{1,2})(?::(\d{2}))?(?: (?:часов|часа|час|утра|вечера))?$`)

func checkClockEvidence(field string, hour, minute int, span string) error {
	s := strings.Trim(strings.Join(strings.Fields(folded(span)), " "), ".,!?")
	m := clockEvidenceRE.FindStringSubmatch(s)
	if m == nil {
		return evidenceFailure(field, "unsupported_clock")
	}
	h, _ := strconv.Atoi(m[1])
	min := 0
	if m[2] != "" {
		min, _ = strconv.Atoi(m[2])
	}
	if strings.HasSuffix(s, "вечера") && h < 12 {
		h += 12
	}
	if h != hour || min != minute {
		return evidenceFailure(field, "value_mismatch")
	}
	return nil
}
