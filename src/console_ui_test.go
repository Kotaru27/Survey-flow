package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSafeConsoleTextDropsTerminalControls(t *testing.T) {
	original := "This\x1b[2J\x1b[31m bad\nquestion  "
	got := safeConsoleText(original, 100)
	if strings.ContainsAny(got, "\x1b\n\r") || !strings.Contains(got, "question") {
		t.Fatalf("unsafe or missing output: %q", got)
	}
	clipped := safeConsoleText("one two three four", 9)
	if len([]rune(clipped)) > 9 {
		t.Fatalf("did not bound: %q", clipped)
	}
}

func TestPlainRendererNoTerminalSequencesAndSummaryTruthful(t *testing.T) {
	var out bytes.Buffer
	r := newConsoleRenderer(&out, false)
	r.header()
	r.section("LIVE SESSION", "")
	r.onLog("[003] clicked What would you choose?")
	r.onLog("Survey navigation requested: https://study.test/?secret=test-token")
	r.runFinished(1, "needs_review", 0, 2)
	r.finish(BatchReport{Requested: 2, Completed: 0, StartedAt: "now", FinishedAt: "later"})
	s := out.String()
	for _, token := range []string{"SURVEY / AUTOPILOT", "LIVE SESSION", "STEP 003", "needs review", "0 / 2", "Target not reached"} {
		if !strings.Contains(s, token) {
			t.Errorf("missing %q in %q", token, s)
		}
	}
	if strings.Contains(s, "\x1b") || strings.Contains(s, "secret=test-token") || strings.Contains(s, "Requested completion count reached") {
		t.Fatalf("unsafe/incorrect rendering: %q", s)
	}
}

func TestColoredInlineStepAndCriticalWarnings(t *testing.T) {
	var out bytes.Buffer
	r := newConsoleRenderer(&out, true)
	r.onLog("[001] selected Who are you?")
	r.onLog("[002] next What year?")
	r.onLog("Step 2: 3 unanswered item(s); stopping for review")
	s := out.String()
	if !strings.Contains(s, "\r\x1b[2K") || !strings.Contains(s, "STEP 002") || !strings.Contains(s, "REVIEW") {
		t.Fatalf("in-place status and warning missing: %q", s)
	}
	if strings.Count(s, "\n") != 2 {
		t.Fatalf("step rewrites emitted extra lines: %q", s)
	}
}

func TestLogLineAlwaysPreservesFileLog(t *testing.T) {
	var out bytes.Buffer
	r := newConsoleRenderer(&out, false)
	consoleUI = r
	defer func() { consoleUI = nil }()
	f, e := os.CreateTemp(t.TempDir(), "launcher-*.log")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	logLine(f, "Survey navigation requested: %s", "https://example.test/?token=private")
	content, e := os.ReadFile(f.Name())
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(content), "https://example.test/?token=private") {
		t.Fatalf("log lost original: %q", content)
	}
	if strings.Contains(out.String(), "token=private") {
		t.Fatalf("UI leaked signed link: %q", out.String())
	}
}

func TestHeaderFitsStandardEightyColumnConsole(t *testing.T) {
	var out bytes.Buffer
	r := newConsoleRenderer(&out, false)
	r.header()
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if strings.Contains(line, "╭") || strings.Contains(line, "╰") || strings.Contains(line, "│") {
			if len([]rune(line)) != 76 {
				t.Errorf("banner does not align (%d columns): %q", len([]rune(line)), line)
			}
		}
	}
}
