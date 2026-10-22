package daemon

import (
	"fmt"
	"os"
	"testing"
)

func testAddr(t *testing.T, n int) string {
	return fmt.Sprintf(`\\.\pipe\bah-test-%d-%d`, os.Getpid(), n)
}
