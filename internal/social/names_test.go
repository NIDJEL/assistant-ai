package social

import "testing"

func TestNameMatchesRussianCasesWithoutChoosingSimilarPeople(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		want        bool
	}{
		{"Лапин Матвей", "Лапина Матвей", true},
		{"Лапин Матвей", "Матвеем Лапиным", true},
		{"Никита Круглов", "Кругловым Никитой", true},
		{"Анна Иванова", "Ивановой Анной", true},
		{"Сергей Петров", "Сергею Петрову", true},
		{"Дмитрий Орлов", "Дмитрия", true},
		{"Павел Сергеев", "Сергеевым Павлом", true},
		{"Алексей Ёлкин", "елкиным алексеем", true},
		{"Матвей Лапин", "Матвей Матвей", false},
		{"Матвей Лапин", "Матвей Лапкин", false},
		{"Матвей Лапин", "Андрей Лапин", false},
		{"Матвей Лапин", "Матюха", false},
		{"John Smith", "Johns Smith", false},
		{"Лапин Матвей", "", false},
	} {
		if got := NameMatches(tc.name, tc.query); got != tc.want {
			t.Errorf("%q / %q = %v, want %v", tc.name, tc.query, got, tc.want)
		}
	}
	if SameName("Лапин Матвей", "Лапина Матвей") {
		t.Fatal("inflected matches must not outrank an exact account name")
	}
}
