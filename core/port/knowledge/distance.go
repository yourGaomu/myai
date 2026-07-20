package knowledge

type DistanceMetric interface {
	ID() string
	Distance(left []float32, right []float32) (float64, error)
	RequiresNormalizedVectors() bool
}

type DistanceMetricRegistry interface {
	Get(metricID string) (DistanceMetric, bool)
	List() []DistanceMetric
}
