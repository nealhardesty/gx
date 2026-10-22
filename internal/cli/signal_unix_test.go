//go:build !windows

package cli

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// A terminal Ctrl-C reaches both gx and the child. gx must survive it, wait for
// the child, and report the child's own exit code.
func TestInterruptWaitsForChild(t *testing.T) {
	e := setupEnv(t)
	t.Setenv("PATH", "/bin") // for sleep(1); never holds the claude CLI
	stage(t, e, "trap 'echo caught' INT; sleep 1; echo child-finished; exit 7")

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	r := gx(t, Options{}, "", "-x")
	if r.code != 7 || r.stdout != "child-finished\n" {
		t.Errorf("want the child to finish and exit 7 after gx ignored SIGINT; got %+v", r)
	}
}
