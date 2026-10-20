package assistant

import (
	"regexp"
	"strings"
)

var invitationWords = regexp.MustCompile(`[\p{L}]+`)

// This guard only makes creation stricter when a model drops the invitation
// requirement. It never selects people, grants consent or sends invitations.
func requestsInvitation(text string) bool {
	requested, _ := invitationIntent(text)
	return requested
}

func invitationIntent(text string) (requested, declined bool) {
	words := invitationWords.FindAllString(strings.ToLower(text), -1)
	for i, word := range words {
		invitation := strings.HasPrefix(word, "приглас") || strings.HasPrefix(word, "приглаша") || in(word, "позови", "позовите", "согласуй", "согласуйте")
		if strings.HasPrefix(word, "приглашени") {
			for j := max(0, i-4); j < i; j++ {
				invitation = invitation || words[j] == "без" || strings.HasPrefix(words[j], "отправ") || strings.HasPrefix(words[j], "пошли")
			}
		}
		if !invitation {
			continue
		}
		negated := false
		for j := max(0, i-4); j < i; j++ {
			negated = negated || words[j] == "без" || words[j] == "не" && !in(words[j+1], "забудь", "забудьте", "забудем", "только")
		}
		requested, declined = requested || !negated, declined || negated
	}
	return requested, declined
}
