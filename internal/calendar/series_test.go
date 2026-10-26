package calendar

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

func seriesFixture(t *testing.T) (*Service, time.Time) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "series.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	zone, _ := time.LoadLocation("Europe/Moscow")
	return &Service{Store: db, Location: zone}, time.Date(2026, 10, 26, 8, 0, 0, 0, zone)
}
func meeting(at time.Time) domain.Event {
	return domain.Event{Kind: domain.EventMeeting, Title: "Обсуждение проекта", Date: domain.Day(at), StartTime: &domain.TimeOfDay{Hour: at.Hour(), Minute: at.Minute()}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
}
func weekly(at time.Time, count int) domain.Event {
	e := meeting(at)
	e.Recurrence = &domain.Recurrence{Frequency: "week", Interval: 1, Count: count, Timezone: at.Location().String(), Anchor: at, LocalTime: e.StartTime, Weekdays: []time.Weekday{at.Weekday()}}
	return e
}
func TestRecurringMeetingChecksLaterDatesInBothDirections(t *testing.T) {
	for _, seriesFirst := range []bool{false, true} {
		t.Run(map[bool]string{true: "series first", false: "single first"}[seriesFirst], func(t *testing.T) {
			s, now := seriesFixture(t)
			first := now.Add(time.Hour)
			a, b := meeting(first.AddDate(0, 0, 14)), weekly(first, 3)
			if seriesFirst {
				a, b = b, a
			}
			if _, err := s.Create(t.Context(), a, now, false); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Create(t.Context(), b, now, false); err == nil {
				t.Fatal("future conflict accepted")
			}
		})
	}
}
func TestFutureOccurrenceIsDetachedOnceAndConflictRollsBack(t *testing.T) {
	s, now := seriesFixture(t)
	first := now.Add(time.Hour)
	master, err := s.Create(t.Context(), weekly(first, 4), now, false)
	if err != nil {
		t.Fatal(err)
	}
	original := first.AddDate(0, 0, 14)
	clock := &domain.TimeOfDay{Hour: 12}
	stamp := original
	child, err := s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &stamp, Time: &clock}, now)
	if err != nil {
		t.Fatal(err)
	}
	if child.ID == master.ID || child.SeriesID == nil || *child.SeriesID != master.ID || child.Recurrence != nil || !child.RecurrenceID.Equal(original) {
		t.Fatal(child)
	}
	current, err := s.Get(t.Context(), master.ID)
	if err != nil || !current.Recurrence.Anchor.Equal(first) || !current.Recurrence.Excluded(original) || current.Date.String() != master.Date.String() {
		t.Fatal(current, err)
	}
	clock = &domain.TimeOfDay{Hour: 13}
	retry, err := s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &stamp, Time: &clock}, now)
	if err != nil || retry.ID != child.ID {
		t.Fatal("duplicate override", retry, err)
	}
	list, err := s.ListOccurrences(t.Context(), domain.Filter{Start: domain.Day(first), End: domain.Day(first).AddDays(28), Status: domain.StatusOpen}, "", 200, 0)
	if err != nil || list.Total != 4 {
		t.Fatal(list, err)
	}
	for _, row := range list.Events {
		if row.Date.String() == child.Date.String() && row.StartTime.Hour == 9 {
			t.Fatal("excluded occurrence still shown")
		}
	}
	blocker := meeting(first.AddDate(0, 0, 21))
	blocker.StartTime = &domain.TimeOfDay{Hour: 15}
	if _, err = s.Create(t.Context(), blocker, now, false); err != nil {
		t.Fatal(err)
	}
	last := first.AddDate(0, 0, 21)
	clock = &domain.TimeOfDay{Hour: 15}
	if _, err = s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &last, Time: &clock}, now); err == nil {
		t.Fatal("conflicting override accepted")
	}
	current, _ = s.Get(t.Context(), master.ID)
	if current.Recurrence.Excluded(last) {
		t.Fatal("parent changed after rejected override")
	}
	// Moving one occurrence onto a different occurrence of its own series must
	// exclude only the original slot, not the whole parent event.
	nextDay := domain.Day(first.AddDate(0, 0, 7))
	if _, err = s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &last, Date: &nextDay}, now); err == nil {
		t.Fatal("override overlapped another occurrence of its own series")
	}
}
func TestCancelledOccurrenceDoesNotCancelSeries(t *testing.T) {
	s, now := seriesFixture(t)
	first := now.Add(time.Hour)
	master, err := s.Create(t.Context(), weekly(first, 3), now, false)
	if err != nil {
		t.Fatal(err)
	}
	at := first.AddDate(0, 0, 7)
	done := domain.StatusDone
	if _, err = s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &at, Status: &done}, now); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListOccurrences(t.Context(), domain.Filter{Start: domain.Day(first), End: domain.Day(first).AddDays(21), Status: domain.StatusOpen}, "", 200, 0)
	if err != nil || list.Total != 2 {
		t.Fatal(list, err)
	}
}

