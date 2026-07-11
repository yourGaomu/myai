package mapper

import (
	"myai/core/adapter/persistence/mongo/authorization/po"
	domainauthorization "myai/core/domain/authorization"
)

func DocumentFromDomain(authorization domainauthorization.ClientAuthorization) po.Document {
	return po.Document{
		ID:         authorization.ID,
		UserID:     authorization.UserID,
		DeviceID:   authorization.DeviceID,
		ClientName: authorization.ClientName,
		RemoteAddr: authorization.RemoteAddr,
		CreatedAt:  authorization.CreatedAt,
		LastSeenAt: authorization.LastSeenAt,
		ExpiresAt:  authorization.ExpiresAt,
		RevokedAt:  authorization.RevokedAt,
	}
}

func DomainFromDocument(value po.Document) domainauthorization.ClientAuthorization {
	return domainauthorization.ClientAuthorization{
		ID:         value.ID,
		UserID:     value.UserID,
		DeviceID:   value.DeviceID,
		ClientName: value.ClientName,
		RemoteAddr: value.RemoteAddr,
		CreatedAt:  value.CreatedAt,
		LastSeenAt: value.LastSeenAt,
		ExpiresAt:  value.ExpiresAt,
		RevokedAt:  value.RevokedAt,
	}
}
