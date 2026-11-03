//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package ops

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// lockDir takes an advisory lock (flock) on the project directory itself, so no lock file is
// written into the project.
func lockDir(dir string, deadline time.Time) (func(), error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) || time.Now().After(deadline) {
			f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, busy(dir)
			}
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
