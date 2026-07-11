package authorization

import "time"

type ClientAuthorization struct {
	ID         string
	UserID     string
	DeviceID   string
	ClientName string
	RemoteAddr string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

func (a ClientAuthorization) ActiveAt(now time.Time) bool {
	if a.RevokedAt != nil {
		return false
	}
	return a.ExpiresAt.IsZero() || a.ExpiresAt.After(now)
}
