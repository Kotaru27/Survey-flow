//go:build !windows

package main

import "sync"

// Linux QA stand-in only. The Windows release uses its native OS mutex.
var instanceMutex uintptr
var testInstanceMu sync.Mutex
var testInstanceNames = make(map[string]bool)

func acquireSingleInstance(name string) bool {
	testInstanceMu.Lock()
	defer testInstanceMu.Unlock()
	if testInstanceNames[name] {
		return false
	}
	testInstanceNames[name] = true
	instanceMutex = 1
	return true
}
func activateExistingInstance() {}
