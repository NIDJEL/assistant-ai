package nlp

import "testing"

func TestOrdinalGroundingRequiresUserEvidence(t *testing.T) {
	for _, tc := range []struct {
		text  string
		index int
		want  bool
	}{
		{"Пожалуйста, вторую перемести на 14:40", 2, true},
		{"У первой поменяй время", 1, true},
		{"Ту первую перенеси на следующий понедельник", 1, true},
		{"Пусть эта запись начнётся в 13:20", 2, false},
		{"Ту передвинь на 2 часа позже", 2, false},
		{"Поменяй время на 1:20", 1, false},
		{"Вторую перенеси", 1, false},
		{"Сдвинь её во второй половине дня", 2, false},
		{"Перенеси её на вторую неделю", 2, false},
		{"Двадцать вторую измени", 2, false},
		{"Первую или вторую перенеси", 2, false},
	} {
		if got := HasOrdinalEvidence(tc.text, tc.index, 8); got != tc.want {
			t.Fatalf("%q: got %v", tc.text, got)
		}
	}
}
