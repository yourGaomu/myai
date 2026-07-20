package port

import retrievalresult "myai/core/application/chat/retrieval/result"

type TriggerPolicy interface {
	ShouldRetrieve(input string) bool
}

type ContextFormatter interface {
	Format(query string, hits []retrievalresult.ContextHit) string
}
