package nlp

import (
	"testing"
	"time"
)

func TestFilterRefinementOnlyChangesExplicitFilter(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 30, 17, 0, time.UTC)
	for _, message := range []string{"А только за прошлую неделю?", "Ну а только на этой неделе?", "Тогда оставь только на следующей неделе?", "А давай только встречи?", "Только задачи"} {
		if ExtractFilterRefinement(message, now) == nil {
			t.Errorf("missing filter %q", message)
		}
	}
	for _, message := range []string{"Найди встречи с Яном", "А только после 15?", "Только за прошлую неделю или на следующей неделе", "А только завтра и удали все", "Не надо, только встречи", "Только встречи с Денисом"} {
		if ExtractFilterRefinement(message, now) != nil {
			t.Errorf("partial/invented filter accepted %q", message)
		}
	}
}
