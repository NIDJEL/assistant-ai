package nlp

// Timer intervals are countdowns only in a timer directive. Meeting duration
// phrases keep their existing meaning everywhere else.
func IsTimerRequest(text string) bool {
	raw := NormalizeText(text).Analysis
	raw = replace(`^(?:(?:пожалуйста|гена|ну|давай)[, ]+)+`, raw, "")
	directive := `(?:(?:поставь|запусти|включи)\s+)?таймер|засеки`
	return has(`^(?:`+directive+`)\b`, raw) || has(`^(?:`+timerAmountPattern+`)\s+(?:`+directive+`)\b`, raw)
}

const timerUnit = `(?:секунд(?:у|ы)?|минут(?:у|ы)?|час(?:а|ов)?)`
const timerAmountPattern = `(?:(?:на\s+)?(?:` + relativeAmount + `\s+` + timerUnit + `|полчаса|час)|` + timerUnit + `\s+(?:на\s+)?` + relativeAmount + `)`

func timerInterval(text string) string {
	if !IsTimerRequest(text) {
		return ""
	}
	raw := NormalizeText(text).Analysis
	if m := first(`\b`+timerAmountPattern+`\b`, raw); m != nil {
		return m.Parts[0]
	}
	return ""
}

func groundedTimerExpression(expression, original string) bool {
	interval := timerInterval(original)
	return interval != "" && temporalAlias(timerRelative(interval, original)) == temporalAlias(timerRelative(expression, original))
}

func timerRelative(expression, original string) string {
	if !IsTimerRequest(original) {
		return expression
	}
	raw := temporalAlias(expression)
	if m := first(`^(`+timerUnit+`)\s+(?:на\s+)?(`+relativeAmount+`)$`, raw); m != nil {
		return "через " + m.Parts[2] + " " + m.Parts[1]
	}
	raw = replace(`^на\s+`, raw, "")
	if full(`(?:`+relativeAmount+`\s+)?(?:`+timerUnit+`|полчаса)`, raw) {
		return "через " + raw
	}
	return expression
}
