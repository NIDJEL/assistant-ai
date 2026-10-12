package nlp

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var compiled sync.Map

type match struct {
	Parts      []string
	Start, End int
}

func regex(pattern string) *regexp.Regexp {
	if value, ok := compiled.Load(pattern); ok {
		return value.(*regexp.Regexp)
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(pattern, `\b`, ""), `\w`, `[\p{L}\p{N}_]`)
	r := regexp.MustCompile(normalized)
	actual, _ := compiled.LoadOrStore(pattern, r)
	return actual.(*regexp.Regexp)
}
func word(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }

// Go's ASCII word boundaries cannot be used for Russian command patterns.
func matches(pattern, text string) []match {
	result := []match{}
	for _, idx := range regex(pattern).FindAllStringSubmatchIndex(text, -1) {
		start, end := idx[0], idx[1]
		if strings.Contains(pattern, `\b`) {
			first, _ := utf8.DecodeRuneInString(text[start:end])
			last, _ := utf8.DecodeLastRuneInString(text[start:end])
			if start > 0 && word(first) {
				before, _ := utf8.DecodeLastRuneInString(text[:start])
				if word(before) {
					continue
				}
			}
			if end < len(text) && word(last) {
				after, _ := utf8.DecodeRuneInString(text[end:])
				if word(after) {
					continue
				}
			}
		}
		parts := []string{}
		for i := 0; i < len(idx); i += 2 {
			v := ""
			if idx[i] >= 0 {
				v = text[idx[i]:idx[i+1]]
			}
			parts = append(parts, v)
		}
		result = append(result, match{parts, start, end})
	}
	return result
}
func first(pattern, text string) *match {
	m := matches(pattern, text)
	if len(m) == 0 {
		return nil
	}
	return &m[0]
}
func has(pattern, text string) bool { return first(pattern, text) != nil }
func replace(pattern, text, value string) string {
	ms := matches(pattern, text)
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		text = text[:m.Start] + value + text[m.End:]
	}
	return text
}
func full(pattern, text string) bool { return regex(`^(?:` + pattern + `)$`).MatchString(text) }
func spaces(text string) string      { return strings.Join(strings.Fields(text), " ") }
