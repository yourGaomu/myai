package sandbox

import (
	"fmt"
	"path"
	"strings"
)

type File struct {
	Path    string
	Content []byte
	Mode    uint32
}

func (file File) Validate() error {
	value := strings.TrimSpace(file.Path)
	if value == "" {
		return fmt.Errorf("sandbox file path is required")
	}
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("sandbox file path must be absolute: %s", value)
	}
	if clean := path.Clean(value); clean != value {
		return fmt.Errorf("sandbox file path must be clean: %s", value)
	}
	return nil
}
