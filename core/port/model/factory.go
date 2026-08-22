package model

import domainmodel "myai/core/domain/model"

type Factory interface {
	CreateModel(config CreationConfig) (ChatModelPort, error)
	ValidateConfig(config CreationConfig) error
	SupportsProtocol(protocol domainmodel.Protocol) bool
}

// ProtocolAdapter encapsulates the SDK-specific construction rules for one
// model protocol. Application services should depend on Factory instead of a
// concrete adapter or an SDK package.
type ProtocolAdapter interface {
	Protocol() domainmodel.Protocol
	ValidateConfig(config CreationConfig) error
	CreateModel(config CreationConfig) (ChatModelPort, error)
}
