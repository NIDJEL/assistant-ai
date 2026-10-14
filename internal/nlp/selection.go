package nlp

import (
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func HasDirective(text string) bool {
	text = strings.ReplaceAll(strings.ToLower(scheduleOnly(text)), "ё", "е")
	return has(`^\s*(?:а\s+)?(?:что|какие|есть\s+ли|список|планы|расписание|когда)\b`, text) || has(`\b(?:напомни|напомнить|поставь|поставить|добавь|добавить|создай|создать|запиши|записать|запланируй|назначь|установи|удали\w*|удалить|отмени\w*|убери\w*|перенеси\w*|перенести|сдвинь|измени\w*|заверши\w*|закончи\w*|выполнил\w*|отметь|найди|найти|отыщи|поищи|покажи|показать)\b`, text)
}

// SelectionReply parses a name/date supplied while choosing an existing record.
// In this context "позвонить маме" is a title, not a request to create a task.
func SelectionReply(text string, now time.Time) domain.Command {
	cmd := domain.Command{Action: domain.ActionDelete, Duration: time.Hour}
	planSelection(&cmd, normalizeConversation(text), now)
	return cmd
}

func selectionTemporal(text string, now time.Time) Temporal {
	t := Extract(text, now)
	if extra := Extract(t.Clean(text), now); t.Time != nil && t.Clean(text) != strings.Trim(spaces(text), " ,.!?;:-") && extra.Time != nil {
		t.Error = "Вижу несколько времён. Уточните одно время нужной записи или переноса."
	}
	return t
}

func planSelection(cmd *domain.Command, text string, now time.Time) {
	outside := strings.NewReplacer("ё", "е", "Ё", "Е").Replace(scheduleOnly(text))
	for _, pair := range [][2]string{{`завтрашн\w*`, "завтра"}, {`сегодняшн\w*`, "сегодня"}, {`вчерашн\w*`, "вчера"}} {
		outside = replace(`(?i)\b`+pair[0]+`\b`, outside, pair[1])
	}
	low := strings.ReplaceAll(strings.ToLower(outside), "ё", "е")
	cmd.TargetID = nil
	if m := first(`(?:#|номер\s*|№\s*)(\d+)`, low); m != nil {
		id, err := strconv.ParseInt(m.Parts[1], 10, 64)
		if err != nil || id < 1 {
			cmd.Error = "Некорректный номер записи."
			return
		}
		cmd.TargetID = &id
	}
	cmd.Kind = inferKind(low)
	sel := &domain.EventSelector{All: has(`\b(?:все|всех|весь|целиком|полностью)\b`, low)}
	cmd.Selector = sel
	if has(`\b(?:кроме|исключая|за\s+исключением|но\s+не|только\s+не)\b`, low) {
		cmd.Error = "Чтобы не затронуть лишнее, укажите записи для действия без исключений: название и дату или период."
		return
	}

	// A move has a target before "на" and a destination after it.
	// Bare "завтра" in "перенеси встречу завтра" is a destination too.
	source, destination := outside, ""
	if cmd.Action == domain.ActionUpdate {
		for _, m := range matches(`\bна\s+`, low) {
			candidate := outside[m.End:]
			t := Extract(candidate, now)
			if t.Date != nil || t.Time != nil || t.Start != nil || t.Error != "" {
				source, destination = outside[:m.Start], candidate
				break
			}
		}
		if destination == "" {
			t := selectionTemporal(outside, now)
			cmd.Date, cmd.Time, cmd.PeriodStart, cmd.PeriodEnd, cmd.Error = t.Date, t.Time, t.Start, t.End, t.Error
			source = t.Clean(outside)
		} else {
			t := selectionTemporal(destination, now)
			cmd.Date, cmd.Time, cmd.PeriodStart, cmd.PeriodEnd, cmd.Error = t.Date, t.Time, t.Start, t.End, t.Error
			if t.Start != nil && t.Date == nil && cmd.Error == "" {
				cmd.Error = "На какой день в указанном периоде перенести запись?"
			}
		}
	}
	t := selectionTemporal(source, now)
	sel.Start, sel.End, sel.Time = t.Start, t.End, t.Time
	if t.Error != "" {
		cmd.Error = t.Error
	}
	if cmd.Action != domain.ActionUpdate {
		cmd.Date, cmd.Time, cmd.PeriodStart, cmd.PeriodEnd = t.Date, t.Time, t.Start, t.End
	}
	query := t.Clean(source)
	query = replace(`(?i)(?:#|номер\s*|№\s*)\d+`, query, " ")
	// Remove the outer instruction once. Another verb can be the actual name:
	// "удали дело удалить временные файлы" must retain "удалить".
	if m := first(`(?i)^\s*(?:(?:а|пожалуйста|можешь|можете|ритм)\s+)*(?:удали\w*|удалить|отмени\w*|убери\w*|перенеси\w*|перенести|сдвинь|измени\w*|выполнил\w*|выполнен\w*|сделан\w*|заверши\w*|закончи\w*|отметь)\b`, query); m != nil {
		query = query[m.End:]
	}
	if cmd.Action == domain.ActionComplete {
		query = replace(`(?i)\b(?:как\s+)?(?:выполнен\w*|сделан\w*)\s*$`, query, " ")
	}
	query = replace(`(?i)\b(?:по\s+названию|с\s+названием|под\s+названием|которая|который|которые|которую|которое|что|ту|тот|те|это|эту|этот|ее|его|их|у\s+меня|мои|мою|мой|мое|мне|пожалуйста|все|всех|весь|целиком|полностью|запис[ьи]|записей|запись|дело|дела|дел|задач\w*|напоминан\w*|уведомлен\w*|событи\w*|встреч[ауи]|встреч|как|а)\b`, query, " ")
	query = strings.Trim(spaces(query), " ,.!?:;-«»\"")
	for {
		next := replace(`(?i)^(?:на|в|во|к|за|с|со|о|об|про|по|от|для)\b\s*|\s*\b(?:на|в|во|к|за|с|со|о|об|про|по|от|для)$`, query, "")
		if next == query {
			break
		}
		query = strings.TrimSpace(next)
	}
	quotes := matches(`[«"]([^»"]+)[»"]`, text)
	if len(quotes) == 1 {
		query = strings.TrimSpace(quotes[0].Parts[1])
		sel.ExactTitle = true
	} else if len(quotes) > 1 {
		cmd.Error = "Укажите одно название или общий период для всех нужных записей."
	}
	cmd.Query = query
}
