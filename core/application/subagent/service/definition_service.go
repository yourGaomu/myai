package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	subagentapi "myai/core/application/subagent/api"
	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

var _ subagentapi.Service = (*Service)(nil)

func (service *Service) Bootstrap(ctx context.Context, _ subagentcommand.BootstrapDefinitions) (subagentresult.Definitions, error) {
	if service == nil || service.Registry == nil {
		return subagentresult.Definitions{}, errors.New("subagent definition registry is nil")
	}
	for _, definition := range domainsubagent.BuiltinDefinitions() {
		definition = definition.Normalized()
		if err := definition.Validate(); err != nil {
			return subagentresult.Definitions{}, err
		}
		if err := service.Registry.Register(definition); err != nil {
			return subagentresult.Definitions{}, err
		}
	}
	if service.Definitions != nil {
		custom, err := service.Definitions.ListDefinitions(ctx, false)
		if err != nil {
			return subagentresult.Definitions{}, err
		}
		for _, definition := range custom {
			definition = definition.Normalized()
			if definition.Deleted || !definition.Enabled {
				continue
			}
			if err := definition.Validate(); err != nil {
				return subagentresult.Definitions{}, err
			}
			if err := service.Registry.Register(definition); err != nil {
				return subagentresult.Definitions{}, err
			}
		}
	}
	return service.ListDefinitions(ctx)
}

func (service *Service) CreateDefinition(ctx context.Context, command subagentcommand.CreateDefinition) (subagentresult.Definition, error) {
	if service == nil || service.Definitions == nil || service.Registry == nil || service.IDs == nil {
		return subagentresult.Definition{}, errors.New("subagent definition service is not configured")
	}
	definition := command.Definition.Normalized()
	if definition.ID == "" {
		definition.ID = service.IDs.NewID()
	}
	if existing, ok := service.Registry.Get(definition.ID); ok && !existing.Deleted {
		return subagentresult.Definition{}, errors.New("subagent definition already exists: " + definition.ID)
	}
	definition.Source = domainsubagent.DefinitionSourceUser
	definition.Version = 1
	now := service.now()
	definition.CreatedAt = now
	definition.UpdatedAt = now
	if err := definition.Validate(); err != nil {
		return subagentresult.Definition{}, err
	}
	if err := service.Definitions.SaveDefinition(ctx, definition); err != nil {
		return subagentresult.Definition{}, err
	}
	if definition.Enabled {
		if err := service.Registry.Register(definition); err != nil {
			return subagentresult.Definition{}, err
		}
	}
	return subagentresult.Definition{Value: domainsubagent.CloneDefinition(definition)}, nil
}

func (service *Service) UpdateDefinition(ctx context.Context, command subagentcommand.UpdateDefinition) (subagentresult.Definition, error) {
	if service == nil || service.Definitions == nil || service.Registry == nil {
		return subagentresult.Definition{}, errors.New("subagent definition service is not configured")
	}
	definition := command.Definition.Normalized()
	existing, err := service.Definitions.GetDefinition(ctx, definition.ID)
	if err != nil {
		return subagentresult.Definition{}, err
	}
	if existing.Source == domainsubagent.DefinitionSourceBuiltin {
		return subagentresult.Definition{}, errors.New("built-in subagent definitions cannot be modified")
	}
	definition.Source = existing.Source
	definition.CreatedAt = existing.CreatedAt
	definition.UpdatedAt = service.now()
	definition.Version = existing.Version + 1
	definition.Deleted = false
	definition.DeletedAt = nil
	if err := definition.Validate(); err != nil {
		return subagentresult.Definition{}, err
	}
	if err := service.Definitions.SaveDefinition(ctx, definition); err != nil {
		return subagentresult.Definition{}, err
	}
	if definition.Enabled {
		if err := service.Registry.Register(definition); err != nil {
			return subagentresult.Definition{}, err
		}
	} else {
		service.Registry.Remove(definition.ID)
	}
	return subagentresult.Definition{Value: domainsubagent.CloneDefinition(definition)}, nil
}

func (service *Service) DeleteDefinition(ctx context.Context, command subagentcommand.DeleteDefinition) error {
	if service == nil || service.Definitions == nil || service.Registry == nil {
		return errors.New("subagent definition service is not configured")
	}
	id := strings.TrimSpace(command.DefinitionID)
	if id == "" {
		return errors.New("subagent definition id is required")
	}
	if existing, ok := service.Registry.Get(id); ok && existing.Source == domainsubagent.DefinitionSourceBuiltin {
		return errors.New("built-in subagent definitions cannot be deleted")
	}
	if err := service.Definitions.DeleteDefinition(ctx, id); err != nil {
		return err
	}
	service.Registry.Remove(id)
	return nil
}

func (service *Service) ListDefinitions(ctx context.Context) (subagentresult.Definitions, error) {
	if service == nil || service.Registry == nil {
		return subagentresult.Definitions{}, errors.New("subagent definition registry is nil")
	}
	items := service.Registry.List()
	if service.Definitions != nil {
		stored, err := service.Definitions.ListDefinitions(ctx, false)
		if err != nil {
			return subagentresult.Definitions{}, err
		}
		known := make(map[string]struct{}, len(items))
		for _, item := range items {
			known[item.ID] = struct{}{}
		}
		for _, item := range stored {
			if item.Deleted {
				continue
			}
			if _, exists := known[item.ID]; exists {
				continue
			}
			items = append(items, domainsubagent.CloneDefinition(item))
		}
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return subagentresult.Definitions{Items: items}, nil
}

func definitionNotFound(id string) error {
	return errors.Join(subagentport.ErrNotFound, errors.New("subagent definition not found: "+id))
}
