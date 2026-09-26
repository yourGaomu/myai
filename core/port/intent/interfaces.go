package intent

import "context"

type Store interface {
	LoadConfig(context.Context) (Config, error)
	SaveConfig(context.Context, Config) error
	SaveTrace(context.Context, Trace) error
	ListTraces(context.Context, string, int) ([]Trace, error)
	GetTrace(context.Context, string) (Trace, error)
	ClearTraces(context.Context, string) error
}

type Client interface {
	BuildRequest(string, []string, string) ([]byte, error)
	Send(context.Context, Config, []byte) ([]byte, int, error)
}
