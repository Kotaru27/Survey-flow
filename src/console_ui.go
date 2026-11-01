package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// consoleUI decorates only the terminal launcher. The file logger and survey
// engine remain unchanged; a redirected stdout receives clean uncolored text.
var consoleUI *consoleRenderer

const (
	ink   = "\x1b[38;2;225;233;238m"
	muted = "\x1b[38;2;127;144;154m"
	mint  = "\x1b[38;2;107;229;192m"
	azure = "\x1b[38;2;135;194;245m"
	amber = "\x1b[38;2;250;200;115m"
	rose  = "\x1b[38;2;255;128;140m"
	bold  = "\x1b[1m"
	reset = "\x1b[0m"
)

var surveyPercent = regexp.MustCompile(`(?i)survey progress:\s*(\d{1,3})%`)

var stepLine = regexp.MustCompile(`^\[(\d+)\]\s+(\S+)\s*(.*)$`)

// safeConsoleText prevents HTML/survey-provided strings from injecting terminal
// escape commands or mangling its layout. Full unmodified text stays in the log.
func safeConsoleText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\u001b' {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if limit < 2 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit-1]) + "…"
}

type consoleRenderer struct {
	mu         sync.Mutex
	out        io.Writer
	colored    bool
	inlineStep bool
	step       int
	target     int
	lastAction string
	lastProgress string
}

func newConsoleRenderer(out io.Writer, colored bool) *consoleRenderer {
	return &consoleRenderer{out: out, colored: colored}
}
func (r *consoleRenderer) stylize(color, s string) string {
	if r.colored {
		return color + s + reset
	}
	return s
}
func (r *consoleRenderer) println(s string) { _, _ = fmt.Fprintln(r.out, s) }
func (r *consoleRenderer) endStep() {
	if r.inlineStep {
		r.println("")
		r.inlineStep = false
	}
}
func (r *consoleRenderer) boxRow(left, right string, mainColor string) {
	gap := 72 - utf8.RuneCountInString(left) - utf8.RuneCountInString(right)
	if gap < 0 {
		gap = 0
	}
	r.println(r.stylize(mint, "  │") + r.stylize(mainColor, left) + strings.Repeat(" ", gap) + r.stylize(muted, right) + r.stylize(mint, "│"))
}
func (r *consoleRenderer) header() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.colored {
		_, _ = fmt.Fprint(r.out, "\x1b[2J\x1b[H")
	}
	r.println(r.stylize(mint, "  ╭"+strings.Repeat("─", 72)+"╮"))
	r.boxRow("", "", ink)
	r.boxRow("    SURVEY / AUTOPILOT", "QA CONSOLE    ", ink+bold)
	r.boxRow("    STUDY-AWARE  •  FOCUSED RUNNER  •  LOCAL REPORTS", "    ", muted)
	r.boxRow("", "", ink)
	r.println(r.stylize(mint, "  ╰"+strings.Repeat("─", 72)+"╯"))
	r.println("")
}
func (r *consoleRenderer) section(title, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endStep()
	r.println("")
	r.println("  " + r.stylize(mint+bold, title) + "  " + r.stylize(muted, strings.Repeat("─", max(0, 67-utf8.RuneCountInString(title)))))
	if detail != "" {
		r.println("  " + r.stylize(muted, safeConsoleText(detail, 75)))
	}
	r.println("")
}
func (r *consoleRenderer) prompt(label, hint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endStep()
	r.println("  " + r.stylize(ink+bold, label))
	if hint != "" {
		r.println("  " + r.stylize(muted, hint))
	}
	_, _ = fmt.Fprint(r.out, "  "+r.stylize(mint+bold, "› "))
}
func (r *consoleRenderer) detail(label, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endStep()
	r.println("  " + r.stylize(muted, fmt.Sprintf("%-14s", label)) + r.stylize(ink, safeConsoleText(value, 61)))
}
func (r *consoleRenderer) event(kind, color, s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventLocked(kind, color, s)
}
func (r *consoleRenderer) eventLocked(kind, color, s string) {
	r.endStep()
	stamp := time.Now().Format("15:04:05")
	r.println("  " + r.stylize(muted, stamp) + "  " + r.stylize(color, fmt.Sprintf("%-9s", kind)) + " " + r.stylize(ink, safeConsoleText(s, 53)))
}
func (r *consoleRenderer) stepStatus(n int, action, question string) {
 r.mu.Lock()
 defer r.mu.Unlock()
 r.step = n
 r.lastAction = action
 progress := "Running survey"
 if m := surveyPercent.FindStringSubmatch(question); len(m) == 2 { progress = "Survey progress: " + m[1] + "%" }
 if !r.colored && progress == r.lastProgress { return }
 r.lastProgress = progress
 line := "  " + r.stylize(azure+bold, "● ") + r.stylize(ink, progress)
 if r.colored { _, _ = fmt.Fprint(r.out, "\r\x1b[2K"+line); r.inlineStep = true } else { r.println(line) }
}
func (r *consoleRenderer) progress(done, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endStep()
	if total < 1 {
		total = 1
	}
	if done > total {
		done = total
	}
	if done < 0 {
		done = 0
	}
	filled := done * 26 / total
	bar := strings.Repeat("━", filled) + strings.Repeat("─", 26-filled)
	r.println("  " + r.stylize(mint, bar) + "  " + r.stylize(ink+bold, fmt.Sprintf("%d / %d", done, total)) + r.stylize(muted, " completed"))
}
func (r *consoleRenderer) runFinished(n int, status string, done, total int) {
 r.mu.Lock()
 r.lastProgress = ""
 r.endStep()
 r.mu.Unlock()
 r.progress(done, total)
}
func statusColor(status string) string {
	switch strings.ToLower(status) {
	case "completed", "success":
		return mint
	case "screenout_pattern", "screenout_or_termination", "quota", "needs_review", "requires_human_verification", "unverified_exit", "unreadable_page":
		return amber
	default:
		return rose
	}
}
// Only explicitly identified screenouts are classified as terminations. Quota,
// errors and unresolved reviews remain incomplete rather than being guessed.
func outcomeCounts(batch BatchReport) (complete, terminated, incomplete int) {
 for _, run := range batch.Runs {
  switch strings.ToLower(strings.TrimSpace(run.Status)) {
  case "completed": complete++
  case "screenout_pattern", "screenout_or_termination": terminated++
  default: incomplete++
  }
 }
 return
}
func (r *consoleRenderer) finish(batch BatchReport) {
 complete, terminated, incomplete := outcomeCounts(batch)
 r.section("RESULTS", "")
 r.progress(batch.Completed, batch.Requested)
 r.detail("Completes", fmt.Sprint(complete))
 r.detail("Terminates", fmt.Sprint(terminated))
 r.detail("Incompletes", fmt.Sprint(incomplete))
 if batch.Completed < batch.Requested { r.event("REVIEW", amber, "Target not reached. See reports for details.") }
}