func TestDeleteAndRestoreSeriesIncludesOnlyItsCascadedChildren(t *testing.T) {
	s, now := seriesFixture(t)
	first := now.Add(time.Hour)
	master, err := s.Create(t.Context(), weekly(first, 4), now, false)
	if err != nil {
		t.Fatal(err)
	}
	at := first.AddDate(0, 0, 7)
	clock := &domain.TimeOfDay{Hour: 12}
	a, err := s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &at, Time: &clock}, now)
	if err != nil {
		t.Fatal(err)
	}
	at = first.AddDate(0, 0, 14)
	b, err := s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &at, Time: &clock}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	} // an earlier, independent deletion
	if err = s.Delete(t.Context(), master.ID); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.Get(t.Context(), a.ID); e != nil {
		t.Fatal("series delete left a detached occurrence active")
	}
	if _, err = s.Restore(t.Context(), a.ID, now, false); err == nil {
		t.Fatal("restored orphan before parent")
	}
	if _, err = s.Restore(t.Context(), master.ID, now, false); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.Get(t.Context(), a.ID); e == nil || e.SeriesDeleted {
		t.Fatal("cascaded child not restored")
	}
	if e, _ := s.Get(t.Context(), b.ID); e != nil {
		t.Fatal("independently deleted occurrence was resurrected")
	}
}

func TestMonthCalendarCountsFrequentRemindersWithoutTruncation(t *testing.T) {
	s, now := seriesFixture(t)
	e := meeting(now)
	e.Kind = domain.EventReminder
	e.Recurrence = &domain.Recurrence{Frequency: "minute", Interval: 1, Timezone: now.Location().String(), Anchor: now}
	if _, err := s.Create(t.Context(), e, now, false); err != nil {
		t.Fatal(err)
	}
	result, err := s.ListOccurrences(t.Context(), domain.Filter{Start: domain.Day(now), End: domain.Day(now).AddDays(42)}, "", 200, 0)
	if err != nil || result.Total != 42 {
		t.Fatal("minute recurrence broke the month", result.Total, err)
	}
	if result.Events[1].OccurrencesInDay != 1440 {
		t.Fatal("lost repeated reminders", result.Events[1])
	}
}

func TestRestoringSeriesRollsBackWhenDetachedChildNowConflicts(t *testing.T) {
	s, now := seriesFixture(t)
	first := now.Add(time.Hour)
	master, err := s.Create(t.Context(), weekly(first, 3), now, false)
	if err != nil {
		t.Fatal(err)
	}
	at := first.AddDate(0, 0, 7)
	clock := &domain.TimeOfDay{Hour: 12}
	child, err := s.Update(t.Context(), master.ID, Patch{OccurrenceAt: &at, Time: &clock}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(t.Context(), master.ID); err != nil {
		t.Fatal(err)
	}
	blocker := meeting(at)
	blocker.StartTime = clock
	if _, err = s.Create(t.Context(), blocker, now, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Restore(t.Context(), master.ID, now, false); err == nil {
		t.Fatal("restoration ignored detached conflict")
	}
	if e, _ := s.Get(t.Context(), master.ID); e != nil {
		t.Fatal("partial restore committed parent")
	}
	if e, _ := s.Get(t.Context(), child.ID); e != nil {
		t.Fatal("partial restore committed child")
	}
}
