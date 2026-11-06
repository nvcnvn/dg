package fsys

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// lockTimeout bounds how long a writer waits for another to finish.
var lockTimeout = 10 * time.Second

var (
	writersMu sync.Mutex
	writers   = map[string]chan struct{}{}
)

// Lock makes the caller the only writer of the project, within this process and across processes
// (an advisory lock, see lockDir), until unlock is called. It waits up to lockTimeout for another
// writer to finish.
func (d Disk) Lock() (unlock func(), err error) {
	abs, err := filepath.Abs(string(d))
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return nil, err
	}
	writersMu.Lock()
	sem := writers[abs]
	if sem == nil {
		sem = make(chan struct{}, 1)
		writers[abs] = sem
	}
	writersMu.Unlock()
	deadline := time.Now().Add(lockTimeout)
	select {
	case sem <- struct{}{}:
	case <-time.After(lockTimeout):
		return nil, busy(abs)
	}
	release, err := lockDir(abs, deadline)
	if err != nil {
		<-sem
		return nil, err
	}
	return func() {
		release()
		<-sem
	}, nil
}

func busy(dir string) error {
	return fmt.Errorf("another dg is writing %s; gave up after %v", dir, lockTimeout)
}
