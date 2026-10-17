package nlp

import (
	"strings"
	"unicode"
)

// HasOrdinalEvidence validates a model's claim about a displayed position.
// This is a bounded grounding allowlist for the eight visible candidates,
// not an intent parser: the full command still always goes to ParsePlan.
// A pronoun, a clock number or an ordinal inside a temporal phrase is not proof
// that the user selected a particular record. Unsupported forms clarify safely.
func HasOrdinalEvidence(text string, position, count int) bool {
	if position < 1 || position > min(count, 8) {
		return false
	}
	words := strings.FieldsFunc(strings.ReplaceAll(strings.ToLower(text), "ё", "е"), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	acc := []string{"первую", "вторую", "третью", "четвертую", "пятую", "шестую", "седьмую", "восьмую"}
	gen := []string{"первой", "второй", "третьей", "четвертой", "пятой", "шестой", "седьмой", "восьмой"}
	nom := []string{"первая", "вторая", "третья", "четвертая", "пятая", "шестая", "седьмая", "восьмая"}
	blocked := map[string]bool{}
	for _, w := range strings.Fields("на в во к ко до после перед через за по каждые каждую двадцать тридцать сорок пятьдесят шестьдесят семьдесят восемьдесят девяносто сто") {
		blocked[w] = true
	}
	found := 0
	for i, w := range words {
		previous := ""
		if i > 0 {
			previous = words[i-1]
		}
		if i+1 < len(words) {
			// A temporal ordinal is not a displayed record position, including
			// nominative forms that have no preceding preposition.
			switch words[i+1] {
			case "неделя", "неделю", "недели", "половина", "половину", "половины", "день", "дня", "месяц", "месяца", "час", "часа", "квартал", "год", "года":
				continue
			}
		}
		for j := range acc {
			if (w == acc[j] || w == nom[j]) && !blocked[previous] || w == gen[j] && previous == "у" {
				if found != 0 && found != j+1 {
					return false
				}
				found = j + 1
			}
		}
	}
	return found == position
}
