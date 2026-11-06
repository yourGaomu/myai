package processpolicy

import (
	"strings"
	"testing"
)

func TestEnvironmentOnlyInheritsRuntimeAllowlist(t *testing.T) {
	parent := map[string]string{
		"PATH": "test-bin", "HOME": "test-home", "SystemRoot": "test-windows",
		"ZZ_TEST_SECRET": "dummy-service-secret", "ZZ_TEST_TOKEN": "dummy-service-token",
		"OPENAI_API_KEY": "dummy-model-key", "AWS_ACCESS_KEY_ID": "dummy-storage-key",
		"HTTP_PROXY": "dummy-proxy-credential", "PYTHONPATH": "dummy-injection",
		"NODE_OPTIONS": "dummy-injection", "MYAI_SETTING": "dummy-setting",
		"LD_PRELOAD": "dummy-injection", "LD_LIBRARY_PATH": "dummy-injection", "JAVA_TOOL_OPTIONS": "dummy-injection",
	}
	queried := make(map[string]bool)
	result := environment(func(name string) (string, bool) {
		queried[name] = true
		value, ok := parent[name]
		return value, ok
	}, nil, false)
	for _, entry := range result {
		name, _, _ := strings.Cut(entry, "=")
		if name != "PATH" && name != "HOME" && name != "SystemRoot" {
			t.Fatalf("unexpected inherited variable name: %s", name)
		}
	}
	if len(result) != 3 {
		t.Fatal("runtime allowlist variables were not retained")
	}
	for _, name := range []string{"ZZ_TEST_SECRET", "ZZ_TEST_TOKEN", "OPENAI_API_KEY", "AWS_ACCESS_KEY_ID", "HTTP_PROXY", "PYTHONPATH", "NODE_OPTIONS", "MYAI_SETTING", "LD_PRELOAD", "LD_LIBRARY_PATH", "JAVA_TOOL_OPTIONS"} {
		if queried[name] {
			t.Errorf("non-allowlisted variable was read: %s", name)
		}
	}
}

func TestEnvironmentRetainsExplicitlyAllowedRuntimePaths(t *testing.T) {
	paths := map[string]string{
		"JAVA_HOME": "test-java-home", "XDG_CONFIG_HOME": "test-config-home",
		"XDG_CACHE_HOME": "test-cache-home", "XDG_DATA_HOME": "test-data-home",
		"XDG_STATE_HOME": "test-state-home", "XDG_RUNTIME_DIR": "test-runtime-dir",
	}
	result := environment(func(name string) (string, bool) {
		value, ok := paths[name]
		return value, ok
	}, nil, false)
	if len(result) != len(paths) {
		t.Fatal("runtime/configuration paths were not retained")
	}
	for _, entry := range result {
		name, value, _ := strings.Cut(entry, "=")
		if expected, ok := paths[name]; !ok || value != expected {
			t.Fatal("unexpected runtime path variable")
		}
	}
}

func TestEnvironmentExplicitConfigurationAndWindowsCase(t *testing.T) {
	result := environment(func(name string) (string, bool) {
		return "inherited-test-path", name == "PATH"
	}, map[string]string{
		"Path": "explicit-test-path", "ZZ_TEST_SECRET": "explicit-dummy-secret",
		" EMPTY ": "", "": "invalid", "BAD=NAME": "invalid", "NULL": "invalid\x00value",
	}, true)
	values := make(map[string]string)
	for _, entry := range result {
		name, value, _ := strings.Cut(entry, "=")
		key := strings.ToUpper(name)
		if _, exists := values[key]; exists {
			t.Fatal("duplicate case-insensitive environment name")
		}
		values[key] = value
	}
	if len(values) != 3 || values["PATH"] != "explicit-test-path" || values["ZZ_TEST_SECRET"] != "explicit-dummy-secret" {
		t.Fatal("explicit environment was not applied exactly")
	}
}

func TestEnvironmentEmptyStillClearsInheritance(t *testing.T) {
	result := environment(func(string) (string, bool) { return "", false }, nil, false)
	if result == nil || len(result) != 0 {
		t.Fatal("empty child environment must be non-nil")
	}
}
