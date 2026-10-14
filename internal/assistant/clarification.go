package assistant

import (
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func creationQuestion(cmd domain.Command) string {
	missing := nlp.MissingEntities(cmd)
	if len(missing) == 0 {
		return ""
	}
	switch missing[0] {
	case nlp.ActionText:
		return "Что нужно сделать или с кем провести встречу?"
	case nlp.RecurrenceDayEntity:
		return "В какой день недели повторять?"
	case nlp.DayEntity:
		if cmd.Kind == domain.EventReminder {
			return "В какой день указанного периода напомнить?"
		}
		return "На какой день в указанном периоде поставить событие?"
	case nlp.DateEntity:
		if cmd.Kind == domain.EventReminder {
			return "Когда напомнить?"
		}
		if cmd.Kind == domain.EventTask {
			return "На какую дату поставить задачу с указанным временем?"
		}
		return "На какую дату запланировать событие?"
	case nlp.TimeEntity:
		if cmd.Kind == domain.EventReminder {
			return "Во сколько напомнить?"
		}
		return "На какое время поставить событие? Например, в 15:00."
	}
	return ""
}
