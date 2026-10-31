package main

import (
	"testing"
	"time"
)

func TestVerifiedCompletionOnly(t *testing.T) {
	cases := []struct {
		url           string
		qualify, term bool
	}{
		{"https://survey.example/BCEndPages/en-Qual.html?__etk=1", true, false},
		{"https://survey.example/Qual.html#finished", true, false},
		{"https://survey.example/BCEndPages/en-Term.html", false, true},
		{"https://other.example/thank-you", false, false},
		{"https://survey.example/disQual.html", false, false},
		{"https://survey.example/q?redirect=Qual.html", false, false},
		{"https://survey.example/EndPages/Qual.html/next", false, false},
		{"about:blank", false, false},
	}
	for _, tc := range cases {
		if got := isQualifiedCompletionURL(tc.url); got != tc.qualify {
			t.Errorf("qualification(%q) = %v want %v", tc.url, got, tc.qualify)
		}
		if got := isTerminatedURL(tc.url); got != tc.term {
			t.Errorf("termination(%q) = %v want %v", tc.url, got, tc.term)
		}
	}
}

func TestAccordionWaitingDoesNotBurnAnswerFailures(t *testing.T) {
	sr := StepReport{Action: "accordion-waiting", Accordion: map[string]any{"pending": float64(1)}}
	if !accordionAwaiting(sr, 1, 0) {
		t.Fatal("asynchronously rendered accordion must count as waiting")
	}
	if accordionAwaiting(sr, 0, 0) {
		t.Fatal("no unanswered controls means no waiting")
	}
	if accordionAwaiting(sr, 1, 1) {
		t.Fatal("new answer is progress, not idle waiting")
	}
	if accordionAwaiting(StepReport{}, 1, 0) {
		t.Fatal("ordinary question must still trigger stall guard")
	}
	if maxAccordionWait < 5*time.Second {
		t.Fatal("accordion wait allowance unexpectedly short")
	}
	if maxUnreadableSteps < 8 || maxUnreadableSteps > 30 {
		t.Fatal("unreadable recovery limit invalid")
	}
}

func TestUnverifiedExitsAbortRetryLoop(t *testing.T) {
	for _, status := range []string{"needs_review", "unverified_exit", "unreadable_page"} {
		if !fatalStatus(status) {
			t.Errorf("%s must stop repeated attempts", status)
		}
	}
	if fatalStatus("screenout_or_termination") {
		t.Fatal("expected qualifying retries to remain nonfatal")
	}
	if maxRenderWait < 5*time.Second {
		t.Fatal("render timeout too small")
	}
}
