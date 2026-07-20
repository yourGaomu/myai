package milvus

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"

	domainknowledge "myai/core/domain/knowledge"
)

func metricType(metricID string) (entity.MetricType, error) {
	switch strings.ToLower(strings.TrimSpace(metricID)) {
	case "cosine":
		return entity.COSINE, nil
	case "l2", "euclidean":
		return entity.L2, nil
	case "ip", "inner_product", "dot":
		return entity.IP, nil
	default:
		return "", fmt.Errorf("unsupported Milvus distance metric %q", metricID)
	}
}

func buildIndex(definition domainknowledge.VectorIndexDefinition) (entity.Index, error) {
	metric, err := metricType(definition.DistanceMetricID)
	if err != nil {
		return nil, err
	}
	switch strings.ToUpper(option(definition.Options, "index_type", "AUTOINDEX")) {
	case "AUTOINDEX":
		return entity.NewIndexAUTOINDEX(metric)
	case "FLAT":
		return entity.NewIndexFlat(metric)
	case "HNSW":
		m, err := intOption(definition.Options, "m", 16)
		if err != nil {
			return nil, err
		}
		efConstruction, err := intOption(definition.Options, "ef_construction", 200)
		if err != nil {
			return nil, err
		}
		return entity.NewIndexHNSW(metric, m, efConstruction)
	case "IVF_FLAT":
		nlist, err := intOption(definition.Options, "nlist", 1024)
		if err != nil {
			return nil, err
		}
		return entity.NewIndexIvfFlat(metric, nlist)
	default:
		return nil, fmt.Errorf("unsupported Milvus index type %q", definition.Options["index_type"])
	}
}

func buildSearchParam(options map[string]string) (entity.SearchParam, error) {
	switch strings.ToUpper(option(options, "index_type", "AUTOINDEX")) {
	case "AUTOINDEX":
		level, err := intOption(options, "search_level", 1)
		if err != nil {
			return nil, err
		}
		return entity.NewIndexAUTOINDEXSearchParam(level)
	case "FLAT":
		return entity.NewIndexFlatSearchParam()
	case "HNSW":
		ef, err := intOption(options, "ef", 64)
		if err != nil {
			return nil, err
		}
		return entity.NewIndexHNSWSearchParam(ef)
	case "IVF_FLAT":
		nprobe, err := intOption(options, "nprobe", 16)
		if err != nil {
			return nil, err
		}
		return entity.NewIndexIvfFlatSearchParam(nprobe)
	default:
		return nil, fmt.Errorf("unsupported Milvus index type %q", options["index_type"])
	}
}

func indexesCompatible(current entity.Index, desired entity.Index) bool {
	if current == nil || desired == nil || current.IndexType() != desired.IndexType() {
		return false
	}
	currentParams := current.Params()
	for key, desiredValue := range desired.Params() {
		if currentParams[key] != desiredValue {
			return false
		}
	}
	return true
}

func option(options map[string]string, key string, fallback string) string {
	if value := strings.TrimSpace(options[key]); value != "" {
		return value
	}
	return fallback
}

func intOption(options map[string]string, key string, fallback int) (int, error) {
	value := strings.TrimSpace(options[key])
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("Milvus option %s must be a positive integer", key)
	}
	return parsed, nil
}
