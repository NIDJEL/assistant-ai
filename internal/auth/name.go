package auth

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Accept international names, including combining accents, without accepting
// controls, digits or markup. Separators must occur between parts of the name.
var namePattern = regexp.MustCompile(`^[\p{L}][\p{L}\p{M}]*(?:[ '\x{2010}\x{2011}\x{2019}-][\p{L}][\p{L}\p{M}]*)*$`)

func normalizeName(value string) (string, error) {
	value = strings.Trim(value, " ")
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 100 || !namePattern.MatchString(value) {
		return "", domain.Validation("Укажите имя до 100 символов: только буквы, пробелы, дефисы и апострофы")
	}
	return value, nil
}
