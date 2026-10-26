package contract

import (
	"fmt"
	"testing"
)

func TestPersonalMeetingSeriesAPIIsolationExceptionsAndRestore(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	_, owner := registerHTTP(t, router, "series-owner@example.test")
	_, other := registerHTTP(t, router, "series-other@example.test")
	input := map[string]any{"kind": "meeting", "title": "Обсуждение проекта", "date": "2026-11-02", "time": "09:00", "duration_minutes": 30, "recurrence": map[string]any{"frequency": "week", "interval": 1, "weekdays": []int{1}, "local_time": "09:00", "timezone": "Europe/Moscow", "count": 3}}
	master, _ := call(t, router, owner, "POST", "/api/events", input, 201)
	path := seriesEventPath(master)
	call(t, router, other, "GET", path, nil, 404)
	rows, _ := call(t, router, other, "GET", "/api/events?expand=true&start=2026-11-01&end=2026-11-24", nil, 200)
	if len(rows["events"].([]any)) != 0 {
		t.Fatal("series leaked across users")
	}
	child, _ := call(t, router, owner, "PATCH", path, map[string]any{"edit_occurrence_at": "2026-11-09T09:00:00+03:00", "time": "11:00"}, 200)
	if child["id"] == master["id"] || child["series_id"] != master["id"] {
		t.Fatal(child)
	}
	rows, _ = call(t, router, owner, "GET", "/api/events?expand=true&start=2026-11-01&end=2026-11-24", nil, 200)
	if len(rows["events"].([]any)) != 3 {
		t.Fatal("missing or duplicated future occurrence", rows)
	}
	call(t, router, owner, "DELETE", path, nil, 204)
	call(t, router, owner, "GET", seriesEventPath(child), nil, 404)
	call(t, router, owner, "POST", path+"/restore", map[string]any{}, 200)
	call(t, router, owner, "GET", seriesEventPath(child), nil, 200)
	input["series_id"] = master["id"]
	input["recurrence_id"] = "2026-11-16T06:00:00Z"
	call(t, router, owner, "POST", "/api/events", input, 422)
}

func seriesEventPath(e map[string]any) string {
	return fmt.Sprintf("/api/events/%.0f", e["id"].(float64))
}
