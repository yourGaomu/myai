package service

import (
	"fmt"
	"math"

	retrievalport "myai/core/application/knowledge/retrieval/port"
	domainknowledge "myai/core/domain/knowledge"
)

type LocalQualityGate struct {
	MinResults int
	MinScore   float64
}

var _ retrievalport.LocalQualityGate = LocalQualityGate{}

func NewLocalQualityGate(minResults int, minScore float64) (LocalQualityGate, error) {
	gate := LocalQualityGate{MinResults: minResults, MinScore: minScore}
	if err := gate.validate(); err != nil {
		return LocalQualityGate{}, err
	}
	return gate, nil
}

func (gate LocalQualityGate) Evaluate(input domainknowledge.LocalQualityInput) (domainknowledge.LocalQualityDecision, error) {
	if err := gate.validate(); err != nil {
		return domainknowledge.LocalQualityDecision{}, err
	}
	if input.ResultCount < 0 {
		return domainknowledge.LocalQualityDecision{}, fmt.Errorf("local quality result count must not be negative")
	}
	if math.IsNaN(input.TopVectorScore) || math.IsInf(input.TopVectorScore, 0) {
		return domainknowledge.LocalQualityDecision{}, fmt.Errorf("local quality top vector score must be finite")
	}
	decision := domainknowledge.LocalQualityDecision{
		Healthy:        input.Healthy,
		ProfileMatched: input.ProfileMatched,
		ResultCount:    input.ResultCount,
		TopVectorScore: input.TopVectorScore,
	}
	if !input.Healthy {
		decision.Reasons = append(decision.Reasons, "local vector index is unavailable")
	}
	if !input.ProfileMatched {
		decision.Reasons = append(decision.Reasons, "local index profile does not match the request")
	}
	if input.ResultCount < gate.MinResults {
		decision.Reasons = append(decision.Reasons, fmt.Sprintf("local results %d are below minimum %d", input.ResultCount, gate.MinResults))
	}
	if input.TopVectorScore < gate.MinScore {
		decision.Reasons = append(decision.Reasons, fmt.Sprintf("local top vector score %.4f is below minimum %.4f", input.TopVectorScore, gate.MinScore))
	}
	decision.Passed = len(decision.Reasons) == 0
	return decision, nil
}

func (gate LocalQualityGate) validate() error {
	if gate.MinResults < 1 {
		return fmt.Errorf("local quality minimum results must be positive")
	}
	if math.IsNaN(gate.MinScore) || math.IsInf(gate.MinScore, 0) {
		return fmt.Errorf("local quality minimum score must be finite")
	}
	if gate.MinScore <= 0 || gate.MinScore > 1 {
		return fmt.Errorf("local quality minimum score must be greater than 0 and at most 1")
	}
	return nil
}
