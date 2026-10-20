package contract

import (
	"path/filepath"
	"testing"
)

func TestPostgresDemoImportIsIdempotentAndPrivate(t *testing.T) {
	h := schedulingFixture(t)
	cal := h.service.Calendar
	cal.DemoPath = filepath.Join(root(), "data", "voice_assistant_events_1000.csv")
	first, err := cal.ImportDemo(h.a, h.now)
	if err != nil || first.Imported != 1000 || first.TotalRows != 1000 {
		t.Fatal(first, err)
	}
	repeat, err := cal.ImportDemo(h.a, h.now)
	if err != nil || repeat.Imported != 0 || repeat.Skipped != 1000 {
		t.Fatal(repeat, err)
	}
	if count := personalRecordCount(t, h); count != 1000 {
		t.Fatal(count)
	}
	other, err := cal.Count(h.b)
	if err != nil || other != 0 {
		t.Fatal("import leaked into another account", other, err)
	}
}
