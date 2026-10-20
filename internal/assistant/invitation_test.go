package assistant

import "testing"

func TestInvitationGuardOnlyRaisesExplicitInvitationRequirements(t *testing.T) {
	for _, text := range []string{"Пригласи Ивана на новую встречу завтра", "Не забудь пригласить Ольгу", "Отправь Анне приглашение на встречу", "Позовите команду завтра в три"} {
		if !requestsInvitation(text) {
			t.Error("invitation omitted", text)
		}
	}
	for _, text := range []string{"Запиши встречу с Иваном", "Не приглашай Ивана, просто сохрани встречу", "Сохрани встречу без приглашений", "Не надо никого приглашать, запиши лично"} {
		if requestsInvitation(text) {
			t.Error("ordinary personal meeting treated as an invitation", text)
		}
	}
}

func TestInvitationIntentDistinguishesRefusalFromReminder(t *testing.T) {
	for _, text := range []string{"Не приглашай Матвея, просто сохрани встречу", "Запиши без приглашений", "Не надо отправлять приглашение"} {
		requested, declined := invitationIntent(text)
		if requested || !declined {
			t.Fatal(text, requested, declined)
		}
	}
	requested, declined := invitationIntent("Не забудь отправить приглашение")
	if !requested || declined {
		t.Fatal(requested, declined)
	}
}
