// Package clipboard provides system clipboard integration.
package clipboard

import "github.com/atotto/clipboard"

// Copy places text on the system clipboard.
func Copy(text string) error {
	return clipboard.WriteAll(text)
}
