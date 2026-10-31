package main

import "testing"

// A Next click has to be recognised however the engine spells it, because that is
// what makes the runner wait for the postback. A bare "next" is what a page with
// nothing left to answer returns - it used to be read as "the page did not move",
// which ended the run as stalled on the very first click.
func TestIsNavAction(t *testing.T) {
	cases := map[string]bool{
		"next":             true,
		"radio+next":       true,
		"drag+next":        true,
		"reentryID+next":   true,
		"carousel+advance": true,
		"drag+hold":        false,
		"hold":             false,
		"":                 false,
		"radio":            false,
	}
	for action, want := range cases {
		if got := isNavAction(action); got != want {
			t.Errorf("isNavAction(%q) = %v, want %v", action, got, want)
		}
	}
}
