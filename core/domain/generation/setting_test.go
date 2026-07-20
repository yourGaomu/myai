package generation

import (
	"math"
	"testing"
)

func TestResolveAppliesSessionThenModelThenSystemDefaults(t *testing.T) {
	modelTemperature := 0.4
	modelTopP := 0.8
	modelTokens := 4096
	sessionTemperature := 0.0
	sessionTokens := 1024

	resolved, err := Resolve(
		Settings{Temperature: &modelTemperature, TopP: &modelTopP, MaxOutputTokens: &modelTokens},
		Settings{Temperature: &sessionTemperature, MaxOutputTokens: &sessionTokens},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Temperature != 0 || resolved.TopP != modelTopP || resolved.MaxOutputTokens != sessionTokens {
		t.Fatalf("Resolve() = %#v", resolved)
	}
}

func TestValidateRejectsNonFiniteValues(t *testing.T) {
	value := math.NaN()
	if err := Validate(Settings{Temperature: &value}); err == nil {
		t.Fatal("expected NaN temperature to be rejected")
	}
}

func TestCloneDoesNotSharePointerValues(t *testing.T) {
	temperature := 0.7
	cloned := Clone(Settings{Temperature: &temperature})
	temperature = 1.2
	if cloned.Temperature == nil || *cloned.Temperature != 0.7 {
		t.Fatalf("Clone() = %#v", cloned)
	}
}
