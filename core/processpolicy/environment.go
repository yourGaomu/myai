// Package processpolicy defines the environment and process cleanup policy for
// host subprocesses. Windows uses Job Objects; POSIX cleanup reaches only the
// child's original process group and cannot guarantee cleanup after setsid or
// setpgid. This package provides no filesystem, network or general OS isolation.
package processpolicy

import (
	"os"
	"runtime"
	"sort"
	"strings"
)

// Only operating-system, executable lookup, home, temporary-directory, locale
// and selected runtime/configuration directory paths cross the boundary by
// default. Service credentials, proxy URLs, language-runtime injection options
// and arbitrary application settings do not.
var inheritedNames = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "JAVA_HOME",
	"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR",
	"TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "LC_CTYPE", "TZ",
	"SystemRoot", "SystemDrive", "WINDIR", "COMSPEC", "PATHEXT",
	"USERPROFILE", "HOMEDRIVE", "HOMEPATH", "LOCALAPPDATA", "APPDATA",
	"ProgramData", "ProgramFiles", "ProgramFiles(x86)", "CommonProgramFiles",
}

// Environment returns a non-nil, minimal child environment. Only the fixed
// runtime allowlist is inherited; extra is explicitly trusted configuration and
// may add or override any valid name, including credentials needed by that child.
// It never enumerates the parent environment. Callers should not log its result.
func Environment(extra map[string]string) []string {
	return environment(os.LookupEnv, extra, runtime.GOOS == "windows")
}

func environment(lookup func(string) (string, bool), extra map[string]string, windows bool) []string {
	canonical := func(name string) string {
		if windows {
			return strings.ToUpper(name)
		}
		return name
	}
	values := make(map[string]string, len(inheritedNames)+len(extra))
	for _, name := range inheritedNames {
		if value, ok := lookup(name); ok && !strings.ContainsRune(value, 0) {
			values[canonical(name)] = name + "=" + value
		}
	}
	// Sort explicit names so case-insensitive duplicates resolve deterministically.
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := extra[name]
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, 0) {
			continue
		}
		values[canonical(name)] = name + "=" + value
	}
	names = names[:0]
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, values[name])
	}
	return result
}
