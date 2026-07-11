package repository

type Store interface {
	SessionRepository
	MessageRepository
	AssetRepository
}
