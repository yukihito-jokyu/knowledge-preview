package domain

import "time"

type OAuthUser struct {
	ID          string
	DisplayName string
	Email       string
}

type OAuthState struct {
	ReturnTo string
	Expires  time.Time
}

type RefreshSession struct {
	SessionID string
	FamilyID  string
	UserID    string
	Expires   time.Time
}
