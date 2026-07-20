package service

import (
	"testing"

	domainknowledge "myai/core/domain/knowledge"
)

func TestLocalQualityGateAppliesAllThresholds(t *testing.T) {
	gate, err := NewLocalQualityGate(3, 0.55)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := gate.Evaluate(domainknowledge.LocalQualityInput{
		Healthy:        true,
		ProfileMatched: true,
		ResultCount:    3,
		TopVectorScore: 0.55,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Passed || len(decision.Reasons) != 0 {
		t.Fatalf("expected quality pass, got %#v", decision)
	}
	decision, err = gate.Evaluate(domainknowledge.LocalQualityInput{
		Healthy:        false,
		ProfileMatched: false,
		ResultCount:    1,
		TopVectorScore: 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Passed || len(decision.Reasons) != 4 {
		t.Fatalf("expected four failure reasons, got %#v", decision)
	}
}
