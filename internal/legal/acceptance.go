// Package legal defines the documents explicitly confirmed on the auth screen.
package legal

import (
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Version changes whenever the published terms, policy or consent changes.
const Version = "2026-10-20"

type Input struct {
	TermsAccepted       bool   `json:"terms_accepted"`
	PersonalDataConsent bool   `json:"personal_data_consent"`
	LegalVersion        string `json:"legal_version"`
}

// Acceptance contains server-assigned evidence, never a client-provided date.
type Acceptance struct {
	Version    string    `json:"version"`
	AcceptedAt time.Time `json:"accepted_at"`
	Action     string    `json:"action"`
}

func (i Input) Confirm(now time.Time, action string) (Acceptance, error) {
	if !i.TermsAccepted || !i.PersonalDataConsent {
		return Acceptance{}, &domain.AppError{Code: "legal_acceptance_required", Message: "Подтвердите ознакомление с документами и отдельно дайте согласие на обработку персональных данных.", Status: 422}
	}
	if i.LegalVersion != Version {
		return Acceptance{}, &domain.AppError{Code: "legal_version_changed", Message: "Документы обновились. Обновите страницу, прочитайте их и подтвердите согласие ещё раз.", Status: 422}
	}
	return Acceptance{Version: Version, AcceptedAt: now, Action: action}, nil
}
