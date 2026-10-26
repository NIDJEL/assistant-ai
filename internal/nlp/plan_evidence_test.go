package nlp

import "testing"

func TestEvidenceLexicalCanonicalValues(t *testing.T) {
	for lex, n := range dayEvidence {
		for _, offset := range []int{-2, -1, 0, 1, 2} {
			x := offset
			err := checkTemporalEvidence("changes.date", TemporalMeaning{Type: "relative_day", Offset: &x}, lex)
			if (err == nil) != (offset == n) {
				t.Fatalf("%s %d: %v", lex, offset, err)
			}
		}
	}
	for lex, day := range weekdayEvidence {
		for _, d := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
			x := d
			err := checkTemporalEvidence("changes.date", TemporalMeaning{Type: "weekday", Weekday: &x}, "в "+lex)
			if (err == nil) != (day == d) {
				t.Fatal(lex, d, err)
			}
		}
	}
	for _, tc := range []struct {
		s    string
		n    int
		unit string
	}{{"через 23 минут", 23, "minute"}, {"через два часа", 2, "hour"}, {"через 3 дня", 3, "day"}, {"через полчаса", 30, "minute"}} {
		x, u := tc.n, tc.unit
		p := TemporalMeaning{Type: "relative_delay", Value: &x, Unit: &u}
		if err := checkTemporalEvidence("changes.delay", p, tc.s); err != nil {
			t.Fatal(err)
		}
		x++
		if checkTemporalEvidence("changes.delay", p, tc.s) == nil {
			t.Fatal("wrong delay passed")
		}
	}
}

func TestEvidenceExactOriginalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		source, span string
		ok           bool
	}{
		{"Перенеси послезавтра", "завтра", false}, {"Завтра перенеси", "завтра", true}, {"Завтра перенеси", "Завтра", true},
		{"ТАЙМЕР на час", "таймер", true}, {"Kelvin", "kelvin", true},
		{"a_завтра", "завтра", false}, {"завтра́", "завтра", false},
		{"в 18:00", "18:00", true}, {"в 18:00", "8:00", false}, {"на завтра", "на  завтра", false}, {"послезавтра", "послезавтра", true},
	} {
		if ExactSourceSpan(tc.source, tc.span) != tc.ok {
			t.Fatal(tc)
		}
	}
	if disjointEvidence("с завтра на завтра", "завтра", "на завтра") == false {
		t.Fatal("separate roles rejected")
	}
	if disjointEvidence("на завтра", "завтра", "на завтра") {
		t.Fatal("same source occurrence reused")
	}
}

func TestTemporalOrdinalAndDelayCannotAuthorizeRecordShift(t *testing.T) {
	if HasOrdinalEvidence("Вторая неделя", 2, 3) || HasOrdinalEvidence("Вторая половина дня", 2, 3) {
		t.Fatal("temporal ordinal selected a record")
	}
	if !HasOrdinalEvidence("Вторая встреча", 2, 3) {
		t.Fatal("record ordinal rejected")
	}
	if _, ok := explicitShift("через два часа"); ok {
		t.Fatal("delay from now was accepted as a record shift")
	}
}
