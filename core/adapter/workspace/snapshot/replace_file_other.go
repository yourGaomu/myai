//go:build !windows

package snapshot

import "os"

func replaceFile(source string, destination string) error {
	return os.Rename(source, destination)
}
