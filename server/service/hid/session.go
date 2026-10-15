package hid

import (
	"errors"
	"sync"
)

var (
	sessionMu sync.Mutex
	ErrBusy   = errors.New("HID is busy")
)

func TryLockHID() bool {
	return sessionMu.TryLock()
}

func UnlockHID() {
	sessionMu.Unlock()
}