// onLog hides noisy internals from the user interface, NOT from the file log.
// Not all messages are displayed; serious faults, run boundaries and actionable
// instructions always are. In particular, avoid echoing signed test URLs.
func (r *consoleRenderer) onLog(s string) {
	if m := stepLine.FindStringSubmatch(s); len(m) == 4 {
		var n int
		_, _ = fmt.Sscanf(m[1], "%d", &n)
		act := m[2]
		if strings.Contains(act, "render-waiting") {
			act = "WAITING FOR OPTIONS"
		}
		if strings.Contains(act, "accordion-waiting") {
			act = "OPENING OPTIONS"
		}
		if strings.Contains(act, "next-waiting") {
			act = "OPENING NEXT"
		}
		if strings.Contains(act, "accordion-expand") {
			act = "OPEN CATEGORY"
		}
		if strings.Contains(act, "instruction-verify") {
			act = "VERIFY ANSWER"
		}
		if strings.Contains(act, "instruction-followed") {
			act = "RED INSTRUCTION"
		}
		r.stepStatus(n, act, m[3])
		return
	}
 lower := strings.ToLower(s)
 // Diagnostics remain in launcher.log; show only failures requiring attention.
 switch {
 case strings.Contains(lower, "could not start"), strings.Contains(lower, "connection failed"), strings.Contains(lower, "navigation failed"), strings.Contains(lower, "no tab available"), strings.Contains(lower, "failed with"), strings.Contains(lower, "could not mint"), strings.Contains(lower, "page unreadable"), strings.Contains(lower, "giving up"), strings.Contains(lower, "still rejecting"), strings.Contains(lower, "did not persist"), strings.Contains(lower, "could not save"):
  r.event("ERROR", rose, s)
 }
}
