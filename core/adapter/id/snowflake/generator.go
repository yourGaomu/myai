package snowflake

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	knowledgeport "myai/core/port/knowledge"
)

const (
	nodeBits       = 10
	sequenceBits   = 12
	maxNodeID      = int64(1<<nodeBits - 1)
	maxSequence    = int64(1<<sequenceBits - 1)
	timestampShift = nodeBits + sequenceBits
	nodeShift      = sequenceBits
)

var defaultEpoch = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

type Generator struct {
	mu            sync.Mutex
	nodeID        int64
	epochMillis   int64
	lastTimestamp int64
	sequence      int64
	now           func() time.Time
}

var _ knowledgeport.IDGenerator = (*Generator)(nil)

func New(nodeID int64) (*Generator, error) {
	return NewWithEpoch(nodeID, defaultEpoch)
}

func NewWithEpoch(nodeID int64, epoch time.Time) (*Generator, error) {
	return newWithClock(nodeID, epoch, time.Now)
}

func newWithClock(nodeID int64, epoch time.Time, now func() time.Time) (*Generator, error) {
	if nodeID < 0 || nodeID > maxNodeID {
		return nil, fmt.Errorf("snowflake node id must be between 0 and %d", maxNodeID)
	}
	if epoch.IsZero() {
		return nil, fmt.Errorf("snowflake epoch is required")
	}
	if now == nil {
		return nil, fmt.Errorf("snowflake clock is required")
	}
	return &Generator{
		nodeID:        nodeID,
		epochMillis:   epoch.UTC().UnixMilli(),
		lastTimestamp: -1,
		now:           now,
	}, nil
}

func (g *Generator) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	timestamp := g.now().UTC().UnixMilli() - g.epochMillis
	if timestamp < 0 {
		timestamp = 0
	}
	// A logical timestamp keeps IDs monotonic when the wall clock moves backward.
	if timestamp < g.lastTimestamp {
		timestamp = g.lastTimestamp
	}

	if timestamp == g.lastTimestamp {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			timestamp = g.lastTimestamp + 1
		}
	} else {
		g.sequence = 0
	}

	g.lastTimestamp = timestamp
	id := timestamp<<timestampShift | g.nodeID<<nodeShift | g.sequence
	return strconv.FormatInt(id, 10)
}
