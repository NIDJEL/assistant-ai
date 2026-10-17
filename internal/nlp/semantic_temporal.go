package nlp

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func parseSemanticID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid explicit target ID")
	}
	return id, nil
}

// These are temporal grammar aliases, never an intent/title rewrite.
func temporalAlias(raw string) string {
	text := NormalizeText(raw).Analysis
	for _, pair := range [][2]string{
		{`завтрашн\w*`, "завтра"}, {`сегодняшн\w*`, "сегодня"}, {`вчерашн\w*`, "вчера"},
		{`полдевятого`, "половину девятого"}, {`полдесятого`, "половину десятого"},
		{`полчасика`, "полчаса"}, {`часов\s+на\s+`, "в "},
		{`часов\s+в\s+`, "в "},
		{`\b(минут(?:у|ы)?|час(?:а|ов)?|секунд(?:у|ы)?)\s+через\s+(` + relativeAmount + `)\b`, "через $2 $1"},
		{`пятницу`, "пятница"}, {`субботу`, "суббота"}, {`среду`, "среда"},
		{`(\d+)\s+сек($|[^\p{L}])`, "${1} секунд${2}"},
	} {
		text = regex(pair[0]).ReplaceAllString(text, pair[1])
	}
	return spaces(text)
}

func groundedExpression(expression, original string) bool {
	if groundedTimerExpression(expression, original) {
		return true
	}
	key := func(raw string) string {
		raw = temporalAlias(raw)
		raw = replace(`^(?:на|в|во|к)\s+`, raw, "")
		return strings.Trim(raw, " ,.!?:;-")
	}
	expression = key(expression)
	// Boundaries prevent "в 9" being grounded by "в 19".
	// Keep a clock's colon and numeric date punctuation intact for exact spans.
	direct := strings.Trim(temporalAlias(original), " ,.!?:;-")
	original = regex(`[,.!?:;«»"]`).ReplaceAllString(temporalAlias(original), " ")
	return expression != "" && (regex(`(?:^|[^\p{L}\p{N}:./-])`+regexp.QuoteMeta(expression)+`(?:$|[^\p{L}\p{N}:./-])`).MatchString(direct) || strings.Contains(" "+spaces(original)+" ", " "+expression+" "))
}

func clockExpression(raw string) string {
	raw = temporalAlias(raw)
	raw = replace(`^на\s+`, raw, "")
	if !has(`^(?:в|к|через)\b`, raw) {
		raw = "в " + raw
	}
	return raw
}

// A model may select one of two conflicting temporal expressions. Validate the
// original scheduling spans too, without invoking intent or title extraction.
func validateSemanticCreationSource(text string, now time.Time) string {
	return ParseDateTime(withoutReminderSettings(scheduleOnly(text)), now).Error
}

func semanticTemporal(date, clock, relative, period *string, original string, now time.Time) (Temporal, error) {
	t := Temporal{Duration: time.Hour}
	for _, p := range []*string{date, clock, relative, period} {
		if p != nil && !groundedExpression(*p, original) {
			return t, errors.New("ungrounded temporal expression")
		}
	}
	// Parse roles separately; a target clock must never become a destination,
	// and a list period must never become an event date.
	if date != nil {
		d := ParseDateTime(temporalAlias(*date), now)
		if d.Date == nil && d.Start == nil && d.Error == "" {
			return t, errors.New("unsupported date expression")
		}
		if d.Time != nil || d.Recurrence != nil || d.Relative {
			return t, errors.New("date expression contains another temporal role")
		}
		t.Date, t.Start, t.End, t.Error = d.Date, d.Start, d.End, d.Error
	}
	if period != nil {
		p := ParseDateTime(temporalAlias(*period), now)
		if p.Start == nil && p.Error == "" {
			return t, errors.New("unsupported period expression")
		}
		if p.Time != nil || p.Recurrence != nil || p.Relative {
			return t, errors.New("period expression contains another temporal role")
		}
		if t.Start != nil && p.Start != nil && (t.Start.String() != p.Start.String() || t.End.String() != p.End.String()) {
			return t, errors.New("conflicting date and period expressions")
		}
		t.Start, t.End = p.Start, p.End
		if p.Error != "" {
			t.Error = p.Error
		}
	}
	if clock != nil {
		c := ParseDateTime(clockExpression(*clock), now)
		if c.Time == nil && c.Error == "" {
			return t, errors.New("unsupported time expression")
		}
		if c.Relative || c.Date != nil || c.Recurrence != nil {
			return t, errors.New("time expression contains another temporal role")
		}
		// Detect a second clock instead of silently using the first one.
		if rest := Extract(c.Clean(clockExpression(*clock)), now); rest.Time != nil {
			return t, errors.New("multiple clocks in time expression")
		}
		t.Time, t.StartSecond = c.Time, c.StartSecond
		if c.Error != "" {
			t.Error = c.Error
		}
	}
	if relative != nil {
		r := ParseDateTime(temporalAlias(timerRelative(*relative, original)), now)
		if !r.Relative && r.Error == "" {
			return t, errors.New("unsupported relative time expression")
		}
		if clock != nil || period != nil {
			return t, errors.New("relative time conflicts with clock/period")
		}
		if t.Date != nil && r.Date != nil && t.Date.String() != r.Date.String() {
			return t, errors.New("relative time conflicts with date")
		}
		t.Date, t.Time, t.Start, t.End = r.Date, r.Time, r.Start, r.End
		t.At, t.Relative, t.StartSecond = r.At, r.Relative, r.StartSecond
		if r.Error != "" {
			t.Error = r.Error
		}
	}
	return t, nil
}
