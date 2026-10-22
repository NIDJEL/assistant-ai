package auth

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Applied only when setting a password. Login must keep working for accounts
// created before this policy, and passwords are never trimmed before hashing.
func validatePassword(password string) error {
	if !utf8.ValidString(password) || len(password) > 1024 {
		return domain.Validation("Пароль слишком длинный: используйте до 1024 байт")
	}
	var compact []rune
	letter, other := false, false
	unique := map[rune]bool{}
	for _, r := range password {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return domain.Validation("Уберите управляющие и невидимые символы из пароля")
		}
		if unicode.IsSpace(r) {
			continue
		}
		compact = append(compact, unicode.ToLower(r))
		unique[unicode.ToLower(r)] = true
		if unicode.IsLetter(r) {
			letter = true
		} else if !unicode.IsMark(r) {
			other = true
		}
	}
	if len(compact) < 10 {
		return domain.Validation("Используйте не меньше 10 символов без учёта пробелов")
	}
	weak := len(unique) < 4
	for period := 1; period <= len(compact)/2 && !weak; period++ {
		if len(compact)%period != 0 {
			continue
		}
		repeated := true
		for i := period; i < len(compact); i++ {
			if compact[i] != compact[i%period] {
				repeated = false
				break
			}
		}
		weak = repeated
	}
	value := string(compact)
	for _, sequence := range []string{"01234567890123456789", "98765432109876543210", "abcdefghijklmnopqrstuvwxyz", "zyxwvutsrqponmlkjihgfedcba", "qwertyuiopasdfghjklzxcvbnm", "йцукенгшщзхъфывапролджэячсмитьбю"} {
		weak = weak || strings.Contains(sequence, value)
	}
	for _, common := range []string{"password", "пароль", "qwerty", "йцукен", "letmein", "welcome", "admin", "administrator", "iloveyou"} {
		if !strings.HasPrefix(value, common) {
			continue
		}
		suffixOnly := true
		for _, r := range strings.TrimPrefix(value, common) {
			if unicode.IsLetter(r) || unicode.IsMark(r) {
				suffixOnly = false
			}
		}
		weak = weak || suffixOnly
	}
	if weak {
		return domain.Validation("Пароль слишком простой: избегайте повторов, последовательностей и популярных сочетаний")
	}
	words := strings.Fields(password)
	phrase := len(words) >= 4 && len(compact) >= 16 && len(unique) >= 8
	if phrase {
		seen := map[string]bool{}
		for _, word := range words {
			seen[strings.ToLower(word)] = true
			if utf8.RuneCountInString(word) < 2 {
				phrase = false
			}
		}
		phrase = phrase && len(seen) >= 4
	}
	if !(letter && other) && !phrase {
		return domain.Validation("Добавьте к буквам цифру или символ либо используйте длинную фразу из четырёх разных слов")
	}
	return nil
}
