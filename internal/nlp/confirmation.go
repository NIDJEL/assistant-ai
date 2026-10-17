package nlp

import (
	"regexp"
	"strings"
)

// A refusal-to-authorize marker for pending previews, not a command parser.
var deferConsent = regexp.MustCompile(`(?i)(согласия\s+(?:пока\s+|еще\s+)?не\s+даю|(?:пока|еще)\s+не\s+подтверждаю|подтверждение\s+(?:отложи|позже)|(?:сначала|пока)\s+(?:посмотрю|подумаю|только\s+смотрю)|не\s+считай\s+это\s+согласием)`)

func DeferredConfirmation(text string) bool { return deferConsent.MatchString(folded(text)) }

type ConfirmationReply int

const (
	ConfirmationUnknown ConfirmationReply = iota
	ConfirmationYes
	ConfirmationNo
)

// ClassifyConfirmation recognizes a complete answer, not a substring. Objects,
// alternative dates, questions and mixed consent remain unknown. The caller
// must separately check that the persisted pending actually asks for consent.
func ClassifyConfirmation(message string) ConfirmationReply {
	text := NormalizeText(message).Analysis
	if strings.ContainsAny(text, `?«»"`) {
		return ConfirmationUnknown
	}
	text = regex(`[,!.;:—–]+`).ReplaceAllString(text, " ")
	text = spaces(text)
	filler := `(?:ну|а|пожалуй|пожалуйста|ладно|хорошо|тогда|пока|просто|лучше|гена)`
	negative := `(?:нет|не\s+(?:надо|нужно|удаляй|удалять|трогай|трогать)|отмена|отмени|отменить|стоп|передумал|передумала|оставь(?:\s+как\s+есть)?|пусть\s+останется)`
	if full(`(?:(?:`+filler+`|`+negative+`)\s+)*(?:`+negative+`)(?:\s+(?:`+filler+`|`+negative+`))*`, text) {
		return ConfirmationNo
	}
	positive := `(?:да|ага|подтверждаю(?:\s+удаление)?|давай|удаляй|удали|удалить|сохрани|сохранить|точно|верно|конечно|все\s+равно)`
	// Scope words only accompany a positive answer; they are not consent alone.
	scope := `(?:все|эти|их|записи|события|задачи|напоминания)`
	if full(`(?:(?:`+filler+`)\s+)*(?:`+positive+`)(?:\s+(?:`+positive+`|`+filler+`|`+scope+`))*`, text) {
		return ConfirmationYes
	}
	return ConfirmationUnknown
}
