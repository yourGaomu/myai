package service

import (
	"fmt"
	"math"
	"sort"
	"strings"

	retrievalport "myai/core/application/knowledge/retrieval/port"
	domainknowledge "myai/core/domain/knowledge"
)

type ReciprocalRankFusion struct {
	K int
}

var _ retrievalport.RankFusion = ReciprocalRankFusion{}

func NewReciprocalRankFusion(k int) (ReciprocalRankFusion, error) {
	fusion := ReciprocalRankFusion{K: k}
	if err := fusion.validate(); err != nil {
		return ReciprocalRankFusion{}, err
	}
	return fusion, nil
}

func (fusion ReciprocalRankFusion) Fuse(lists []domainknowledge.RankedList, limit int) ([]domainknowledge.FusedItem, error) {
	if err := fusion.validate(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, fmt.Errorf("RRF limit must be positive")
	}
	type accumulated struct {
		item     domainknowledge.FusedItem
		bestRank int
		order    int
	}
	items := make(map[string]*accumulated)
	order := 0
	for _, list := range lists {
		if err := validateRankedList(list); err != nil {
			return nil, err
		}
		seen := make(map[string]struct{}, len(list.Items))
		for _, ranked := range list.Items {
			chunkID := strings.TrimSpace(ranked.ChunkID)
			if _, exists := seen[chunkID]; exists {
				continue
			}
			seen[chunkID] = struct{}{}
			current, exists := items[chunkID]
			if !exists {
				current = &accumulated{
					item: domainknowledge.FusedItem{
						ChunkID: chunkID,
						Channel: list.Channel,
						Origin:  list.Origin,
					},
					bestRank: ranked.Rank,
					order:    order,
				}
				items[chunkID] = current
				order++
			}
			current.item.Score += 1 / float64(fusion.K+ranked.Rank)
			if ranked.Rank < current.bestRank {
				current.bestRank = ranked.Rank
				current.item.Channel = list.Channel
				current.item.Origin = list.Origin
			}
		}
	}
	accumulatedItems := make([]*accumulated, 0, len(items))
	for _, item := range items {
		accumulatedItems = append(accumulatedItems, item)
	}
	sort.SliceStable(accumulatedItems, func(left, right int) bool {
		if math.Abs(accumulatedItems[left].item.Score-accumulatedItems[right].item.Score) > 1e-15 {
			return accumulatedItems[left].item.Score > accumulatedItems[right].item.Score
		}
		if accumulatedItems[left].bestRank != accumulatedItems[right].bestRank {
			return accumulatedItems[left].bestRank < accumulatedItems[right].bestRank
		}
		if accumulatedItems[left].order != accumulatedItems[right].order {
			return accumulatedItems[left].order < accumulatedItems[right].order
		}
		return accumulatedItems[left].item.ChunkID < accumulatedItems[right].item.ChunkID
	})
	if len(accumulatedItems) > limit {
		accumulatedItems = accumulatedItems[:limit]
	}
	result := make([]domainknowledge.FusedItem, 0, len(accumulatedItems))
	for index, item := range accumulatedItems {
		item.item.Rank = index + 1
		result = append(result, item.item)
	}
	return result, nil
}

func (fusion ReciprocalRankFusion) validate() error {
	if fusion.K < 1 {
		return fmt.Errorf("RRF k must be positive")
	}
	return nil
}

func validateRankedList(list domainknowledge.RankedList) error {
	if list.Channel != domainknowledge.RetrievalChannelVector && list.Channel != domainknowledge.RetrievalChannelKeyword {
		return fmt.Errorf("unsupported retrieval channel %q", list.Channel)
	}
	if list.Origin != domainknowledge.RetrievalOriginLocal && list.Origin != domainknowledge.RetrievalOriginRemote {
		return fmt.Errorf("unsupported retrieval origin %q", list.Origin)
	}
	for _, item := range list.Items {
		if strings.TrimSpace(item.ChunkID) == "" {
			return fmt.Errorf("ranked item chunk id is required")
		}
		if item.Rank < 1 {
			return fmt.Errorf("ranked item rank must be positive")
		}
		if math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
			return fmt.Errorf("ranked item score must be finite")
		}
	}
	return nil
}
