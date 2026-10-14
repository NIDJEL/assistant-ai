package nlp

import "strings"

const minutePattern = `(?:\d{1,2}|ноль(?:\s+(?:один|два|три|четыре|пять|шесть|семь|восемь|девять))?|(?:двадцать|тридцать|сорок|пятьдесят)(?:\s+(?:один|два|три|четыре|пять|шесть|семь|восемь|девять))?|одиннадцать|двенадцать|тринадцать|четырнадцать|пятнадцать|шестнадцать|семнадцать|восемнадцать|девятнадцать|десять)`

func spokenClock(text string) (*match, int, int, string) {
	ordinals := []string{"первого", "второго", "третьего", "четвертого", "пятого", "шестого", "седьмого", "восьмого", "девятого", "десятого", "одиннадцатого", "двенадцатого"}
	if m := first(`\b(?:в\s+)?(?:половину|половина)\s+(`+strings.Join(ordinals, "|")+`)(?:\s+(утра|вечера|дня|ночи))?\b`, text); m != nil {
		for i, ordinal := range ordinals {
			if ordinal == m.Parts[1] {
				return m, (i + 12) % 12, 30, m.Parts[2]
			}
		}
	}
	m := first(`\b(?:в|к)\s+(`+numberPattern+`)\s+(?:час(?:а|ов)?\s+)?(`+minutePattern+`)(?:\s+минут\w*)?(?:\s+(утра|вечера|дня|ночи))?\b`, text)
	if m == nil {
		return nil, 0, 0, ""
	}
	minute := number(m.Parts[2])
	for _, part := range strings.Fields(m.Parts[2]) {
		switch part {
		case "тридцать":
			minute += 30
		case "сорок":
			minute += 40
		case "пятьдесят":
			minute += 50
		}
	}
	return m, int(number(m.Parts[1])), int(minute), m.Parts[3]
}
