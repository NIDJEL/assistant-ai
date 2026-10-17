package nlp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func planStr(s string) *string { return &s }
func planInt(n int) *int       { return &n }
func calendarPeriod(unit string, offset int) TemporalMeaning {
	return TemporalMeaning{Type: "calendar_period", Unit: planStr(unit), Offset: planInt(offset)}
}

func TestNormalizedTemporalCalendar(t *testing.T) {
	for _, tc := range []struct {
		now, unit  string
		offset     int
		start, end string
	}{
		{"2026-01-18", "month", -1, "2025-12-01", "2026-01-01"},
		{"2026-12-31", "month", 1, "2027-01-01", "2027-02-01"},
		{"2024-03-17", "month", -1, "2024-02-01", "2024-03-01"},
		{"2026-10-18", "week", 0, "2026-10-12", "2026-10-19"},
		{"2026-10-19", "week", 1, "2026-10-26", "2026-11-02"},
		{"2026-01-01", "week", -1, "2025-12-22", "2025-12-29"},
		{"2026-01-08", "quarter", -1, "2025-10-01", "2026-01-01"},
		{"2026-11-17", "quarter", 1, "2027-01-01", "2027-04-01"},
		{"2026-08-01", "year", -1, "2025-01-01", "2026-01-01"},
	} {
		t.Run(tc.now+tc.unit, func(t *testing.T) {
			now, _ := time.Parse("2006-01-02", tc.now)
			r, err := ResolveTemporal(calendarPeriod(tc.unit, tc.offset), now)
			if err != nil || r.Start.String() != tc.start || r.End.String() != tc.end {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestNormalizedTemporalTimezoneAndDST(t *testing.T) {
	instant, _ := time.Parse(time.RFC3339, "2026-03-31T22:30:00Z")
	moscow, _ := time.LoadLocation("Europe/Moscow")
	r, err := ResolveTemporal(calendarPeriod("month", -1), instant.In(moscow))
	if err != nil || r.Start.String() != "2026-03-01" || r.End.String() != "2026-04-01" {
		t.Fatal(r, err)
	}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, berlin)
	day, err := ResolveTemporal(TemporalMeaning{Type: "relative_delay", Unit: planStr("day"), Value: planInt(1)}, now)
	if err != nil || day.At.Hour() != 12 || day.At.Sub(now) != 23*time.Hour {
		t.Fatal(day, err)
	}
	hours, err := ResolveTemporal(TemporalMeaning{Type: "relative_delay", Unit: planStr("hour"), Value: planInt(24)}, now)
	if err != nil || hours.At.Hour() != 13 || hours.At.Sub(now) != 24*time.Hour {
		t.Fatal(hours, err)
	}
}

func TestNormalizedTemporalRejectsImpossibleAndIncompatible(t *testing.T) {
	for _, v := range []TemporalMeaning{
		{Type: "calendar_period", Unit: planStr("hour"), Offset: planInt(0)},
		{Type: "relative_delay", Unit: planStr("minute"), Value: planInt(-1)},
		{Type: "calendar_date", Year: planInt(2026), Month: planInt(2), Day: planInt(29)},
		{Type: "weekday", Weekday: planStr("friday"), Direction: planStr("guess")},
		{Type: "relative_day", Offset: planInt(1), Year: planInt(2026)},
	} {
		if _, err := ResolveTemporal(v, time.Now()); err == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
}

func TestNormalizedRollingPeriodIncludesExactlyFourteenDays(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	r, err := ResolveTemporal(TemporalMeaning{Type: "rolling_period", Unit: planStr("week"), Value: planInt(2)}, now)
	if err != nil || r.Start.String() != "2026-10-04" || r.End.String() != "2026-10-18" {
		t.Fatal(r, err)
	}
}

func TestNormalizedWeekdayHasCanonicalDefault(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	r, err := ResolveTemporal(TemporalMeaning{Type: "weekday", Weekday: planStr("wednesday")}, now)
	if err != nil || r.Start.String() != "2026-10-21" {
		t.Fatal(r, err)
	}
}

func TestStrictSemanticPlan(t *testing.T) {
	p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("search"), Kind: planStr("meeting"), Query: planStr("Алисой"), Filters: &PlanFilters{Period: func() *TemporalMeaning { v := calendarPeriod("month", -1); return &v }()}}
	raw, _ := json.Marshal(p)
	if _, err := DecodeSemanticPlan(raw); err != nil {
		t.Fatal(err)
	}
	for name, invalid := range map[string]string{
		"unknown":        strings.Replace(string(raw), `"status":"ok"`, `"id":42,"status":"ok"`, 1),
		"duplicate":      strings.Replace(string(raw), `"offset":-1`, `"offset":0,"offset":-1`, 1),
		"trailing":       string(raw) + ` {}`,
		"missing":        strings.Replace(string(raw), `"title":null,`, "", 1),
		"enum":           strings.Replace(string(raw), `"operation":"search"`, `"operation":"execute_sql"`, 1),
		"wrong null":     strings.Replace(string(raw), `"status":"ok"`, `"status":null`, 1),
		"fraction":       strings.Replace(string(raw), `"offset":-1`, `"offset":-1.5`, 1),
		"nested unknown": strings.Replace(string(raw), `"exclude":null`, `"exclude":[{"id":1}]`, 1),
		"wrong scalar":   strings.Replace(string(raw), `"query":"Алисой"`, `"query":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSemanticPlan([]byte(invalid)); err == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
	p.Operation = planStr("delete")
	if p.Validate() == nil {
		t.Fatal("accepted targetless mutation")
	}
	p.Status = "ambiguous"
	raw, _ = json.Marshal(p)
	refusal, err := DecodeSemanticPlan(raw)
	if err != nil || refusal.Status != "ambiguous" || refusal.Operation != nil || refusal.Target != nil {
		t.Fatal("non-executable refusal was not canonicalized", refusal, err)
	}
}
