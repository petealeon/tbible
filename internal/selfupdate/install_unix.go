//go:build !windows

package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Install atomically replaces target with the staged binary. On Unix this is
// safe while the old executable is still running.
func Install(staged, target string) error {
	in, err := os.Open(staged)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".tbible-update-*")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	installed := false
	defer func() {
		if !installed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("replace %s: %w", target, err)
	}
	installed = true
	return nil
}
