package po

import "time"

type Document struct {
	ID         string     `bson:"_id"`
	UserID     string     `bson:"user_id"`
	DeviceID   string     `bson:"device_id"`
	ClientName string     `bson:"client_name"`
	RemoteAddr string     `bson:"remote_addr"`
	CreatedAt  time.Time  `bson:"created_at"`
	LastSeenAt time.Time  `bson:"last_seen_at"`
	ExpiresAt  time.Time  `bson:"expires_at"`
	RevokedAt  *time.Time `bson:"revoked_at,omitempty"`
}
