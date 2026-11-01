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

func TestPlainRendererProgressAndOutcomeSummary(t *testing.T) {
 var out bytes.Buffer
 r := newConsoleRenderer(&out, false)
 r.header()
 r.onLog("[003] clicked Survey progress: 23% Private question")
 r.onLog("[004] clicked Survey progress: 23% Private question")
 r.onLog("Survey navigation requested: https://study.test/?secret=test-token")
 r.onLog("Step 4: red instruction needs review: unmatched")
 r.runFinished(1, "completed", 1, 2)
 r.finish(BatchReport{Requested: 2, Completed: 1, Runs: []BatchRun{{Status:"completed"}, {Status:"screenout_or_termination"}, {Status:"quota"}, {Status:"needs_review"}}})
 s := out.String()
 for _, token := range []string{"SURVEY / AUTOPILOT", "Survey progress: 23%", "1 / 2", "Completes", "Terminates", "Incompletes", "Target not reached"} {
  if !strings.Contains(s, token) { t.Errorf("missing %q in %q", token, s) }
 }
 complete, terminated, incomplete := outcomeCounts(BatchReport{Runs: []BatchRun{{Status:"completed"}, {Status:"screenout_pattern"}, {Status:"quota"}, {Status:"needs_review"}}})
 if complete != 1 || terminated != 1 || incomplete != 2 { t.Fatalf("incorrect outcome counts: %d %d %d", complete, terminated, incomplete) }
 if strings.Count(s, "Survey progress: 23%") != 1 || strings.Contains(s, "Private question") || strings.Contains(s, "needs review") || strings.Contains(s, "\x1b") || strings.Contains(s, "secret=test-token") { t.Fatalf("noisy or unsafe output: %q", s) }
}

func TestColoredInlineStepAndCriticalErrors(t *testing.T) {
 var out bytes.Buffer
 r := newConsoleRenderer(&out, true)
 r.onLog("[001] selected Survey progress: 10% question")
 r.onLog("[002] next Survey progress: 20% question")
 r.onLog("Run 2 failed with validation_failure")
 s := out.String()
 if !strings.Contains(s, "\r\x1b[2K") || !strings.Contains(s, "Survey progress: 20%") || !strings.Contains(s, "ERROR") { t.Fatalf("in-place status and error missing: %q", s) }
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
