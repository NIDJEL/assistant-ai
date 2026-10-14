package user

import "time"

type User struct {
	ID                  string    `json:"id"`
	Email               string    `json:"email"`
	Name                string    `json:"name"`
	AvatarURL           string    `json:"avatar_url"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"-"`
	Providers           []string  `json:"providers"`
}
