//go:build unix

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// acquireInstanceLock impede duas instâncias simultâneas do backup, que
// compartilhariam os mesmos arquivos temporários nos servidores remotos.
// O lock é liberado automaticamente pelo kernel se o processo morrer.
func acquireInstanceLock() (func(), error) {
	lockPath := filepath.Join(os.TempDir(), "db-backup-tool.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file %s: %v", lockPath, err)
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another backup instance is already running (lock: %s)", lockPath)
	}

	return func() { f.Close() }, nil
}
