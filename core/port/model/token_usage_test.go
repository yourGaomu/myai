package model

import "testing"

func TestTokenUsageAddPromptCacheCoverage(t *testing.T) {
	empty := TokenUsage{}
	unreported := empty.AsSample()
	legacyUnknown := TokenUsage{PromptTokens: 3, TotalTokens: 3, Available: true}
	legacyHit := TokenUsage{PromptCachedTokens: 2}
	zero := TokenUsage{PromptCacheReported: true}
	hit := TokenUsage{PromptTokens: 7, TotalTokens: 7, PromptCachedTokens: 4, PromptCacheReported: true, Available: true}

	for _, sample := range []TokenUsage{empty, unreported, legacyUnknown, legacyHit, zero, hit} {
		if empty.Add(sample) != sample || sample.Add(empty) != sample {
			t.Errorf("empty accumulator changed sample: %+v", sample)
		}
	}
	for _, tc := range []struct {
		name             string
		samples          []TokenUsage
		count            int
		known            bool
		incomplete       bool
		providerReported bool
	}{
		{"explicit zero", []TokenUsage{zero.AsSample()}, 0, true, false, true},
		{"reported hits", []TokenUsage{hit, zero, hit}, 8, true, false, true},
		{"legacy positive", []TokenUsage{legacyHit.AsSample(), legacyHit}, 4, true, false, false},
		{"wholly unreported", []TokenUsage{unreported, legacyUnknown}, 0, false, true, false},
		{"unreported before zero", []TokenUsage{unreported, zero}, 0, true, true, true},
		{"unreported after hit", []TokenUsage{hit, unreported}, 4, true, true, true},
		{"legacy positive and missing", []TokenUsage{legacyHit, legacyUnknown, hit}, 6, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := empty
			for _, usage := range tc.samples {
				got = got.Add(usage)
			}
			if got.PromptCachedTokens != tc.count || got.HasPromptCacheCount() != tc.known || got.PromptCacheIncomplete != tc.incomplete || got.PromptCacheReported != tc.providerReported {
				t.Fatalf("unexpected cache usage: %+v", got)
			}
		})
	}

	got := hit.Add(legacyUnknown).Add(unreported)
	if got.PromptTokens != 10 || got.TotalTokens != 10 || !got.Available {
		t.Fatalf("ordinary totals changed: %+v", got)
	}
	if want := hit.Add(legacyUnknown.Add(unreported)); got != want {
		t.Fatalf("cache coverage depends on grouping: got %+v, want %+v", got, want)
	}
}
