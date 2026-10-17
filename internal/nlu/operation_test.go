package nlu

import (
	"testing"
	"time"
)

func TestOperationsRejectDatabaseIDsAndCrossOperationFields(t *testing.T) {
	for _, raw := range []string{
		`{"intent":"delete","confidence":"high","arguments":{"target":{"type":"last","id":481}}}`,
		`{"intent":"create_task","confidence":"high","arguments":{"title":"X","target":{"type":"last"}}}`,
		`{"intent":"batch_delete","confidence":"high","arguments":{"filter":{"id":4}}}`,
		`{"intent":"chat","confidence":"high","arguments":{"reply":"Привет","changes":{"time":"12:00"}}}`,
		`{"intent":"delete","confidence":"high","arguments":{"target":{"type":"last"}}} {}`,
	} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatal("unsafe output accepted", raw)
		}
	}
}
func TestCalendarArithmeticUsesUserTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 28, 23, 45, 0, 0, loc)
	start, end, _, err := (Temporal{Type: "relative_day", Offset: 1}).Resolve(now)
	if err != nil || start.String() != "2026-03-29" || end.String() != "2026-03-30" {
		t.Fatal(start, end, err)
	}
	start, _, exact, err := (Temporal{Type: "relative_delay", Unit: "minute", Value: 30}).Resolve(now)
	if err != nil || start.String() != "2026-03-29" || exact.Format("15:04") != "00:15" {
		t.Fatal(start, exact, err)
	}
	start, end, _, err = (Temporal{Type: "calendar_period", Unit: "month", Offset: -1}).Resolve(now)
	if err != nil || start.String() != "2026-02-01" || end.String() != "2026-03-01" {
		t.Fatal(start, end, err)
	}
}
