package agent

import (
	"context"

	pluginruntime "myai/core/plugin"
)

// PluginManagerFacade keeps the remote transport dependent on the plugin
// lifecycle operations it actually exposes, rather than the whole Application.
type PluginManagerFacade interface {
	Root() string
	List() []pluginruntime.Info
	Reload(context.Context) error
	SetEnabled(context.Context, string, bool) error
}
