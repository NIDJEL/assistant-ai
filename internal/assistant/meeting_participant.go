package assistant

import "strings"

// A group label may describe a personal meeting, but is not a made-up person.
// This is only used after real friend/tag resolution has already failed.
func collectiveParticipant(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "команда", "командой", "команды", "родители", "родителями", "родителей", "семья", "семьей", "семьёй", "семьи", "коллеги", "коллегами", "коллег":
		return true
	}
	return false
}

func personalParticipantLabel(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "дизайнером":
		return "Дизайнер"
	case "бухгалтером":
		return "Бухгалтер"
	case "редактором":
		return "Редактор"
	}
	return name
}
