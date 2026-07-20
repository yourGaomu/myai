package port

import domainknowledge "myai/core/domain/knowledge"

type LocalQualityGate interface {
	Evaluate(input domainknowledge.LocalQualityInput) (domainknowledge.LocalQualityDecision, error)
}

type RankFusion interface {
	Fuse(lists []domainknowledge.RankedList, limit int) ([]domainknowledge.FusedItem, error)
}
