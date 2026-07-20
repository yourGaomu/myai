package service

import (
	"math"
	"testing"

	domainknowledge "myai/core/domain/knowledge"
)

func TestReciprocalRankFusionDeduplicatesAndRanksDeterministically(t *testing.T) {
	fusion, err := NewReciprocalRankFusion(60)
	if err != nil {
		t.Fatal(err)
	}
	items, err := fusion.Fuse([]domainknowledge.RankedList{
		{
			Channel: domainknowledge.RetrievalChannelVector,
			Origin:  domainknowledge.RetrievalOriginLocal,
			Items: []domainknowledge.RankedItem{
				{ChunkID: "chunk-a", Rank: 1, Score: 0.9},
				{ChunkID: "chunk-b", Rank: 2, Score: 0.8},
			},
		},
		{
			Channel: domainknowledge.RetrievalChannelKeyword,
			Origin:  domainknowledge.RetrievalOriginLocal,
			Items: []domainknowledge.RankedItem{
				{ChunkID: "chunk-b", Rank: 1, Score: 10},
				{ChunkID: "chunk-a", Rank: 2, Score: 8},
			},
		},
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ChunkID != "chunk-a" || items[1].ChunkID != "chunk-b" {
		t.Fatalf("unexpected fused order: %#v", items)
	}
	want := 1.0/61 + 1.0/62
	if math.Abs(items[0].Score-want) > 1e-12 || items[0].Rank != 1 || items[1].Rank != 2 {
		t.Fatalf("unexpected fused scores: %#v", items)
	}
}
