//go:build !windows

package device

import "os"

func replaceFileAtomic(source, destination string) error {
	return os.Rename(source, destination)
}
