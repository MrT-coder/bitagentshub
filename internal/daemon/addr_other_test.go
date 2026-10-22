//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// testAddr keeps the socket path short: macOS refuses socket paths over 104
// bytes, and t.TempDir there is already most of that.
func testAddr(t *testing.T, n int) string {
	dir, err := os.MkdirTemp("", "bah")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, fmt.Sprintf("d%d", n), "daemon.sock")
}
