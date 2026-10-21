package social

import "strings"

// Exact account names outrank grammatical forms. Name resolution still considers
// every confirmed friend and asks for clarification when several people match.
func SameName(a, b string) bool {
	left, right := nameWords(a), nameWords(b)
	return len(left) == len(right) && matchNameWords(left, right, false)
}

func NameMatches(name, query string) bool {
	return matchNameWords(nameWords(name), nameWords(query), true)
}

func matchNameWords(name, query []string, inflected bool) bool {
	if len(query) == 0 || len(query) > len(name) {
		return false
	}
	matched := make([]int, len(name))
	for i := range matched {
		matched[i] = -1
	}
	var match func(int, []bool) bool
	match = func(index int, seen []bool) bool {
		for i, word := range name {
			if !seen[i] && (word == query[index] || inflected && russianNameForm(word, query[index])) {
				seen[i] = true
				if matched[i] < 0 || match(matched[i], seen) {
					matched[i] = index
					return true
				}
			}
		}
		return false
	}
	for index := range query {
		if !match(index, make([]bool, len(name))) {
			return false
		}
	}
	return true
}

// Generate bounded case endings from a known contact, rather than stemming the
// request or guessing another person's spelling. Nicknames need an explicit tag.
func russianNameForm(name, query string) bool {
	runes := []rune(name)
	if len(runes) < 3 {
		return false
	}
	for _, r := range runes {
		if r < 'а' || r > 'я' {
			return false
		}
	}
	stem, endings := name, []string{}
	switch {
	case name == "павел":
		stem, endings = "павл", []string{"а", "у", "ом", "е"}
	case name == "лев":
		stem, endings = "льв", []string{"а", "у", "ом", "е"}
	case name == "петр":
		endings = []string{"а", "у", "ом", "е"}
	case strings.HasSuffix(name, "ова"), strings.HasSuffix(name, "ева"), strings.HasSuffix(name, "ина"):
		stem, endings = string(runes[:len(runes)-1]), []string{"ой", "ою", "у", "е", "ы"}
	case strings.HasSuffix(name, "ский"), strings.HasSuffix(name, "цкий"), strings.HasSuffix(name, "ый"), strings.HasSuffix(name, "ой"):
		stem, endings = string(runes[:len(runes)-2]), []string{"ого", "ому", "ым", "ом"}
	case strings.HasSuffix(name, "ий"):
		stem, endings = string(runes[:len(runes)-1]), []string{"я", "ю", "ем", "и"}
	case strings.HasSuffix(name, "ей"), strings.HasSuffix(name, "ь"), strings.HasSuffix(name, "й"):
		stem, endings = string(runes[:len(runes)-1]), []string{"я", "ю", "ем", "е"}
	case strings.HasSuffix(name, "а"):
		stem, endings = string(runes[:len(runes)-1]), []string{"ы", "и", "е", "у", "ой", "ою"}
	case strings.HasSuffix(name, "я"):
		stem, endings = string(runes[:len(runes)-1]), []string{"и", "е", "ю", "ей", "ею"}
	case strings.HasSuffix(name, "ов"), strings.HasSuffix(name, "ев"), strings.HasSuffix(name, "ин"):
		endings = []string{"а", "у", "ым", "е"}
	case strings.ContainsRune("бвгджзклмнпрстфхцчшщ", runes[len(runes)-1]):
		endings = []string{"а", "у", "ом", "е"}
	}
	for _, ending := range endings {
		if query == stem+ending {
			return true
		}
	}
	return false
}
