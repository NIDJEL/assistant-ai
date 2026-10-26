package assistant

import (
	"encoding/json"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"regexp"
)

var readTemporalMention = regexp.MustCompile(`(?i)(сегодня|сейчас|завтра|вчера|недел|месяц|год|сутк|день|дня|дней|утр|вечер|понедельник|вторник|сред[ауеы]|четверг|пятниц|суббот|воскресень|январ|феврал|март|апрел|ма[йяе]|июн|июл|август|сентябр|октябр|ноябр|декабр|[0-9])`)

// A topic question does not imply "today". Only withhold an invented default;
// never infer a date or broaden an explicit or inherited temporal constraint.
func guardReadDefault(op nlu.Operation, message string) nlu.Operation {
	args, err := op.Args()
	if err != nil {
		return op
	}
	read, ok := args.(*nlu.Read)
	if ok && !read.Inherit && read.Query != "" && read.Date != nil && read.Date.Type == "relative_day" && read.Date.Offset == 0 && !readTemporalMention.MatchString(message) {
		read.Date = nil
		op.Arguments, _ = json.Marshal(read)
	}
	return op
}
