package generation

import (
	"fmt"
	"math"
)

const (
	DefaultTemperature     = 0.7
	DefaultTopP            = 1.0
	DefaultMaxOutputTokens = 2048
	MaxOutputTokensLimit   = 131072
)

type Settings struct {
	Temperature     *float64
	TopP            *float64
	MaxOutputTokens *int
}

type ResolvedSettings struct {
	Temperature     float64
	TopP            float64
	MaxOutputTokens int
}

func (settings ResolvedSettings) Validate() error {
	return Validate(Settings{
		Temperature:     &settings.Temperature,
		TopP:            &settings.TopP,
		MaxOutputTokens: &settings.MaxOutputTokens,
	})
}

func SystemDefaults() ResolvedSettings {
	return ResolvedSettings{
		Temperature:     DefaultTemperature,
		TopP:            DefaultTopP,
		MaxOutputTokens: DefaultMaxOutputTokens,
	}
}

func Clone(settings Settings) Settings {
	return Settings{
		Temperature:     cloneFloat64(settings.Temperature),
		TopP:            cloneFloat64(settings.TopP),
		MaxOutputTokens: cloneInt(settings.MaxOutputTokens),
	}
}

func Validate(settings Settings) error {
	if settings.Temperature != nil {
		value := *settings.Temperature
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("temperature must be finite")
		}
		if value < 0 || value > 2 {
			return fmt.Errorf("temperature must be between 0 and 2")
		}
	}
	if settings.TopP != nil {
		value := *settings.TopP
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("top_p must be finite")
		}
		if value < 0 || value > 1 {
			return fmt.Errorf("top_p must be between 0 and 1")
		}
	}
	if settings.MaxOutputTokens != nil {
		value := *settings.MaxOutputTokens
		if value < 1 || value > MaxOutputTokensLimit {
			return fmt.Errorf("max_output_tokens must be between 1 and %d", MaxOutputTokensLimit)
		}
	}
	return nil
}

func Resolve(
	modelDefaults Settings,
	sessionOverrides Settings,
) (ResolvedSettings, error) {
	if err := Validate(modelDefaults); err != nil {
		return ResolvedSettings{}, fmt.Errorf("invalid model generation settings: %w", err)
	}
	if err := Validate(sessionOverrides); err != nil {
		return ResolvedSettings{}, fmt.Errorf("invalid session generation settings: %w", err)
	}

	resolved := SystemDefaults()
	apply(&resolved, modelDefaults)
	apply(&resolved, sessionOverrides)
	return resolved, nil
}

func apply(target *ResolvedSettings, source Settings) {
	if source.Temperature != nil {
		target.Temperature = *source.Temperature
	}
	if source.TopP != nil {
		target.TopP = *source.TopP
	}
	if source.MaxOutputTokens != nil {
		target.MaxOutputTokens = *source.MaxOutputTokens
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
