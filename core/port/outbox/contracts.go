package outbox

import "context"

// Journal is single-owner durable storage. Append commits before returning.
// ACK is legal only after all immutable backend writes succeeded. Done receipts
// are retained for a bounded window; outside it backend immutability is required.
type Journal interface {
	Append(context.Context, Batch) (Entry, error)
	Pending(context.Context) ([]Entry, error)
	Ack(context.Context, int64) error
	Block(context.Context, string) error
	Resume(context.Context, string) error
	Blocked(context.Context, string) (bool, error)
	Stats(context.Context) (Stats, error)
}

type Sink interface {
	Apply(context.Context, Batch) error
}
