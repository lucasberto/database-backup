//go:build windows

package main

// No Windows a trava de instância única não está implementada; a proteção
// contra execuções simultâneas existe apenas nos builds unix.
func acquireInstanceLock() (func(), error) {
	return func() {}, nil
}
