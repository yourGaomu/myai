package sqliteruntime

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	_ "github.com/asg017/sqlite-vec-go-bindings/ncruces"
	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

var configureOnce sync.Once

// Configure enables the WebAssembly features required by the sqlite-vec build.
// It must run before the first ncruces SQLite connection is opened.
func Configure() {
	configureOnce.Do(func() {
		sqlite3.RuntimeConfig = wazero.NewRuntimeConfig().WithCoreFeatures(
			api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
		)
	})
}

func DataSourceName(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite path: %w", err)
	}
	slashPath := filepath.ToSlash(absolute)
	// Opaque keeps the Windows form file:C:/... that ncruces accepts while
	// still letting net/url escape spaces, fragments and query characters.
	uri := url.URL{Scheme: "file", Opaque: slashPath}
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_pragma", "foreign_keys(ON)")
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()
	return uri.String(), nil
}
