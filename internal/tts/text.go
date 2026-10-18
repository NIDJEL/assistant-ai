package tts

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	fences     = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")
	hiddenHTML = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)\s*>`)
	tags       = regexp.MustCompile(`<[/!?]?[A-Za-z][^>]*>`)
	links      = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	lineMarks  = regexp.MustCompile(`(?m)^\s{0,3}(?:#{1,6}\s+|>\s*|[-*+]\s+|\d+\.\s+)`)
	markup     = regexp.MustCompile("[*_`~]+")
)

// PrepareText removes presentation/code, preserving ordinary prose, dates and times.
// Callers must validate the original size too, before doing any parsing.
func PrepareText(text string) string {
	text = strings.TrimSpace(text)
	if (strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")) && json.Valid([]byte(text)) {
		return ""
	}
	text = fences.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = hiddenHTML.ReplaceAllString(text, " ")
	text = tags.ReplaceAllString(text, " ")
	text = links.ReplaceAllString(text, "$1")
	text = removeJSON(text)
	text = lineMarks.ReplaceAllString(text, "")
	text = markup.ReplaceAllString(text, "")
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || (unicode.IsControl(r) && r != '\n') }), " ")
}

func removeJSON(text string) string {
	var result strings.Builder
	for len(text) > 0 {
		i := strings.IndexAny(text, "{[")
		if i < 0 {
			result.WriteString(text)
			break
		}
		result.WriteString(text[:i])
		text = text[i:]
		decoder := json.NewDecoder(strings.NewReader(text))
		var value json.RawMessage
		if decoder.Decode(&value) == nil {
			result.WriteByte(' ')
			text = text[decoder.InputOffset():]
		} else {
			result.WriteByte(text[0])
			text = text[1:]
		}
	}
	return result.String()
}

func ValidText(text string) bool {
	return utf8.ValidString(text) && utf8.RuneCountInString(text) <= MaxTextRunes && strings.TrimSpace(text) != ""
}
