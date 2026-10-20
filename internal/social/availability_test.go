package social

import (
	"testing"
	"time"
)

func TestMergeBusyDeduplicatesProvidersAndUsesHalfOpenIntervals(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	slots := MergeBusy([]Slot{{start, start.Add(time.Hour)}, {start, start.Add(time.Hour)}, {start.Add(30 * time.Minute), start.Add(2 * time.Hour)}, {start.Add(2 * time.Hour), start.Add(3 * time.Hour)}})
	if len(slots) != 1 || !slots[0].End.Equal(start.Add(3*time.Hour)) {
		t.Fatal(slots)
	}
	if overlaps(Slot{start, start.Add(time.Hour)}, Slot{start.Add(time.Hour), start.Add(2 * time.Hour)}) {
		t.Fatal("adjacent appointments conflict")
	}
}
