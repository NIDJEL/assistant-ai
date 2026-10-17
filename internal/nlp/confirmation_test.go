package nlp

import "testing"

func TestConfirmationPolarityAndCompleteAnswer(t *testing.T) {
	for _, message := range []string{"нет", "не надо", "не удаляй", "нет, не удаляй", "отмена", "отмени", "передумал", "передумала", "не нужно", "оставь", "пусть останется", "не трогай", "Пожалуй, не надо", "Ну нет, пожалуйста, не трогай", "Оставь как есть", "Гена, пока не нужно"} {
		if got := ClassifyConfirmation(message); got != ConfirmationNo {
			t.Errorf("negative %q: %v", message, got)
		}
	}
	for _, message := range []string{"да", "ага", "подтверждаю", "давай", "удаляй", "точно", "верно", "Да, удаляй", "Ну да, пожалуйста", "Да все равно", "да, удали все эти задачи"} {
		if got := ClassifyConfirmation(message); got != ConfirmationYes {
			t.Errorf("positive %q: %v", message, got)
		}
	}
	for _, message := range []string{"давай лучше завтра", "нет, перенеси на среду", "не удаляй задачу, а создай встречу", "да нет", "да, не удаляй", "подтверждаю?", "слово «да»", "удаляй архив", "пусть останется до завтра", "не отменяй", "неточность", "подтверждаю удаление чужих записей"} {
		if got := ClassifyConfirmation(message); got != ConfirmationUnknown {
			t.Errorf("ambiguous %q: %v", message, got)
		}
	}
}
