// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "sync"

var busyObserver struct {
	sync.RWMutex
	fn func(bool)
}

// SetBusyObserver delivers foreground-work state to an in-process platform
// shell. The callback is invoked after releasing the observer lock.
func SetBusyObserver(fn func(bool)) {
	busyObserver.Lock()
	busyObserver.fn = fn
	busyObserver.Unlock()
}

func notifyBusyObserver(busy bool) {
	busyObserver.RLock()
	fn := busyObserver.fn
	busyObserver.RUnlock()
	if fn != nil {
		fn(busy)
	}
}
