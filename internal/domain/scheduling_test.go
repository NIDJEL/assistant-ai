package domain

import "testing"

func TestMeetingDisplayTitleLeavesStoredSubjectsAndUsesRealNames(t *testing.T) {
	for _, c := range []struct {
		title string
		names []string
		want  string
	}{
		{"Встреча", []string{"Никита Круглов"}, "Встреча с Никитой Кругловым"},
		{"Встреча", []string{"Матвей Лапин"}, "Встреча с Матвеем Лапиным"},
		{"Обсуждение дизайна приложения", []string{"Никита Круглов"}, "Обсуждение дизайна приложения"},
	} {
		if got := MeetingDisplayTitle(c.title, c.names); got != c.want {
			t.Fatalf("%q: %q != %q", c.title, got, c.want)
		}
	}
}
