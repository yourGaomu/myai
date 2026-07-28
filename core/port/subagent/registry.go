package subagent

import domainsubagent "myai/core/domain/subagent"

type DefinitionRegistry interface {
	Get(definitionID string) (domainsubagent.Definition, bool)
	List() []domainsubagent.Definition
	Register(definition domainsubagent.Definition) error
	Remove(definitionID string)
}
