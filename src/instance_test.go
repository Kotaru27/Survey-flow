package main

import (
	"fmt"
	"testing"
	"time"
)

// The whole single-instance guarantee rests on the second claim being refused,
// so that is what is proven here: one name can be claimed once, and a different
// name is still free afterwards. The name carries a timestamp because a repeat
// run inside the same process would otherwise meet the handle it won before.
func TestAcquireSingleInstanceRejectsSecondClaim(t *testing.T) {
	name := fmt.Sprintf(`Local\SurveyAutopilot_Test_Instance_%d`, time.Now().UnixNano())
	if !acquireSingleInstance(name) {
		t.Fatal("the first claim on a fresh name must succeed")
	}
	if acquireSingleInstance(name) {
		t.Fatal("a second claim on the same name must be refused")
	}
	if instanceMutex == 0 {
		t.Fatal("the winning claim must keep its handle for the life of the process")
	}
	other := fmt.Sprintf(`Local\SurveyAutopilot_Test_Other_%d`, time.Now().UnixNano())
	if !acquireSingleInstance(other) {
		t.Fatal("a different name must still be claimable")
	}
}
