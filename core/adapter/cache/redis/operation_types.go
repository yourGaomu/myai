package redis

type SortedSetMember struct {
	Value string
	Score float64
}

type ScoreRange struct {
	Min     string
	Max     string
	Offset  int64
	Count   int64
	Reverse bool
}
