package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Target struct {
	Id                   string `json:"id"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	URL                  string `json:"url"`
	Type                 string `json:"type"`
}
type rpcResp struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type WS struct {
	c  net.Conn
	mu sync.Mutex
}

type Persona struct {
	Name             string `json:"name"`
	Gender           string `json:"gender"`
	Age              int    `json:"age"`
	Country          string `json:"country"`
	City             string `json:"city"`
	YesNo            string `json:"yesNo"`
	Rating           string `json:"rating"`
	MultiSelectCount int    `json:"multiSelectCount"`
	OpenText         string `json:"openText"`
	Number           int    `json:"number"`
	ReentryID        string `json:"reentryID"`
	ReentryTemplate  string `json:"reentryTemplate"`
	ReentryStart     int    `json:"reentryStart"`
	AnswerMode       string `json:"answerMode"`
	VariedPerRun     bool   `json:"variedPerRun"`
}
type AnswerState struct {
	Values         map[string]any `json:"values"`
	SelectedLabels []string       `json:"selectedLabels"`
}

// QuestionState is one question the analyzer found on a step: the controls it owns,
// how many of them are on, and whether the page rendered it as several rows that
// shared one name and one question text.
type QuestionState struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	N    int    `json:"n"`
	On   int    `json:"on"`
	Grid bool   `json:"grid,omitempty"`
}
type StepReport struct {
	Step          int              `json:"step"`
	Time          string           `json:"time"`
	URL           string           `json:"url"`
	Title         string           `json:"title"`
	Question      string           `json:"question"`
	QuestionType  string           `json:"questionType"`
	Action        string           `json:"action"`
	Status        string           `json:"status"`
	Unanswered    int              `json:"unanswered"`
	Validation    []string         `json:"validation"`
	ConsoleErrors []string         `json:"consoleErrors,omitempty"`
	Controls      map[string]int   `json:"controls,omitempty"`
	Iframes       int              `json:"iframes,omitempty"`
	Questions     []QuestionState  `json:"questions,omitempty"`
	OpenQuestions []string         `json:"openQuestions,omitempty"`
	Probe         []map[string]any `json:"probe,omitempty"`
	Screenshot    string           `json:"screenshot,omitempty"`
	// DurationMS is what this step cost end to end. A page that is slow for a reason
	// shows up here rather than in a guess about the engine.
	DurationMS int64 `json:"ms,omitempty"`
	// NeedsHuman is set when the page is asking a person to prove they are one, which
	// no amount of answering will get past.
	NeedsHuman bool `json:"needsHuman,omitempty"`
	// Interaction is the model dispatch.js read the page as - carousel_grid,
	// sequential_open_text, emo_resonance or timeline - with how far through it the
	// step got. It is what shows which adapter owned the page.
	Interaction  map[string]any `json:"interaction,omitempty"`
	Accordion    map[string]any `json:"accordion,omitempty"`
	Instructions map[string]any `json:"instructions,omitempty"`
}
type RunReport struct {
	StartedAt        string       `json:"startedAt"`
	FinishedAt       string       `json:"finishedAt"`
	Run              int          `json:"run,omitempty"`
	URL              string       `json:"url"`
	Status           string       `json:"status"`
	Steps            int          `json:"steps"`
	Persona          Persona      `json:"persona"`
	ReentryID        string       `json:"reentryID"`
	ReturningRetries int          `json:"returningRetries"`
	State            AnswerState  `json:"state"`
	Errors           []string     `json:"errors,omitempty"`
	Learned          []string     `json:"learned,omitempty"`
	AvoidRules       int          `json:"avoidRules,omitempty"`
	Selections       []Selection  `json:"selections"`
	StepsLog         []StepReport `json:"stepsLog"`
}

// Selection is one answer the run actually submitted, in the order it was made. The
// whole list is the run's answer set: kept in memory when the run ends on a screen-out
// or a validation, cleared again when the run is registered as complete.
type Selection struct {
	Step     int    `json:"step"`
	Question string `json:"question"`
	Option   string `json:"option"`
}

type BatchRun struct {
	Run        int      `json:"run"`
	Window     int      `json:"window,omitempty"`
	Status     string   `json:"status"`
	ReentryID  string   `json:"reentryID"`
	Steps      int      `json:"steps"`
	DurationMS int64    `json:"durationMs"`
	Errors     []string `json:"errors,omitempty"`
	Learned    []string `json:"learned,omitempty"`
	Report     string   `json:"report"`
}
type BatchReport struct {
	StartedAt    string         `json:"startedAt"`
	FinishedAt   string         `json:"finishedAt"`
	URL          string         `json:"url"`
	Requested    int            `json:"requested"`
	Completed    int            `json:"completed"`
	Windows      int            `json:"windows"`
	AvoidRules   int            `json:"avoidRules,omitempty"`
	StatusCounts map[string]int `json:"statusCounts"`
	Runs         []BatchRun     `json:"runs"`
}

func wsConnect(raw string) (*WS, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return nil, e
	}
	host := u.Host
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	c, e := net.DialTimeout("tcp", host, 5*time.Second)
	if e != nil {
		return nil, e
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	key := base64.StdEncoding.EncodeToString(b)
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, host, key)
	if _, e = c.Write([]byte(req)); e != nil {
		c.Close()
		return nil, e
	}
	r := bufio.NewReader(c)
	status, e := r.ReadString('\n')
	if e != nil || !strings.Contains(status, "101") {
		c.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	for {
		line, e := r.ReadString('\n')
		if e != nil {
			c.Close()
			return nil, e
		}
		if line == "\r\n" {
			break
		}
	}
	return &WS{c: c}, nil
}
func (w *WS) sendText(s string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := []byte(s)
	h := []byte{0x81}
	n := len(p)
	m := make([]byte, 4)
	_, _ = rand.Read(m)
	switch {
	case n < 126:
		h = append(h, byte(0x80|n))
	case n <= 65535:
		h = append(h, 126|0x80, byte(n>>8), byte(n))
	default:
		h = append(h, 127|0x80)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(n))
		h = append(h, b...)
	}
	h = append(h, m...)
	for i := range p {
		p[i] ^= m[i%4]
	}
	h = append(h, p...)
	_, e := w.c.Write(h)
	return e
}
func (w *WS) recv() ([]byte, error) {
	_ = w.c.SetReadDeadline(time.Now().Add(12 * time.Second))
	h := make([]byte, 2)
	if _, e := io.ReadFull(w.c, h); e != nil {
		return nil, e
	}
	op := h[0] & 15
	if op == 8 {
		return nil, io.EOF
	}
	n := int64(h[1] & 127)
	if n == 126 {
		b := make([]byte, 2)
		if _, e := io.ReadFull(w.c, b); e != nil {
			return nil, e
		}
		n = int64(binary.BigEndian.Uint16(b))
	} else if n == 127 {
		b := make([]byte, 8)
		if _, e := io.ReadFull(w.c, b); e != nil {
			return nil, e
		}
		n = int64(binary.BigEndian.Uint64(b))
	}
	masked := h[1]&128 != 0
	var m [4]byte
	if masked {
		if _, e := io.ReadFull(w.c, m[:]); e != nil {
			return nil, e
		}
	}
	if n > 32*1024*1024 {
		return nil, errors.New("websocket frame too large")
	}
	p := make([]byte, n)
	if _, e := io.ReadFull(w.c, p); e != nil {
		return nil, e
	}
	if masked {
		for i := range p {
			p[i] ^= m[i%4]
		}
	}
	return p, nil
}
func (w *WS) call(id int, method string, params any) (json.RawMessage, error) {
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	b, _ := json.Marshal(msg)
	if e := w.sendText(string(b)); e != nil {
		return nil, e
	}
	for {
		p, e := w.recv()
		if e != nil {
			return nil, e
		}
		var r rpcResp
		if json.Unmarshal(p, &r) != nil || r.ID != id {
			continue
		}
		if len(r.Error) > 0 && string(r.Error) != "null" {
			return nil, fmt.Errorf("CDP %s: %s", method, string(r.Error))
		}
		return r.Result, nil
	}
}
func jsonTargets(port int) ([]Target, error) {
	r, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	var t []Target
	e = json.NewDecoder(r.Body).Decode(&t)
	return t, e
}

// jsonVersion returns the browser-level CDP websocket URL (used to open and close tabs).
func jsonVersion(port int) (string, error) {
	r, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if e := json.NewDecoder(r.Body).Decode(&v); e != nil {
		return "", e
	}
	if v.WebSocketDebuggerURL == "" {
		return "", errors.New("no browser websocket url")
	}
	return v.WebSocketDebuggerURL, nil
}

// blankTargetURL reports Chrome's own pages and data: documents (the app's
// own UI is a data:text/html page), which are never the survey tab.
func blankTargetURL(u string) bool {
	return u == "" || strings.HasPrefix(u, "about:") || strings.HasPrefix(u, "chrome://") ||
		strings.HasPrefix(u, "data:")
}

// BrowserSession is one Chrome window. A batch opens a window, runs several completes in
// it (one tab each, one after another), then closes it Ã¢â‚¬â€ so the laptop pays the process
// start cost once per window instead of once per complete.
type BrowserSession struct {
	cmd      *exec.Cmd
	port     int
	profile  string
	done     chan struct{}
	waitErr  error
	bws      *WS     // browser-level CDP socket for Target.createTarget / closeTarget
	nextID   int     // id counter for browser-level calls
	ctxID    string  // browser context our tabs live in (off-the-record for incognito)
	current  *Target // tab the active complete is driving
	ws       *WS     // that tab's websocket
	embedded bool    // session is the app's own in-window browser, not a spawned Chrome
}

func (b *BrowserSession) browserCall(method string, params any) (json.RawMessage, error) {
	if b.bws == nil {
		return nil, errors.New("no browser-level CDP socket")
	}
	b.nextID += 100
	return b.bws.call(b.nextID, method, params)
}

// embeddedCDPPort reports the debug port of this app's own in-window browser,
// or 0 when the batch must use a separate Chrome. The port only exists when
// the GUI published it before creating its webviews (external mode never
// does), and the console launcher never creates a WebView2 at all.
func embeddedCDPPort(cfg AppConfig) int {
	if cfg.BrowserMode == "external" {
		return 0
	}
	p, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SAP_EMBEDDED_CDP")))
	if err != nil || p < 1 || p > 65535 {
		return 0
	}
	return p
}

// attachEmbedded connects to the pane the GUI already opened: it waits for the
// debug endpoint to come up and picks the target carrying the pane's wait-page
// fingerprint. Matching the fingerprint (not "first non-blank page") is what
// keeps the app's own UI page out of the equation: it is a data:text/html
// document too, and navigating it to the survey would wipe the window's
// interface. An empty status means success; anything else sends the caller
// back to the external-Chrome path.
func attachEmbedded(port int, logFile *os.File) (*BrowserSession, string, error) {
	sess := &BrowserSession{port: port, done: make(chan struct{}), embedded: true}
	var pane Target
	for i := 0; i < 40; i++ {
		if sess.bws == nil {
			if v, e := jsonVersion(port); e == nil {
				if w, ce := wsConnect(v); ce == nil {
					sess.bws = w
				}
			}
		}
		if ts, e := jsonTargets(port); e == nil {
			for _, t := range ts {
				if t.Type == "page" && t.WebSocketDebuggerURL != "" && strings.Contains(t.URL, "SAP-PANE-WAIT") {
					pane = t
					break
				}
			}
		}
		if pane.WebSocketDebuggerURL != "" && sess.bws != nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if pane.WebSocketDebuggerURL == "" || sess.bws == nil {
		sess.detach()
		return nil, "cdp_error", errors.New("in-window browser never exposed its fingerprinted pane over CDP")
	}
	sess.current = &pane
	logLine(logFile, "Using the in-window browser pane. Port=%d Tab=%s", port, shortID(pane.Id))
	return sess, "", nil
}

// launchBrowser starts one window (incognito by default) and returns it with an empty
// status, or a failure status plus the error.
// windowed reports whether this process is the desktop app rather than the console
// launcher. Only the app has a pane; only the console may fall back to Chrome.
var windowed = func() bool { return false }

// embeddedPaneReset puts the app's browser pane back on its waiting page. The batch calls
// it before it lets go of the pane, so the next window finds the pane again instead of
// failing on a browser that is still sitting on the previous survey's last page.
var embeddedPaneReset = func() {}

func launchBrowser(root string, logFile *os.File, cfg AppConfig) (*BrowserSession, string, error) {
	// In-window mode first: the GUI already runs a WebView2 with CDP on the
	// published port, so attach to its pane instead of spawning Chrome. Any
	// problem here falls back to the normal external window, because a batch
	// must never die at launch.
	if port := embeddedCDPPort(cfg); port > 0 {
		if sess, st, e := attachEmbedded(port, logFile); st == "" {
			return sess, "", nil
		} else {
			logLine(logFile, "In-window browser unavailable (%s): %v; falling back to a separate browser window.", st, e)
		}
	}
	// A windowed run must never put a browser window on the desktop. The survey
	// belongs inside this app: a second browser popping up beside it is the one
	// thing that makes a run look like it is being driven from somewhere else, and
	// on a study that links the test ID to the person it is worse than that. If
	// the pane cannot be attached, the run says so and stops rather than opening
	// Chrome; the console launcher, which has no window to drive, is the only path
	// that still uses an external browser.
	if windowed() {
		return nil, "browser_missing", errors.New("the in-window browser could not be attached, and a windowed run does not open an external browser")
	}
	browser := findBrowser(root)
	if browser == "" {
		return nil, "browser_missing", errors.New("no browser found; put Chrome for Testing at .\\browser\\chrome.exe")
	}
	port, listener, err := freePort()
	if err != nil {
		return nil, "port_error", err
	}
	_ = listener.Close()
	profile := filepath.Join(root, "data", "browser-profile-"+fmt.Sprint(time.Now().UnixNano()))
	_ = os.MkdirAll(profile, 0755)
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins=*",
		"--user-data-dir=" + profile,
		// The survey tab waits on short setTimeout chains; a window that Chrome
		// considers hidden or occluded gets those timers stretched to seconds or
		// minutes, the engine's await then never settles within the CDP deadline
		// and the run dies with an i/o timeout. Automation browsers must never be
		// throttled, so every backgrounding path is switched off here.
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--no-first-run", "--no-default-browser-check",
		"--disable-features=Translate,CalculateNativeWinOcclusion",
		// Lets the launcher open the next tab with window.open when Chrome's CDP refuses to
		// (incognito windows are not addressable by Target.createTarget).
		"--disable-popup-blocking",
	}
	if cfg.Incognito {
		args = append(args, "--incognito")
	}
	// Used only by the cloud QA runner; normal console launches remain visible.
	if os.Getenv("SURVEY_QA_HEADLESS") == "1" { args = append(args, "--headless=new", "--disable-gpu") }
	args = append(args, "about:blank")
	cmd := exec.Command(browser, args...)
	sess := &BrowserSession{cmd: cmd, port: port, profile: profile, done: make(chan struct{})}
	if e := cmd.Start(); e != nil {
		return nil, "browser_error", e
	}
	go func() {
		sess.waitErr = cmd.Wait()
		close(sess.done)
	}()
	var launch Target
	for i := 0; i < 60; i++ {
		select {
		case <-sess.done:
			return sess, "browser_exited", errors.New("browser exited during startup")
		default:
		}
		if ts, e := jsonTargets(port); e == nil {
			for _, t := range ts {
				if t.Type == "page" && t.WebSocketDebuggerURL != "" {
					launch = t
					break
				}
			}
		}
		if launch.WebSocketDebuggerURL != "" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if launch.WebSocketDebuggerURL == "" {
		return sess, "cdp_error", errors.New("CDP target unavailable after 15 seconds")
	}
	sess.current = &launch
	if v, e := jsonVersion(port); e == nil {
		if w, ce := wsConnect(v); ce == nil {
			sess.bws = w
		} else {
			logLine(logFile, "Browser-level CDP socket unavailable: %v", ce)
		}
	} else {
		logLine(logFile, "Browser version endpoint unavailable: %v", e)
	}
	mode := ""
	if cfg.Incognito {
		mode = " (incognito)"
	}
	logLine(logFile, "Window opened%s. PID=%d Port=%d Profile=%s", mode, cmd.Process.Pid, port, profile)
	return sess, "", nil
}

// createTab opens a new tab in the window and waits until its websocket is registered.
// Incognito windows live in an off-the-record browser context, so if the plain call is
// refused we retry with each known browser context id until one of them is ours.
func (b *BrowserSession) createTab(logFile *os.File) (*Target, error) {
	// The pane is the window's only tab: each complete reuses it, so "a fresh
	// tab" means re-reading the same target (its id survives navigations).
	// Creating or closing targets is what would kill the survey surface.
	if b.embedded {
		if b.current == nil {
			return nil, errors.New("no in-window pane attached")
		}
		ts, err := jsonTargets(b.port)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			if t.Id == b.current.Id && t.WebSocketDebuggerURL != "" {
				tab := t
				logLine(logFile, "Reusing the in-window pane for the next complete.")
				return &tab, nil
			}
		}
		return nil, errors.New("in-window pane target vanished")
	}
	// Incognito windows sit in an off-the-record browser context that is not advertised by
	// Target.getBrowserContexts, so ask the live tab which context it belongs to.
	if b.ctxID == "" && b.current != nil {
		b.ctxID = b.contextOfTarget(b.current.Id, logFile)
	}
	contexts := []string{""}
	if b.ctxID != "" {
		contexts = []string{b.ctxID, ""}
	} else {
		contexts = append(contexts, b.browserContexts()...)
	}
	logLine(logFile, "Opening a fresh tab (browserContextId=%q, candidates=%d).", b.ctxID, len(contexts))
	var lastErr error
	for _, cid := range contexts {
		params := map[string]any{"url": "about:blank", "background": false, "newWindow": false}
		if cid != "" {
			params["browserContextId"] = cid
		}
		// Browser-level socket first, then the live tab's socket.
		for _, sock := range b.sockets() {
			tab, err := b.createTargetOn(sock, params)
			if err == nil {
				b.ctxID = cid
				return tab, nil
			}
			lastErr = err
		}
	}
	// Chrome refuses Target.createTarget inside an incognito window (it only resolves tabs
	// for the regular profile). Open the tab from the live page instead â€” the launcher runs
	// with --disable-popup-blocking, so window.open is not suppressed.
	if tab, err := b.openTabViaScript(logFile); err == nil {
		return tab, nil
	} else if lastErr == nil {
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no browser context available")
	}
	return nil, lastErr
}

// openTabViaScript asks the current page to open a new tab and returns it.
func (b *BrowserSession) openTabViaScript(logFile *os.File) (*Target, error) {
	if b.ws == nil {
		return nil, errors.New("no live page session for a scripted tab open")
	}
	seen := map[string]bool{}
	if ts, err := jsonTargets(b.port); err == nil {
		for _, t := range ts {
			seen[t.Id] = true
		}
	}
	b.nextID++
	if _, err := js(b.ws, b.nextID, "window.open('about:blank','_blank'); 'opened'"); err != nil {
		return nil, fmt.Errorf("scripted tab open failed: %w", err)
	}
	for i := 0; i < 40; i++ {
		if ts, err := jsonTargets(b.port); err == nil {
			for _, t := range ts {
				if t.Type == "page" && t.WebSocketDebuggerURL != "" && !seen[t.Id] {
					tab := t
					logLine(logFile, "Tab opened from script (CDP createTarget is unavailable in incognito).")
					return &tab, nil
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil, errors.New("scripted tab open produced no new tab")
}

// sockets returns every CDP socket a Target command can be sent on, in preference order.
func (b *BrowserSession) sockets() []*WS {
	var out []*WS
	if b.bws != nil {
		out = append(out, b.bws)
	}
	if b.ws != nil {
		out = append(out, b.ws)
	}
	return out
}

// contextOfTarget returns the browser context id that owns the given target.
func (b *BrowserSession) contextOfTarget(id string, logFile *os.File) string {
	if id == "" {
		return ""
	}
	r, err := b.browserCall("Target.getTargetInfo", map[string]any{"targetId": id})
	if err != nil {
		logLine(logFile, "Target.getTargetInfo failed: %v", err)
		return ""
	}
	var res struct {
		TargetInfo struct {
			BrowserContextID string `json:"browserContextId"`
		} `json:"targetInfo"`
	}
	if json.Unmarshal(r, &res) != nil {
		return ""
	}
	return res.TargetInfo.BrowserContextID
}

func (b *BrowserSession) browserContexts() []string {
	r, err := b.browserCall("Target.getBrowserContexts", nil)
	if err != nil {
		return nil
	}
	var res struct {
		BrowserContextIDs []string `json:"browserContextIds"`
	}
	if json.Unmarshal(r, &res) != nil {
		return nil
	}
	return res.BrowserContextIDs
}

func (b *BrowserSession) createTargetOn(w *WS, params map[string]any) (*Target, error) {
	if w == nil {
		return nil, errors.New("no CDP socket")
	}
	b.nextID++
	r, e := w.call(b.nextID, "Target.createTarget", params)
	if e != nil {
		return nil, e
	}
	var res struct {
		TargetID string `json:"targetId"`
	}
	if json.Unmarshal(r, &res) != nil || res.TargetID == "" {
		return nil, errors.New("Target.createTarget returned no target id")
	}
	for i := 0; i < 40; i++ {
		if ts, te := jsonTargets(b.port); te == nil {
			for _, t := range ts {
				if t.Id == res.TargetID && t.WebSocketDebuggerURL != "" {
					tab := t
					return &tab, nil
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil, errors.New("new tab never became available")
}

func (b *BrowserSession) closeTab(id string) {
	if id == "" || b.embedded {
		return // closing the pane's only target would close the browser surface
	}
	_, _ = b.browserCall("Target.closeTarget", map[string]any{"targetId": id})
}

// detach closes our CDP sockets but leaves the window running (single-run mode, where the
// browser is intentionally kept open for inspection).
func (b *BrowserSession) detach() {
	if b == nil {
		return
	}
	if b.ws != nil {
		_ = b.ws.c.Close()
		b.ws = nil
	}
	if b.bws != nil {
		_ = b.bws.c.Close()
		b.bws = nil
	}
}

// stop closes the window (killing the process tree) and, for batch runs, its profile.
func (b *BrowserSession) stop(logFile *os.File, purgeProfile bool) {
	if b == nil {
		return
	}
	b.detach()
	if b.embedded {
		// The browser belongs to the app window, not to this batch: there is
		// no process to kill and no profile to purge, only the CDP sockets to
		// drop. The pane stays alive for the next run.
		logLine(logFile, "In-window browser kept open; CDP detached.")
		return
	}
	select {
	case <-b.done:
	default:
		if b.cmd != nil {
			killTree(b.cmd)
		}
	}
	select {
	case <-b.done:
	case <-time.After(5 * time.Second):
	}
	if purgeProfile {
		removeProfile(b.profile)
	}
	_ = b.waitErr
}
func js(w *WS, id int, expr string) (string, error) {
	r, e := w.call(id, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true})
	if e != nil {
		return "", e
	}
	var x struct {
		Result struct {
			Value       any    `json:"value"`
			Description string `json:"description"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text       string `json:"text"`
			LineNumber int    `json:"lineNumber"`
			Exception  *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if json.Unmarshal(r, &x) != nil {
		return string(r), nil
	}
	// A thrown script comes back as a result with nothing in it, which reads exactly like
	// an empty page. The throw is the only description of what went wrong, so it is passed
	// on instead of being dropped.
	if x.ExceptionDetails != nil {
		detail := x.ExceptionDetails.Text
		if x.ExceptionDetails.Exception != nil && x.ExceptionDetails.Exception.Description != "" {
			detail = x.ExceptionDetails.Exception.Description
		}
		return "", fmt.Errorf("script error at line %d: %s", x.ExceptionDetails.LineNumber, detail)
	}
	if x.Result.Value == nil {
		return x.Result.Description, nil
	}
	b, _ := json.Marshal(x.Result.Value)
	return string(b), nil
}

// contentAnswer marks the answers a panelist is judged on rather than screened out on:
// the brands, the grid cells, the pictures and the sliders. Those are re-picked on every
// attempt, because replaying them unchanged is what makes ten completes of the same
// survey ten identical respondents instead of ten different ones. Everything else - the
// profile the screener qualifies - is replayed exactly.
func contentAnswer(question string) bool {
	q := strings.ToLower(question)
	if strings.Contains(q, " #") {
		return true
	}
	for _, w := range []string{"emo resonance", "brand", "logo", "image", "picture", "photo", "resonan", "grid", "matrix", "slider", "mosaic", "carousel"} {
		if strings.Contains(q, w) {
			return true
		}
	}
	return false
}

// waitDocSettled blocks until the inspected document has loaded and its URL stopped changing
// (a test link usually redirects once), so the engine never runs mid-navigation and a retried
// engine never clicks Next on a page that is still loading. It polls quickly and returns as
// soon as the document has been complete and unchanged twice in a row. The wait is short on
// purpose: a page that has not settled in a couple of seconds is read anyway by the engine's
// own render check, and sitting here is what a run's time goes.
func waitDocSettled(w *WS, id int) {
	// A short courtesy, not a nap: the engine has its own render check, and a page
	// that has not settled in this long is read anyway rather than waited on.
	deadline := time.Now().Add(200 * time.Millisecond)
	last, stable := "", 0
	for time.Now().Before(deadline) {
		if v, e := js(w, id, "({c:document.readyState==='complete'?1:0,u:String(location.href)})"); e == nil {
			var st struct {
				C int    `json:"c"`
				U string `json:"u"`
			}
			if json.Unmarshal([]byte(v), &st) == nil && st.C == 1 && st.U != "" && st.U != "about:blank" {
				if st.U == last {
					stable++
					if stable >= 2 {
						return
					}
				} else {
					last, stable = st.U, 0
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// freshSession drops the survey's own state - cookies, local and session storage - before
// a new attempt is started. Restarting without this resumes the session that was just
// terminated: the survey puts the attempt back on the very question it refused, every
// time, and a session that is resumed over and over is how a test link expires.
func freshSession(w *WS, u string, id int) {
	isolateRespondent(w, id, u)
}

// browserDeadPage is a page the browser itself put up - a failed navigation, a blank
// tab. It is not the survey and it is not an end page either: nothing was answered and
// nothing ended, so the run leaves it rather than calling it a complete.
func browserDeadPage(raw string) bool {
	l := strings.ToLower(strings.TrimSpace(raw))
	if l == "" {
		// An empty URL is a page mid-postback, not a dead browser: reading it again is
		// the whole answer. Treating it as a dead page restarted the attempt every few
		// steps and spent the page budget on a page that was loading.
		return false
	}
	for _, p := range []string{"chrome-error:", "edge://", "devtools:"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// leftSurvey reports whether the browser has left the survey itself: a hosted end page,
// a thank-you on another domain, anything that is not the questionnaire. The interview
// is over at that point, and a step taken there finds nothing to answer - which is how a
// run ends up sitting on a page that is not even the survey.
func leftSurvey(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	su, serr := url.Parse(surveyURL)
	if serr != nil || su.Host == "" {
		return false
	}
	if !strings.EqualFold(u.Host, su.Host) {
		return true
	}
	return regexp.MustCompile(`(?i)EndPages?|Term\.html|/BCEndPage|thank.?you`).MatchString(u.Path)
}

// isQualifiedCompletionURL is the only accepted completion proof. An external
// landing page or a generic thank-you is not a qualified submission.
var qualifiedPath = regexp.MustCompile(`(?i)(?:^|[/-])Qual\.html$`)
var terminatedPath = regexp.MustCompile(`(?i)(?:^|[/-])Term\.html$`)

func isQualifiedCompletionURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		u.Host != "" && qualifiedPath.MatchString(u.Path)
}

func isTerminatedURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && terminatedPath.MatchString(u.Path)
}

// A page without a readable URL or signature should not silently consume the
// entire survey step budget. Keep retries generous, then report the exact cause.
const maxUnreadableSteps = 12
const maxAccordionWait = 8 * time.Second
const maxRenderWait = 8 * time.Second

// Accordion rendering is asynchronous. No new answer is possible while its
// controls are mounting; this is waiting, not three failed answer attempts.
func accordionAwaiting(sr StepReport, unanswered, newAnswers int) bool {
	return unanswered > 0 && newAnswers == 0 && sr.Accordion != nil && toInt(sr.Accordion["pending"]) > 0
}

// surveyURL is the link the current run is answering. leftSurvey reads it to tell an
// end page on another host from a page of the questionnaire.
var surveyURL string

// isNavAction reports whether the step moved the survey forward - a Next click or a
// carousel's own advance - so the runner knows to let the document settle instead of
// judging the page before its render has happened. The action is a "+"-joined list of
// what the engine did, so each part is matched on its own: the bare "next" a page with
// nothing left to answer returns is a navigation too.
func isNavAction(action string) bool {
	for _, p := range strings.Split(action, "+") {
		if p == "next" || p == "advance" {
			return true
		}
	}
	return false
}
func findBrowser(root string) string {
	for _, p := range []string{filepath.Join(root, "browser", "chrome.exe"), filepath.Join(root, "browser", "chromium.exe"), os.Getenv("LOCALAPPDATA") + `\\Google\\Chrome\\Application\\chrome.exe`, os.Getenv("LOCALAPPDATA") + `\\Microsoft\\Edge\\Application\\msedge.exe`, os.Getenv("ProgramFiles") + `\\Google\\Chrome\\Application\\chrome.exe`, os.Getenv("ProgramFiles") + `\\Microsoft\\Edge\\Application\\msedge.exe`, os.Getenv("ProgramFiles(x86)") + `\\Google\\Chrome\\Application\\chrome.exe`, os.Getenv("ProgramFiles(x86)") + `\\Microsoft\\Edge\\Application\\msedge.exe`} {
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	return ""
}

// An optional study key can be appended to the Windows executable. The fixed
// footer makes it possible to distribute one file without publishing the key.
func readEmbeddedRedHerring(path string) []byte {
	f, err := os.Open(path)
	if err != nil { return nil }
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() < 16 { return nil }
	footer := make([]byte, 16)
	if _, err := f.ReadAt(footer, st.Size()-16); err != nil || string(footer[:8]) != "SFAQKEY1" { return nil }
	n := binary.LittleEndian.Uint64(footer[8:])
	if n == 0 || n > 4*1024*1024 || n > uint64(st.Size()-16) { return nil }
	b := make([]byte, int(n))
	if _, err := f.ReadAt(b, st.Size()-16-int64(n)); err != nil || !json.Valid(stripBOM(b)) { return nil }
	return b
}

// The embedded key and optional external keys remain isolated by study.
func readRedHerring(root, surveyLink string) []map[string]any {
	var global []map[string]any
	if path, err := os.Executable(); err == nil {
		b := readEmbeddedRedHerring(path)
		global = parseEmbeddedGlobalChecks(b)
		if result := parseRedHerring(b, surveyLink); len(result) > 0 { return append(result, global...) }
	}
	b, _ := os.ReadFile(filepath.Join(root, "config", "red-herring.json"))
	return append(parseRedHerring(b, surveyLink), global...)
}

// Only the executable's own narrowly described multi-choice defaults are
// allowed across studies. External configuration remains study-scoped.
func parseEmbeddedGlobalChecks(b []byte) []map[string]any {
	var doc struct { GlobalRules []map[string]any `json:"globalRules"` }
	if json.Unmarshal(stripBOM(b), &doc)!=nil { return nil }
	var out []map[string]any
	for _,r:=range doc.GlobalRules {
		prompt,_:=r["contains"].(string);safe,sok:=r["answers"].([]any);bad,bok:=r["never"].([]any)
		if r["kind"]=="multi_exact" && r["strict"]==true && len(strings.TrimSpace(prompt))>=40 && sok && bok && len(safe)>0 && len(bad)>0 { out=append(out,r) }
	}
	return out
}

func parseRedHerring(b []byte, surveyLink string) []map[string]any {
	var doc struct {
		StudyKey   string           `json:"studyKey"`
		StudyURL   string           `json:"studyUrl"`
		Rules      []map[string]any `json:"rules"`
		IDPrefixes []struct {
			Prefix string `json:"prefix"`
			Note   string `json:"note"`
		} `json:"idPrefixes"`
	}
	if json.Unmarshal(stripBOM(b), &doc) != nil {
		return nil
	}
	// A rule written for a previous study must never be applied to another.
	// Legacy unscoped files are disabled until the owner explicitly binds them.
	key := memoryFingerprint(surveyLink)
	scoped := strings.TrimSpace(doc.StudyKey)
	if scoped == "" && strings.TrimSpace(doc.StudyURL) != "" {
		scoped = memoryFingerprint(doc.StudyURL)
	} else if scoped != "" && !strings.HasPrefix(scoped, "sha256:") {
		scoped = memoryFingerprint(scoped)
	}
	if scoped == "" || !strings.EqualFold(scoped, key) {
		return nil
	}
	out := make([]map[string]any, 0, len(doc.Rules)+len(doc.IDPrefixes))
	for _, r := range doc.Rules {
		if len(r) > 0 {
			out = append(out, r)
		}
	}
	// The study numbers some of its checks with an id prefix rather than by their words;
	// they are carried through as prefix rules so an unruled one is reported by id.
	for _, px := range doc.IDPrefixes {
		if px.Prefix != "" {
			out = append(out, map[string]any{"idStarts": px.Prefix, "note": px.Note})
		}
	}
	return out
}

func readPersona(root string) Persona {
	p := Persona{Name: "Normal Test Respondent", Gender: "Male", Age: 30, Country: "United Kingdom", City: "London", YesNo: "Yes", Rating: "middle", MultiSelectCount: 4, OpenText: "This is a test response", Number: 30, ReentryID: "", ReentryTemplate: "MT{uid}", ReentryStart: 1, AnswerMode: "random"}
	b, e := os.ReadFile(filepath.Join(root, "config", "persona.json"))
	if e == nil {
		_ = json.Unmarshal(stripBOM(b), &p)
	}
	// Persona mode answers consistently inside one run; variedPerRun rolls that
	// profile again for every run so no two runs submit the same set of answers.
	if p.VariedPerRun && strings.Contains(strings.ToLower(p.AnswerMode), "persona") {
		varyPersona(&p)
	}
	// A free-text answer must never end in a full stop: the trailing period reads as
	// punctuation the respondent never wrote and makes every open answer look
	// machine-made.
	p.OpenText = strings.TrimRight(strings.TrimSpace(p.OpenText), ".")
	if p.OpenText == "" {
		p.OpenText = "This is a test response"
	}
	return p
}

// pickOne returns one entry of opts from the process random source.
func pickOne(opts ...string) string {
	if len(opts) == 0 {
		return ""
	}
	return opts[pickN(len(opts))]
}

// pickN returns a value in [0,n).
func pickN(n int) int {
	if n <= 0 {
		return 0
	}
	var b [4]byte
	if _, e := rand.Read(b[:]); e != nil {
		return 0
	}
	v := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if v < 0 {
		v = -v
	}
	return v % n
}

// varyPersona rolls a fresh, internally consistent respondent for one run: every
// choice question of that run is answered from the same person, while the next run
// rolls a different one. Fields that identify the run - the test ID template, free
// text, country and name - stay exactly as configured in config/persona.json, and
// per-survey memory still overrides anything a survey has already rejected.
func varyPersona(p *Persona) {
	if p.Gender != "" {
		p.Gender = pickOne("Male", "Female", "Other")
	}
	if p.Age > 0 {
		p.Age = 18 + pickN(43)
	}
	if p.City != "" {
		p.City = pickOne("London", "Manchester", "Birmingham", "Leeds", "Bristol", "Sheffield", "Liverpool", "Newcastle", "Cambridge", "Norwich", "Oxford", "York", "Brighton", "Cardiff", "Edinburgh", "Glasgow")
	}
	if p.YesNo != "" {
		p.YesNo = pickOne("Yes", "No")
	}
	if p.MultiSelectCount > 0 {
		// A respondent who buys sports nutrition buys several kinds of it. Answering a
		// "select all that apply" list with one or two boxes is not a person, and on the
		// studies that cross-check the list against the answers given later it is read
		// as an inconsistent profile.
		p.MultiSelectCount = 2 + pickN(5)
	}
	p.Number = p.Age
}

// renderReentryTemplate expands a test ID template.
// Supported tokens: "#" or "{seq}" = sequence number, "{ts}" = unix milliseconds,
// "{rand}" = 4 char random token, "{uid}" = 10 char random unique token.
func renderReentryTemplate(template string, counter int) string {
	s := strings.ReplaceAll(template, "#", strconv.Itoa(counter))
	s = strings.ReplaceAll(s, "{seq}", strconv.Itoa(counter))
	s = strings.ReplaceAll(s, "{ts}", strconv.FormatInt(time.Now().UnixMilli(), 10))
	if strings.Contains(s, "{rand}") {
		s = strings.ReplaceAll(s, "{rand}", randomToken(4))
	}
	if strings.Contains(s, "{uid}") {
		s = strings.ReplaceAll(s, "{uid}", randomToken(10))
	}
	if !strings.Contains(template, "#") && !strings.Contains(template, "{seq}") &&
		!strings.Contains(template, "{ts}") && !strings.Contains(template, "{rand}") &&
		!strings.Contains(template, "{uid}") {
		s = fmt.Sprintf("%s%d", template, counter)
	}
	return s
}

func randomToken(n int) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return strings.Repeat("x", n)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// nextReentryID mints a test ID that has never been issued before.
// Every issued ID is appended to data/reentry-history.txt so a re-used ID can never be handed out twice.
func nextReentryID(root string, persona Persona) (string, int, error) {
	template := strings.TrimSpace(persona.ReentryTemplate)
	if template == "" {
		template = "MT{uid}"
	}
	counterPath := filepath.Join(root, "data", "reentry-counter.txt")
	historyPath := filepath.Join(root, "data", "reentry-history.txt")
	counter := persona.ReentryStart
	if counter <= 0 {
		counter = 1
	}
	if b, err := os.ReadFile(counterPath); err == nil {
		var stored int
		if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &stored); scanErr == nil && stored >= counter {
			counter = stored
		}
	}
	used := map[string]bool{}
	b, readErr := os.ReadFile(historyPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		// Fail loudly rather than risk re-issuing an ID we cannot see.
		return "", 0, fmt.Errorf("cannot read test ID history %s: %w", historyPath, readErr)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			used[t] = true
		}
	}
	// An explicitly pinned ID from persona.json is honoured as-is.
	if fixed := strings.TrimSpace(persona.ReentryID); fixed != "" && !used[fixed] {
		if err := appendHistory(historyPath, fixed); err != nil {
			return "", 0, err
		}
		return fixed, counter, nil
	}
	id := renderReentryTemplate(template, counter)
	for i := 0; i < 100000 && used[id]; i++ {
		counter++
		id = renderReentryTemplate(template, counter)
	}
	if used[id] {
		return "", 0, errors.New("could not mint a unique test ID")
	}
	if strings.Contains(template, "#") || strings.Contains(template, "{seq}") {
		if err := os.WriteFile(counterPath, []byte(strconv.Itoa(counter+1)), 0644); err != nil {
			return "", 0, err
		}
	}
	if err := appendHistory(historyPath, id); err != nil {
		return "", 0, err
	}
	return id, counter, nil
}

// appendHistory records an issued test ID. Append-only so an existing history can never be truncated.
func appendHistory(path, id string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(id + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// maxReturningRetries is how many fresh unique test IDs are minted before a run gives up on a
// survey that insists the respondent already has an active/completed session.
const maxReturningRetries = 3

// maxCompletes caps one batch so a typo cannot launch hundreds of browsers.
const maxCompletes = 50

// AppConfig controls evidence capture and batch scheduling. Screenshots are the only
// per-step artefact that can grow without bound (100 completes x a 30 minute LOI x one PNG
// per step fills a disk), and one Chrome process per complete keeps the laptop pinned, so
// both are budgeted here.
type AppConfig struct {
	Screenshots             string `json:"screenshots"`             // all | key | none
	ScreenshotFormat        string `json:"screenshotFormat"`        // png | jpg
	ScreenshotQuality       int    `json:"screenshotQuality"`       // jpeg quality 1-100
	ScreenshotRetentionDays int    `json:"screenshotRetentionDays"` // 0 = keep forever
	MaxScreenshotsPerRun    int    `json:"maxScreenshotsPerRun"`    // 0 = unlimited
	CompletesPerWindow      int    `json:"completesPerWindow"`      // completes run as tabs in one window
	BreathingMs             int    `json:"breathingMs"`             // rest between completes
	WindowPauseMs           int    `json:"windowPauseMs"`           // rest between windows
	Incognito               bool   `json:"incognito"`               // batch windows open incognito
	BrowserMode             string `json:"browserMode"`             // auto (in-window) | external | embedded
}

// A config file saved by a Windows editor can carry a byte-order mark, and a single
// unrecognised byte at the top is enough for the whole file to be ignored and every
// setting to fall back to its default - which looks exactly like the setting not working.
func stripBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
}

func readAppConfig(root string) AppConfig {
	c := AppConfig{
		Screenshots: "key", ScreenshotFormat: "jpg", ScreenshotQuality: 60,
		ScreenshotRetentionDays: 7, MaxScreenshotsPerRun: 0,
		CompletesPerWindow: 5, BreathingMs: 150, WindowPauseMs: 5000, Incognito: true,
		BrowserMode: "auto",
	}
	if b, err := os.ReadFile(filepath.Join(root, "config", "app.json")); err == nil {
		_ = json.Unmarshal(stripBOM(b), &c)
	}
	c.Screenshots = strings.ToLower(strings.TrimSpace(c.Screenshots))
	if c.Screenshots != "all" && c.Screenshots != "none" && c.Screenshots != "key" {
		c.Screenshots = "key"
	}
	c.ScreenshotFormat = strings.ToLower(strings.TrimSpace(c.ScreenshotFormat))
	if c.ScreenshotFormat == "jpg" {
		c.ScreenshotFormat = "jpeg"
	}
	if c.ScreenshotFormat != "jpeg" {
		c.ScreenshotFormat = "png"
	}
	if c.ScreenshotQuality < 1 || c.ScreenshotQuality > 100 {
		c.ScreenshotQuality = 60
	}
	if c.ScreenshotRetentionDays < 0 {
		c.ScreenshotRetentionDays = 0
	}
	if c.CompletesPerWindow < 1 {
		c.CompletesPerWindow = 1
	}
	if c.CompletesPerWindow > maxCompletes {
		c.CompletesPerWindow = maxCompletes
	}
	if c.BreathingMs < 0 {
		c.BreathingMs = 0
	}
	if c.WindowPauseMs < 0 {
		c.WindowPauseMs = 0
	}
	c.BrowserMode = strings.ToLower(strings.TrimSpace(c.BrowserMode))
	if c.BrowserMode != "external" && c.BrowserMode != "embedded" {
		c.BrowserMode = "auto"
	}
	return c
}

func shotExt(format string) string {
	if format == "jpeg" {
		return ".jpg"
	}
	return ".png"
}

//go:embed engine/header.js engine/detect.js engine/helpers.js engine/accordion.js engine/analyze.js engine/dispatch.js engine/adapters.js engine/adapters-custom.js engine/navigate.js
var engineFS embed.FS

// engineFiles keeps the pipeline in a fixed order: what the page is (detect), how to
// read it (helpers), how to answer it (adapters) and when Next may be pressed
// (navigate). dispatch.js sits between the analyzer and the adapters: it picks the one
// interaction model the page is running on, and the custom adapters plus the contextual
// Next both read that choice. The runner still injects one string because CDP
// evaluates one call.
var engineFiles = []string{
	"engine/header.js",
	"engine/detect.js",
	"engine/helpers.js",
	"engine/accordion.js",
	"engine/analyze.js",
	"engine/dispatch.js",
	"engine/adapters.js",
	"engine/adapters-custom.js",
	"engine/navigate.js",
}

// buildEngineJS assembles the injected script out of src/engine.
func buildEngineJS() string {
	var sb strings.Builder
	for i, name := range engineFiles {
		b, err := engineFS.ReadFile(name)
		if err != nil {
			panic("engine script missing: " + err.Error())
		}
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.TrimRight(string(b), "\r\n"))
	}
	return sb.String()
}

var engineJS = buildEngineJS()

func screenshot(w *WS, id int, path, format string, quality int) error {
	params := map[string]any{"format": format, "captureBeyondViewport": false}
	if format == "jpeg" {
		params["quality"] = quality
	}
	r, e := w.call(id, "Page.captureScreenshot", params)
	if e != nil {
		return e
	}
	var x struct {
		Data string `json:"data"`
	}
	if e = json.Unmarshal(r, &x); e != nil {
		return e
	}
	b, e := base64.StdEncoding.DecodeString(x.Data)
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0644)
}
func appRoot() string {
	exe, err := os.Executable()
	if err == nil {
		if dir, err := filepath.Abs(filepath.Dir(exe)); err == nil {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func freePort() (int, net.Listener, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	return port, l, nil
}

// uiPush mirrors every launcher log line into the desktop window's activity
// list when one is open. It is set by the GUI before a batch starts and cleared
// again when the window closes; the console launcher simply leaves it nil.
var uiPush func(string)

func logLine(f *os.File, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), message)
	if consoleUI != nil {
		consoleUI.onLog(message)
	} else {
		fmt.Print(line)
	}
	if f != nil {
		_, _ = f.WriteString(line)
		_ = f.Sync()
	}
	if uiPush != nil {
		uiPush(strings.TrimSuffix(line, "\n"))
	}
}

func waitForEnter(in *bufio.Reader, prompt string) {
	fmt.Print(prompt)
	_, _ = in.ReadString('\n')
}

// findCurrentTarget locates the tab to talk to: our own tab first, then the requested URL,
// then the first real (non-Chrome-blank) page, and only as a last resort any page at all.
func findCurrentTarget(port int, currentURL, preferID string) (Target, error) {
	ts, err := jsonTargets(port)
	if err != nil {
		return Target{}, err
	}
	if preferID != "" {
		for _, t := range ts {
			if t.Type == "page" && t.WebSocketDebuggerURL != "" && t.Id == preferID {
				return t, nil
			}
		}
	}
	var best Target
	for _, t := range ts {
		if t.Type != "page" || t.WebSocketDebuggerURL == "" {
			continue
		}
		if currentURL != "" && t.URL == currentURL {
			return t, nil
		}
		if best.WebSocketDebuggerURL == "" && !blankTargetURL(t.URL) {
			best = t
		}
	}
	if best.WebSocketDebuggerURL == "" {
		for _, t := range ts {
			// Last resort still never hands out a data: page: in-window mode
			// that would be the app's own UI, and driving it would wipe the
			// window's interface.
			if t.Type == "page" && t.WebSocketDebuggerURL != "" && !strings.HasPrefix(t.URL, "data:") {
				return t, nil
			}
		}
	}
	if best.WebSocketDebuggerURL == "" {
		return Target{}, errors.New("no page target available")
	}
	return best, nil
}

func main() {
	if runtime.GOOS != "windows" {
		fmt.Println("Windows build only.")
		return
	}
	// One copy of the app per session: a second launch hands its link to the
	// window that is already open instead of starting a second batch behind it.
	if !acquireSingleInstance(`Local\SurveyAutopilot_Instance`) {
		activateExistingInstance()
		return
	}
	root := appRoot()
	_ = os.MkdirAll(filepath.Join(root, "data"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "reports"), 0755)
	_ = os.MkdirAll(filepath.Join(root, "screenshots"), 0755)
	cfg := readAppConfig(root)
	cleanupStaleProfiles(root)
	cleanupOldScreenshots(root, cfg.ScreenshotRetentionDays)
	// Always start the dedicated terminal-only interface.
	runConsole(root, cfg)
}

// runConsole is the terminal launcher: read an authorized link and a complete
// count from stdin, then run the batch under a log file.
func runConsole(root string, cfg AppConfig) {
	view := newConsoleRenderer(os.Stdout, prepareConsole())
	consoleUI = view
	defer func() { consoleUI = nil }()
	view.header()
	view.section("NEW STUDY", "Configure an authorized QA run. Each study keeps its own memory.")
	in := bufio.NewReader(os.Stdin)
	logPath := filepath.Join(root, "data", "launcher.log")
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}()
		logLine(logFile, "Survey Autopilot — portable console QA launcher")
	logLine(logFile, "Application root: %s", root)
	view.prompt("01   STUDY LINK", "Paste the authorized http:// or https:// testing URL.")
	u, err := in.ReadString('\n')
	if err != nil && strings.TrimSpace(u) == "" {
		view.event("ERROR", rose, "No link supplied. No run was started.")
		return
	}
	u = strings.TrimSpace(u)
	if !regexp.MustCompile(`^https?://`).MatchString(u) {
		logLine(logFile, "Invalid URL")
		view.event("ERROR", rose, "Invalid link; provide a full HTTP(S) URL.")
		view.prompt("CLOSE", "Press Enter to exit.")
		_, _ = in.ReadString('\n')
		return
	}
	mem := openMemory(root, u)
	if mem.readFailed {
  view.event("ERROR", rose, "Study memory could not be read; see data/launcher.log.")
 } else if mem.Surveys[memoryFingerprint(u)] != nil {
  view.detail("Study memory", "Loaded automatically")
 } else {
  view.detail("Study memory", "New study")
 }
	total := promptCompletes(in, logFile)
	view.section("RUNNING", "")
	view.detail("Target", fmt.Sprintf("%d completed run(s)", total))
	view.target = total
	batch := runBatch(root, u, total, cfg, mem, logFile, nil)
	view.finish(batch)
	view.prompt("ALL DONE", "Press Enter to close the console. Your reports are saved.")
	_, _ = in.ReadString('\n')
}

// runBatch drives the whole job: `total` completes of one link, a browser window per
// group of them, a report per run. It is the same work the console launcher does,
// extracted so the desktop window can start it without a terminal prompt.
func runBatch(root, u string, total int, cfg AppConfig, mem *MemoryStore, logFile *os.File, progress func(string)) BatchReport {
	perWindow := cfg.CompletesPerWindow
	if embeddedCDPPort(cfg) > 0 && total > 0 {
		// One pane, one window: every complete reuses the in-window browser,
		// so the whole batch is a single session with a tab swap per run.
		perWindow = total
	}
	windowCount := (total + perWindow - 1) / perWindow
	// Every batch numbers its own reports, so one batch's run-1 never overwrites the last
	// one's and the record of what each attempt chose survives.
	batchSeq++
	batch := BatchReport{StartedAt: time.Now().Format(time.RFC3339), URL: u, Requested: total, Windows: windowCount, StatusCounts: map[string]int{}}
	logLine(logFile, "Plan: %d complete(s) in %d window(s) of up to %d tab(s); %d ms between completes, %d ms between windows.",
		total, windowCount, perWindow, cfg.BreathingMs, cfg.WindowPauseMs)
	runNo := 1
	windowNo := 0
	abort := false
	// The request is a number of COMPLETES, so a screened-out run does not use one up:
	// the batch keeps launching runs, each on a fresh session with what it has learned,
	// until it holds that many completes. The attempt ceiling is what stops a study
	// nothing can pass from running for ever.
	attemptCeiling := total * maxRunsPerComplete
	if attemptCeiling < total {
		attemptCeiling = total
	}
	logLine(logFile, "Target: %d complete(s); up to %d run(s) will be attempted to reach it.", total, attemptCeiling)
	logLine(logFile, "Attention-check key: %d rule(s) loaded for this study.", len(readRedHerring(root, u)))
	for runNo <= attemptCeiling && batch.Completed < total && !abort {
		windowNo++
		if windowCount <= 1 && windowNo > 1 {
			logLine(logFile, "=== Continuing in window 1 (attempt pass %d; this app has one in-window browser) ===", windowNo)
		} else {
			logLine(logFile, "=== Window %d of %d ===", windowNo, windowCount)
		}
		sess, lstatus, lerr := launchBrowser(root, logFile, cfg)
		if lstatus != "" {
			logLine(logFile, "Window %d could not start (%s): %v", windowNo, lstatus, lerr)
			report := RunReport{StartedAt: time.Now().Format(time.RFC3339), Run: runNo, URL: u, Status: lstatus, Persona: readPersona(root), State: AnswerState{Values: map[string]any{}, SelectedLabels: []string{}}}
			if lerr != nil {
				report.Errors = append(report.Errors, lerr.Error())
			}
			saveReport(root, report)
			saveReportAs(root, runReportName(runNo, total), report)
			batch.Runs = append(batch.Runs, BatchRun{Run: runNo, Window: windowNo, Status: report.Status, Report: runReportName(runNo, total), Errors: report.Errors})
			batch.StatusCounts[report.Status]++
			sess.stop(logFile, total > 1)
			abort = true
			break
		}
		inWindow := 0
		for runNo <= attemptCeiling && inWindow < perWindow && batch.Completed < total {
			if total > 1 {
				logLine(logFile, "=== Run %d (pass %d, tab %d) - complete %d of %d wanted, attempt %d of up to %d ===", runNo, windowNo, inWindow+1, batch.Completed+1, total, runNo, attemptCeiling)
			}
			started := time.Now()
			report := runComplete(root, u, readPersona(root), mem, logFile, runNo, total, cfg, sess, inWindow == 0)
			if progress != nil {
				// The window counts completes, not runs: a run that was screened out has
				// not put one in the hand, and a counter that said otherwise was counting
				// attempts while the job asked for completes.
				progress(fmt.Sprintf("run %d of %d finished in %s: %s; %d of %d complete", runNo, total, time.Since(started).Round(time.Millisecond), report.Status, batch.Completed, total))
			}
			batch.Runs = append(batch.Runs, BatchRun{Run: runNo, Window: windowNo, Status: report.Status, ReentryID: report.ReentryID, Steps: report.Steps, DurationMS: time.Since(started).Milliseconds(), Errors: report.Errors, Learned: report.Learned, Report: runReportName(runNo, total)})
			batch.StatusCounts[report.Status]++
			if report.Status == "completed" {
				batch.Completed++
			}
			if consoleUI != nil {
				consoleUI.runFinished(runNo, report.Status, batch.Completed, total)
			}
			if total > 1 {
				saveReportAs(root, runReportName(runNo, total), report)
				logLine(logFile, "Run %d finished: %s (test ID %s) - %d of %d completes in hand", runNo, report.Status, report.ReentryID, batch.Completed, total)
				for _, l := range report.Learned {
					logLine(logFile, "Run %d learned: %s", runNo, l)
				}
			}
			inWindow++
			runNo++
			if fatalStatus(report.Status) {
				logLine(logFile, "Run %d failed with %s; aborting the remaining runs.", runNo-1, report.Status)
				abort = true
				break
			}
			// The batch is not abandoned over a screened-out profile: the next run mints a
			// new test ID and replays the profile with what the store has learned, which is
			// how a study that hides its rule is searched. Only the complete count decides
			// when the job is done.
			if report.Status == "screenout_pattern" || report.Status == "screenout_or_termination" {
				logLine(logFile, "Run %d: %s (%d of %d completes so far); continuing until %d completes are in.", runNo-1, report.Status, batch.Completed, total, total)
			}
			if runNo <= total && inWindow < perWindow && cfg.BreathingMs > 0 {
				logLine(logFile, "Breathing %d ms before run %d.", cfg.BreathingMs, runNo)
				time.Sleep(time.Duration(cfg.BreathingMs) * time.Millisecond)
			}
		}
		if sess != nil && sess.embedded {
			// The pane belongs to the app window and outlives the batch: hand it back
			// with its fingerprint page showing, so a batch that needs another pass can
			// pick the same pane up again.
			embeddedPaneReset()
			sess.detach()
			logLine(logFile, "In-window pane handed back after %d attempt(s) in window %d.", inWindow, windowNo)
		} else if abort {
			sess.detach()
			logLine(logFile, "Browser left open for review after an unresolved run.")
		} else if total > 1 {
			sess.stop(logFile, true)
			logLine(logFile, "Window %d closed after %d complete(s).", windowNo, inWindow)
		} else {
			sess.detach()
			logLine(logFile, "Browser process is intentionally NOT terminated by the launcher.")
		}
		if !abort && runNo <= attemptCeiling && batch.Completed < total && cfg.WindowPauseMs > 0 {
			if windowCount <= 1 {
				logLine(logFile, "Resting %d ms before the next pass on the same in-window browser.", cfg.WindowPauseMs)
			} else {
				logLine(logFile, "Resting %d ms before window %d.", cfg.WindowPauseMs, windowNo+1)
			}
			time.Sleep(time.Duration(cfg.WindowPauseMs) * time.Millisecond)
		}
	}
	batch.FinishedAt = time.Now().Format(time.RFC3339)
	batch.AvoidRules = mem.avoidCount(memoryFingerprint(u))
	saveBatchReport(root, batch)
	if total > 1 {
		logLine(logFile, "Batch finished: %d of %d completed across %d window(s). Report: %s", batch.Completed, total, windowCount, filepath.Join(root, "reports", "batch-report.json"))
		if batch.AvoidRules > 0 {
			logLine(logFile, "Survey memory now holds %d answer(s) to avoid for this survey (%s).", batch.AvoidRules, filepath.Join(root, "data", "survey-memory.json"))
		}
	}
	return batch
}

// runComplete executes one complete survey attempt inside an already-open window: a tab
// (the window's launch tab for the first complete, a fresh tab after that), a unique test
// ID, the answer loop and the report. It never prompts and never closes the window.
func runComplete(root, u string, persona Persona, mem *MemoryStore, logFile *os.File, runNo, total int, cfg AppConfig, sess *BrowserSession, firstInWindow bool) RunReport {
	// leftSurvey compares the browser's URL against this one, so the run's own address is
	// what counts as "still the survey".
	surveyURL = u
	reentryID, reentryNumber, reentryErr := nextReentryID(root, persona)
	if reentryErr != nil {
		logLine(logFile, "Could not mint a test ID: %v", reentryErr)
		report := RunReport{StartedAt: time.Now().Format(time.RFC3339), Run: runNo, URL: u, Status: "reentry_id_error", Persona: persona, ReentryID: "", State: AnswerState{Values: map[string]any{}, SelectedLabels: []string{}}}
		report.Errors = append(report.Errors, reentryErr.Error())
		saveReport(root, report)
		return report
	}
	persona.ReentryID = reentryID
	report := RunReport{StartedAt: time.Now().Format(time.RFC3339), Run: runNo, URL: u, Status: "running", Persona: persona, ReentryID: persona.ReentryID, State: AnswerState{Values: map[string]any{}, SelectedLabels: []string{}}}
	logLine(logFile, "Reentry template: %s", persona.ReentryTemplate)
	logLine(logFile, "Sequence slot: %d", reentryNumber)
	logLine(logFile, "Synthetic test respondent reentry ID: %s", persona.ReentryID)
	screenshotDir := filepath.Join(root, "screenshots")
	if total > 1 {
		screenshotDir = filepath.Join(root, "screenshots", fmt.Sprintf("run-%03d", runNo))
	}
	// One tab per complete. The new tab is opened before the old one is closed so the
	// window never drops to zero tabs (Chrome would exit).
	if !firstInWindow && sess.current != nil {
		if nt, te := sess.createTab(logFile); te == nil {
			if sess.ws != nil {
				_ = sess.ws.c.Close()
				sess.ws = nil
			}
			sess.closeTab(sess.current.Id)
			logLine(logFile, "Closed previous tab %s.", shortID(sess.current.Id))
			sess.current = nt
			logLine(logFile, "Opened fresh tab %s for run %d.", shortID(nt.Id), runNo)
		} else {
			logLine(logFile, "Could not open a fresh tab (%v); reusing the current one.", te)
		}
	}
	if sess.current == nil || sess.current.WebSocketDebuggerURL == "" {
		logLine(logFile, "No tab available for run %d", runNo)
		report.Status = "cdp_error"
		report.Errors = append(report.Errors, "no tab available")
		saveReport(root, report)
		return report
	}
	if sess.ws != nil {
		_ = sess.ws.c.Close()
		sess.ws = nil
	}
	target := *sess.current
	w, e := wsConnect(target.WebSocketDebuggerURL)
	if e != nil {
		logLine(logFile, "WebSocket connection failed: %v", e)
		report.Status = "cdp_error"
		report.Errors = append(report.Errors, e.Error())
		saveReport(root, report)
		return report
	}
	sess.ws = w
	// The socket is deliberately not closed here: the next complete needs it alive to open
	// its tab (and in incognito only a live page session can do that). It is closed when
	// the tab is swapped or when the window stops.
	_, _ = w.call(1, "Runtime.enable", nil)
	_, _ = w.call(2, "Page.enable", nil)
	logLine(logFile, "Connected to browser CDP (tab %s)", shortID(target.Id))
	if total > 1 {
		// The window is reused for several completes: wipe cookies and site storage so the
		// survey treats this test ID as a brand new respondent.
		isolateRespondent(w, 3, u)
	}
	// One complete may still follow a run that was stopped half way, so the session is
	// cleared either way: a resumed survey drops the new attempt on the question the last
	// one was terminated on, and a link that is entered that way stops being a test.
	isolateRespondent(w, 4, u)
	if _, e = w.call(6, "Page.navigate", map[string]any{"url": u}); e != nil {
		logLine(logFile, "Navigation failed: %v", e)
		report.Status = "navigation_error"
		report.Errors = append(report.Errors, e.Error())
		saveReport(root, report)
		return report
	}
	logLine(logFile, "Survey navigation requested: %s", u)
	waitDocSettled(w, 7)
	// capture writes one screenshot for the step just logged, honouring the storage budget.
	capture := func(step int) bool {
		if cfg.Screenshots == "none" {
			return false
		}
		shots := 0
		for _, s := range report.StepsLog {
			if s.Screenshot != "" {
				shots++
			}
		}
		if cfg.MaxScreenshotsPerRun > 0 && shots >= cfg.MaxScreenshotsPerRun {
			return false
		}
		_ = os.MkdirAll(screenshotDir, 0755)
		p := filepath.Join(screenshotDir, fmt.Sprintf("step-%03d%s", step, shotExt(cfg.ScreenshotFormat)))
		if screenshot(w, 2000+step, p, cfg.ScreenshotFormat, cfg.ScreenshotQuality) == nil {
			if n := len(report.StepsLog); n > 0 {
				report.StepsLog[n-1].Screenshot = p
			}
			return true
		}
		return false
	}
	var wsMu sync.Mutex
	lastSignature := ""
	// A page that has not changed for several steps is being answered and refused, and
	// the answer to that is a different set of answers - not a navigation. This is what
	// stops a page from being clicked at until the step cap, now that nothing walks away
	// from a page any more.
	samePageSteps := 0
	repeatCount := 0
	// A page that is replaced while the engine runs leaves nothing to read: no URL, no
	// signature. That is not progress and must not look like a new page either, or the
	// run would repeat the same blank step until it hits the step limit.
	emptySteps := 0
	// A free-text box that hangs off a closed question is filled only when the page
	// insists on it: either it is marked required, or the page has already refused the
	// answers it was given. This remembers that the page said no.
	needOpen := false
	// While a survey keeps screening out, the profile that failed is replayed and one
	// question is varied. Both are read by the engine through the memory payload.
	probeQuestion, probeExclude := "", ""
	pinned := map[string]string{}

	// A validation message blocks the run but must not end it: the page is re-answered
	// a few times, and when it still refuses, the attempt moves on to a fresh test ID.
	// Waiting on one page is how a run times out.
	validationRounds := 0
	const maxValidationRounds = 4
	returningRetries := 0
	answeredInAttempt := 0
	// What this run has learned about the survey, and what it is still allowed to learn.
	memFP := memoryFingerprint(u)
	// A run starts where the last one stopped: if the previous profile was screened out,
	// it is replayed here with one question changed, so a survey that hides its rule is
	// searched a question at a time instead of guessed at again from scratch.
	// A profile that has already been refused twice unchanged is the point at which the
	// search starts moving one answer; until then it is replayed exactly.
	const plainRetriesBeforeProbe = 2
	plainAttempts := 0
	reopen := map[string]bool{}
	forceNext := false
	unansweredStreak := 0
	lastModelAnswered := -1
	lastUnanswered := 1 << 30
	var accordionWaitSince time.Time
	var renderWaitSince time.Time
	learnCount := 0
	clearCount := 0
	screenoutRetries := 0
	// Leaving a page is not a rejection: it has its own budget, so an unreadable page
	// can never use up the retries a screen-out is entitled to.
	pageLeaves := 0
	var lastPairs []answerPair
	var lastSubmitted []answerPair
	lastSuspectRecorded := false
	// runPairs is every selection this run submitted, across all of its attempts: it is
	// the answer set kept on a screen-out or a validation and cleared on a completion.
	var runPairs []answerPair
	// attemptPairs is every selection made since this attempt started. A screen-out is a
	// verdict on the whole profile, so the next attempt replays all of it, not just the
	// answers that happened to sit on the last page.
	attemptPairs := []answerPair{}
	// A rejection is read from the answers closest to it: at most this many of them, and
	// only this many of them are ruled out. One screen-out is evidence about the answer
	// next to it, not about the whole profile, and a study is qualified one answer at a
	// time.
	// noteProgress remembers an attempt that carried the profile further into the survey
	// than any attempt before it. Nothing else is known about a study that screens people
	// out, so "this got us past that question" is what the next run is built from.
	attemptStart := 1
	lastStep := 0
	noteProgress := func(atStep int) {
		if mem.noteFurthest(memFP, attemptPairs, atStep-attemptStart) {
			mem.save()
			logLine(logFile, "Observed: this profile reached %d step(s); %d answer(s) are not treated as a verified success.", atStep-attemptStart, len(attemptPairs))
		}
	}
	// blameEnd remembers selections under this survey so the next run does not repeat
	// them. budget limits how many of them one call may add.
	blameEnd := func(pairs []answerPair, reason string, budget int) {
		if reason != "screenout" || lastSuspectRecorded || len(lastSubmitted) != 1 { return }
		p := lastSubmitted[0]
		if ok, saveErr := mem.recordScreenoutSuspect(memFP, lastSubmitted, reason); ok {
			lastSuspectRecorded = true
			if saveErr != nil {
				report.Errors = append(report.Errors, "could not save termination memory: "+saveErr.Error())
				logLine(logFile, "Could not save termination memory: %v", saveErr)
			} else {
				report.Learned = append(report.Learned, fmt.Sprintf("termination suspect: %s = %s", p.Question, p.Option))
				logLine(logFile, "Termination followed %q on %q; saved it as an unproven suspect for this survey.", p.Option, p.Question)
			}
		}
	}

	// leavePage is how the runner refuses to sit on a page. A page that will not take the
	// answers - a rejection that did not clear, a widget that never renders - is left
	// behind rather than looked at again: the answers it refused are remembered and the
	// attempt restarts from a fresh test ID. Sitting on one page is how a run times out.
	leavePage := func(atStep int, why string) bool {
		// Two budgets, one path. Walking away from an unreadable page is the first way
		// out; when those are used up, the attempt still restarts on the screen-out
		// budget, because an interview that ended leaves a page nothing can read and the
		// run has to carry on looking for a complete.
		if pageLeaves < maxPageLeaves {
			pageLeaves++
		} else if screenoutRetries < maxScreenoutRetries {
			screenoutRetries++
			noteProgress(atStep)
			blameEnd(attemptPairs, "screenout", maxLearnPerRun-learnCount)
			logLine(logFile, "Leaving the page at step %d (%s) with the page budget spent; recording it as the end of this attempt and starting again (attempt %d of %d).", atStep, why, screenoutRetries, maxScreenoutRetries)
		} else {
			return false
		}
		noteProgress(atStep)
		blameEnd(attemptPairs, why, maxLearnPerRun-learnCount)
		if newID, newNumber, idErr := nextReentryID(root, persona); idErr == nil && newID != persona.ReentryID {
			persona.ReentryID = newID
			report.ReentryID = newID
			if pageLeaves <= maxPageLeaves {
				logLine(logFile, "Leaving the page at step %d (%s); restarting with fresh test ID %s (leave %d of %d, sequence %d).", atStep, why, newID, pageLeaves, maxPageLeaves, newNumber)
			}
		}
		report.Errors = append(report.Errors, fmt.Sprintf("step %d: left the page (%s) instead of waiting on it; restarted with test ID %s", atStep, why, persona.ReentryID))
		answeredInAttempt = 0
		attemptPairs = []answerPair{}
		lastSubmitted = nil
		lastSuspectRecorded = false
		attemptStart = atStep + 1
		lastSignature = ""
		repeatCount = 0
		validationRounds = 0
		unansweredStreak = 0
		forceNext = false
		freshSession(w, u, 5290000+atStep)
		if _, ne := w.call(5300000+atStep, "Page.navigate", map[string]any{"url": u}); ne != nil {
			report.Status = "navigation_error"
			report.Errors = append(report.Errors, ne.Error())
			return false
		}
		waitDocSettled(w, 4300000+atStep)
		time.Sleep(300 * time.Millisecond)
		return true
	}
	// The cap is a runaway guard, not a budget: a long brand audit asks one question
	// per brand and runs to several hundred steps at a few dozen milliseconds each, so a
	// survey that is still making progress must never be cut off by it.
	// The scoped answer key does not change within one run. Reusing it avoids
	// reopening and decoding the executable or config for every card.
	redHerringRules := readRedHerring(root, u)
	staleTargets := true
	for step := 1; step <= 900; step++ {
		lastStep = step
		stepStarted := time.Now()
		// Re-discover our tab periodically, and always after a step the engine could not
		// read: a postback keeps the same tab, so paying an HTTP round trip for every
		// question is time the survey does not need.
		var ts []Target
		var targetErr error
		if staleTargets || step == 1 || step%4 == 0 {
			staleTargets = false
			ts, targetErr = jsonTargets(sess.port)
		} else {
			ts = []Target{target}
		}
		if targetErr != nil {
			if step > 1 && strings.Contains(strings.ToLower(targetErr.Error()), "connection") {
				logLine(logFile, "Browser target discovery failed at step %d: %v", step, targetErr)
			}
		} else {
			// Our own tab first; if the survey replaced it, follow the first real page
			// (never Chrome's own blank/newtab page).
			chosen := Target{}
			if sess.current != nil {
				for _, t := range ts {
					if t.Type == "page" && t.WebSocketDebuggerURL != "" && t.Id == sess.current.Id {
						chosen = t
						break
					}
				}
			}
			if chosen.WebSocketDebuggerURL == "" {
				for _, t := range ts {
					if t.Type == "page" && t.WebSocketDebuggerURL != "" && !blankTargetURL(t.URL) {
						chosen = t
						break
					}
				}
			}
			if chosen.WebSocketDebuggerURL != "" {
				if chosen.WebSocketDebuggerURL != target.WebSocketDebuggerURL {
					wsMu.Lock()
					_ = w.c.Close()
					wsMu.Unlock()
					nw, ce := wsConnect(chosen.WebSocketDebuggerURL)
					if ce == nil {
						w = nw
						target = chosen
						tc := chosen
						sess.current = &tc
						sess.ws = nw
						_, _ = w.call(500000+step, "Runtime.enable", nil)
						_, _ = w.call(600000+step, "Page.enable", nil)
						logLine(logFile, "Reconnected to active browser target at step %d", step)
					}
				}
			}
		}

		select {
		case <-sess.done:
			errTxt := "unknown error"
			if sess.waitErr != nil {
				errTxt = sess.waitErr.Error()
			}
			logLine(logFile, "Browser process exited during test: %s", errTxt)
			report.Errors = append(report.Errors, "Browser process exited during test")
			report.Status = "browser_exited"
			goto END_RUN
		default:
		}

		reopenList := make([]string, 0, len(reopen))
		for q := range reopen {
			reopenList = append(reopenList, q)
		}
		mem.forURL(u)
		avoidList := mem.avoidFor(memFP)
		memPayload := map[string]any{"avoid": avoidList, "reopen": reopenList, "forceNext": forceNext,
			// What has already completed this survey, the answer sets that ended a run
			// badly, how each known answer has scored, and which single question this
			// attempt is here to vary.
			"prefer": mem.preferFor(memFP), "banned": []string{}, "needOpen": needOpen,
			"score": map[string]any{}, "pin": pinned, "probeQ": probeQuestion, "probeExclude": probeExclude,
			// The attention checks the study ends an interview over. They are answered from
			// config/red-herring.json, before anything else, and never re-opened: a trap
			// answer here is the one mistake that cannot be learned back out of.
			"redHerring": redHerringRules}
		forceUsed := forceNext
		forceNext = false // this step spent the forced Next
		expr := strings.NewReplacer(
			"PERSONA", mustJSON(persona),
			"MEMORY", mustJSON(memPayload),
			"__ANSWERED__", strconv.Itoa(answeredInAttempt),
		).Replace(engineJS)
		var m map[string]any
		v := ""
		if os.Getenv("SAP_DUMP_EXPR") != "" {
			_ = os.WriteFile(filepath.Join(root, "data", fmt.Sprintf("expr-step-%02d.js", step)), []byte(expr), 0644)
		}
		// Pages can navigate underneath us (ASP.NET postbacks), which kills the CDP session.
		// Re-discover the target, reconnect and retry instead of failing the whole run.
		for attempt := 0; ; attempt++ {
			var evErr error
			m = nil
			tEval := time.Now()
			if v, evErr = js(w, 1000+step*10+attempt, expr); evErr == nil && json.Unmarshal([]byte(v), &m) == nil {
				if os.Getenv("SAP_EVAL_MS") != "" {
					logLine(logFile, "Step %d eval ok in %dms (target %s)", step, time.Since(tEval).Milliseconds(), target.Id)
				}
				// A widget page parks the answers its passes committed on the page, because
				// the pass that rated an item is often the pass whose postback destroyed the
				// context it was read in. The result is in hand now, so they are cleared and
				// never counted a second time.
				break
			}
			if attempt >= 5 {
				staleTargets = true
				if evErr != nil {
					logLine(logFile, "Step %d CDP/JS error after %dms: %v (target %s %s)", step, time.Since(tEval).Milliseconds(), evErr, target.Id, target.URL)
					report.Errors = append(report.Errors, fmt.Sprintf("step %d: %v", step, evErr))
				} else {
					report.Errors = append(report.Errors, "invalid engine response: "+v)
				}
				// Before calling the run a failure, look at where the browser actually is:
				// a survey that has already ended the interview on its own end page looks
				// exactly like a dead page from here, and a page that cannot be read is not
				// a page worth staying on.
				if where, we := js(w, 1002+step*10, "String(location.href)"); we == nil {
					if here := strings.Trim(str(where), `"`); here != "" && !browserDeadPage(here) {
						if isQualifiedCompletionURL(here) {
							report.Status = "completed"
							logLine(logFile, "Step %d: verified qualification on %s after the page evaluation failed.", step, here)
							goto END_RUN
						}
						if leftSurvey(here) {
							if isTerminatedURL(here) {
								report.Status = "screenout_or_termination"
							} else {
								report.Status = "unverified_exit"
							}
							report.Errors = append(report.Errors, fmt.Sprintf("step %d: survey redirected to %s without a verified Qual.html result", step, here))
							logLine(logFile, "Step %d: redirected to %s; not counting as completed.", step, here)
							goto END_RUN
						}
					}
				}
				if leavePage(step, "the page could not be read any further") {
					report.Errors = report.Errors[:len(report.Errors)-1]
					continue
				}
				report.Status = "automation_error"
				goto END_RUN
			}
			if attempt == 0 {
				logLine(logFile, "Step %d CDP/JS error after %dms: %v (target %s %s) Ã¢â‚¬â€ reconnecting and retrying", step, time.Since(tEval).Milliseconds(), evErr, target.Id, target.URL)
				if os.Getenv("SAP_EVAL_MS") != "" {
					tQ := time.Now()
					qv, qe := js(w, 990000+step, "'PROBE:'+location.href+':'+document.title+':'+document.body.children.length")
					logLine(logFile, "  trivial probe after failure: err=%v in %dms -> %s", qe, time.Since(tQ).Milliseconds(), strings.TrimSpace(qv))
				}
			}
			time.Sleep(time.Duration(400*(attempt+1)) * time.Millisecond)
			prefer := ""
			if sess.current != nil {
				prefer = sess.current.Id
			}
			if nt, ne := findCurrentTarget(sess.port, "", prefer); ne == nil {
				if nw, ce := wsConnect(nt.WebSocketDebuggerURL); ce == nil {
					_ = w.c.Close()
					w = nw
					target = nt
					tc := nt
					sess.current = &tc
					sess.ws = nw
					_, _ = w.call(700000+step*10+attempt, "Runtime.enable", nil)
					_, _ = w.call(800000+step*10+attempt, "Page.enable", nil)
				}
			}
			// The engine clicks Next from inside the evaluate call, so a postback destroys the
			// execution context and kills the promise. Re-running the engine while the new
			// document is still loading would click Next again, so wait for it to settle first.
			if w != nil {
				waitDocSettled(w, 910000+step*10+attempt)
			}
		}
		// A widget that positions itself from a CSS variable or a captured pointer
		// ignores events the page builds for itself, so the drag is repeated here as
		// real browser input: press, move in steps, release. The engine is then run
		// once more straight away, so the step that placed the last card also presses
		// Next instead of spending another round trip to notice that it can.
		if drs, ok := m["dragRequests"].([]any); ok && len(drs) > 0 {
			dragged := 0
			// A drop the widget only records on the re-read is an answer the first pass
			// could not see, and those answers are what the step is remembered by: they
			// are collected here so a brand that committed on the last drop is not lost
			// from the report and from the survey memory.
			extraPairs := []answerPair{}
			extraLabels := []string{}
			// Each drop re-lays the page out, so the page is read again after every
			// drag instead of trusting coordinates measured before it. The loop stops
			// as soon as the engine stops asking for drags, which is also the moment
			// the page is complete and Next can be pressed.
			for round := 0; round < 14; round++ {
				for _, d := range drs {
					dm, ok := d.(map[string]any)
					if !ok {
						continue
					}
					x1, y1 := toFloat(dm["x1"]), toFloat(dm["y1"])
					x2, y2 := toFloat(dm["x2"]), toFloat(dm["y2"])
					if x2 == 0 && y2 == 0 {
						continue
					}
					_, _ = w.call(4700000+step, "Input.dispatchMouseEvent", map[string]any{
						"type": "mousePressed", "x": x1, "y": y1, "button": "left", "buttons": 1, "clickCount": 1, "pointerType": "mouse"})
					// A scripted slider often ignores a movement that starts immediately;
					// a short pause after the press makes the drag register.
					time.Sleep(8 * time.Millisecond)
					const dragSteps = 4
					for i := 1; i <= dragSteps; i++ {
						f := float64(i) / float64(dragSteps)
						_, _ = w.call(4710000+step*16+i, "Input.dispatchMouseEvent", map[string]any{
							"type": "mouseMoved", "x": x1 + (x2-x1)*f, "y": y1 + (y2-y1)*f, "button": "left", "buttons": 1, "pointerType": "mouse"})
						time.Sleep(3 * time.Millisecond)
					}
					_, _ = w.call(4720000+step, "Input.dispatchMouseEvent", map[string]any{
						"type": "mouseReleased", "x": x2, "y": y2, "button": "left", "buttons": 0, "clickCount": 1, "pointerType": "mouse"})
					time.Sleep(20 * time.Millisecond)
					dragged++
				}
				lastSignature = ""
				repeatCount = 0
				v2, e2 := js(w, 1500+step*20+round, expr)
				if e2 != nil {
					break
				}
				var m2 map[string]any
				if json.Unmarshal([]byte(v2), &m2) != nil {
					break
				}
				_, _ = js(w, 1501+step*20+round, "(()=>{try{window.__apEmoPending.length=0}catch{}})()")
				for _, p := range parsePairs(m2["selectedPairs"]) {
					dup := false
					for _, q := range extraPairs {
						if q == p {
							dup = true
							break
						}
					}
					if !dup {
						extraPairs = append(extraPairs, p)
					}
				}
				for _, v := range toStrings(m2["selected"]) {
					seen := false
					for _, x := range extraLabels {
						if x == v {
							seen = true
							break
						}
					}
					if !seen {
						extraLabels = append(extraLabels, v)
					}
				}
				next2, ok2 := m2["dragRequests"].([]any)
				if !ok2 || len(next2) == 0 {
					break
				}
				drs = next2
			}
			logLine(logFile, "Step %d: performed %d real mouse drag(s) the page would not accept from scripted events.", step, dragged)
			if len(extraPairs) > 0 || len(extraLabels) > 0 {
				mergeStepAnswers(m, extraPairs, extraLabels)
			}
		}
		sr := StepReport{Step: step, Time: time.Now().Format(time.RFC3339), URL: str(m["url"]), Title: str(m["title"]), Question: str(m["question"]), QuestionType: str(m["questionType"]), Action: str(m["action"]), Validation: toStrings(m["validation"])}
		sr.DurationMS = time.Since(stepStarted).Milliseconds()
		sr.NeedsHuman = boolv(m["needsHuman"])
		if c, ok := m["controls"].(map[string]any); ok && len(c) > 0 {
			sr.Controls = map[string]int{}
			for _, k := range []string{"radios", "roleRadios", "checkboxes", "roleCheckboxes", "selects", "texts", "clickables"} {
				if n, has := c[k]; has {
					sr.Controls[k] = toInt(n)
				}
			}
		}
		if inv, ok := m["inventory"].(map[string]any); ok {
			sr.Iframes = toInt(inv["iframes"])
		}
		if list, ok := m["questions"].([]any); ok {
			for _, it := range list {
				qm, ok := it.(map[string]any)
				if !ok {
					continue
				}
				sr.Questions = append(sr.Questions, QuestionState{Key: str(qm["key"]), Type: str(qm["type"]), N: toInt(qm["n"]), On: toInt(qm["on"]), Grid: qm["grid"] == true})
			}
		}
		sr.OpenQuestions = toStrings(m["openQuestions"])
		// Attention checks are written out one by one: what was answered, and - the one
		// that explains a screen-out nobody expected - a check this build has no rule for.
		// The scan is written out too, including the questions the pass walked past: "no
		// check on this page" and "the pass never fired" look identical from outside the
		// engine, and they are fixed in different files.
		if scan := asMaps(m["redHerringScan"]); len(scan) > 0 {
			parts := []string{}
			for _, d := range scan {
				how := str(d["how"])
				if how != "" {
					how = " via " + how
				}
				extra := ""
				if a := str(d["answer"]); a != "" {
					extra = " -> " + a
				} else if a := str(d["avoided"]); a != "" {
					extra = " -> blocked " + a
				} else if a := str(d["want"]); a != "" {
					extra = " -> wanted " + a
				}
				parts = append(parts, fmt.Sprintf("%s [%s] %q%s%s", str(d["q"]), str(d["did"]), str(d["text"]), how, extra))
			}
			logLine(logFile, "Step %d: attention-check scan (%d rule(s) loaded): %s", step, len(redHerringRules), strings.Join(parts, " / "))
		}
		for _, e := range asMaps(m["redHerring"]) {
			q, did := str(e["q"]), str(e["did"])
			switch did {
			case "answered":
				logLine(logFile, "Step %d: attention check %q answered %q.", step, q, str(e["answer"]))
			case "ANSWER-NOT-FOUND":
				logLine(logFile, "Step %d: attention check %q wanted %q but none of the options matched (%s). Add a rule to config/red-herring.json.", step, q, str(e["want"]), strings.Join(toStrings(e["options"]), " | "))
			case "trap-avoided":
				logLine(logFile, "Step %d: attention check %q avoided trap option %q.", step, q, str(e["avoided"]))
				// Every attention check is written out in full - what it was, what was answered, and above
				// all a check the file has no rule for - because this log is the only place the answer
				// to "what did it do at the red herring" exists.
				if did := str(e["did"]); did == "answered" || did == "answered-safe-option" || did == "trap-avoided" || did == "trap-avoided-unknown-check" || did == "NO-RULE-DO-NOT-GUESS" || did == "NO-OPTIONS-DO-NOT-GUESS" || did == "ANSWER-NOT-FOUND" || did == "CLICK-FAILED" || did == "grid-check-left-to-key" {
					logLine(logFile, "Step %d: RH %s on %q -> answered=%q avoided=%q options=%q text=%q.", step, did, fmt.Sprint(e["q"]), fmt.Sprint(e["answer"]), fmt.Sprint(e["avoided"]), fmt.Sprint(e["options"]), fmt.Sprint(e["text"]))
				}
			case "trap-list-applied":
				logLine(logFile, "Step %d: attention check %q: %s.", step, q, str(e["note"]))
			case "already-answered":
				logLine(logFile, "Step %d: attention check %q was already answered.", step, q)
			}
		}
		if im, ok := m["interaction"].(map[string]any); ok && len(im) > 0 {
			sr.Interaction = im
		}
		if am, ok := m["accordion"].(map[string]any); ok && toInt(am["count"]) > 0 {
			sr.Accordion = am
		}
		if rm, ok := m["instructions"].(map[string]any); ok && toInt(rm["blocked"]) > 0 {
			sr.Instructions = rm
			for _, detail := range asMaps(rm["details"]) {
				logLine(logFile, "Step %d: red instruction needs review: reason=%q question=%q instruction=%q options=%v; no fallback answer selected.", step, str(detail["reason"]), str(detail["question"]), str(detail["instruction"]), detail["options"])
			}
			if len(asMaps(rm["details"])) == 0 {
				logLine(logFile, "Step %d: red instruction needs review (%v), no fallback answer selected.", step, rm["details"])
			}
		}
		if pl, ok := m["probe"].([]any); ok {
			for _, it := range pl {
				if pm, ok := it.(map[string]any); ok {
					sr.Probe = append(sr.Probe, pm)
				}
			}
		}
		report.StepsLog = append(report.StepsLog, sr)
		report.Steps = step
		// Every selection this run submits is logged, so a completed run can clear the
		// answers that worked while a screen-out or a validation keeps them remembered.
		stepPairs := parsePairs(m["selectedPairs"])
		if isNavAction(sr.Action) { lastSubmitted = parsePairs(m["submittedPairs"]) }
		for _, p := range stepPairs {
			runPairs = append(runPairs, p)
			attemptPairs = append(attemptPairs, p)
			report.Selections = append(report.Selections, Selection{Step: step, Question: p.Question, Option: p.Option})
		}
		if vals, ok := m["selected"].([]any); ok {
			cleaned := []string{}
			for _, vv := range vals {
				label := strings.TrimSpace(str(vv))
				if label != "" && !strings.EqualFold(label, "undefined") && !strings.EqualFold(label, "null") {
					cleaned = append(cleaned, label)
				}
			}
			if len(cleaned) > 0 {
				answeredInAttempt += len(cleaned)
				report.State.Values[fmt.Sprintf("step_%03d", step)] = map[string]any{"question": sr.Question, "selected": cleaned}
				report.State.SelectedLabels = append(report.State.SelectedLabels, cleaned...)
			}
		}
		qline := sr.Question
		if sr.Interaction != nil {
			qline = fmt.Sprintf("[%s %d/%d] %s", str(sr.Interaction["kind"]), toInt(sr.Interaction["answered"]), toInt(sr.Interaction["total"]), sr.Question)
		}
		logLine(logFile, "[%03d] %-16s %s", step, sr.Action, qline)
		if len(sr.Validation) > 0 {
			needOpen = true
		} else if isNavAction(sr.Action) {
			needOpen = false
		}
		if strings.Contains(sr.Action, "reentryID_mismatch") {
			logLine(logFile, "Step %d: test ID %s did not persist in the reentry field.", step, persona.ReentryID)
			report.Errors = append(report.Errors, fmt.Sprintf("step %d: test ID %s did not persist in the reentry field", step, persona.ReentryID))
		}
		// A screenshot costs a frame on a long survey, so "key" keeps only the ones that
		// are evidence: a page the survey refused, and the page the run ended on.
		wantShot := cfg.Screenshots == "all" ||
			(cfg.Screenshots == "key" && (len(sr.Validation) > 0 || strings.Contains(sr.Action, "attention-check")))
		if wantShot {
			capture(step)
		}

		if boolv(m["completion"]) && !boolv(m["returningSession"]) && !boolv(m["termination"]) && isQualifiedCompletionURL(str(m["url"])) {
			report.Status = "completed"
			break
		}
		if browserDeadPage(str(m["url"])) {
			logLine(logFile, "Step %d: the browser is showing %s rather than the survey; reloading it.", step, str(m["url"]))
			if leavePage(step, "the browser page went blank") {
				continue
			}
			report.Status = "stalled"
			break
		}
		// The survey can also end the interview by sending the browser to a page of its
		// own. Nothing is answerable there, so the run is finished rather than left
		// looking at a page that is not the survey.
		if leftSurvey(str(m["url"])) {
			low := strings.ToLower(str(m["bodyPreview"]) + " " + str(m["title"]))
			if isTerminatedURL(str(m["url"])) || boolv(m["termination"]) ||
				regexp.MustCompile(`unfortunately|did not qualify|not qualify|does not correspond|screened out|not eligible`).MatchString(low) {
				// Let the ordinary screenout path below record the rejection.
				m["termination"] = true
			} else if isQualifiedCompletionURL(str(m["url"])) && !boolv(m["returningSession"]) {
				report.Status = "completed"
				logLine(logFile, "Step %d: verified survey qualification URL %s.", step, str(m["url"]))
				break
			} else {
				// An outside domain is not necessarily the interview's end page.
				// Do not invent a completion or retry with a fabricated result.
				report.Status = "unverified_exit"
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: unverified survey exit to %s; completion requires Qual.html", step, str(m["url"])))
				logLine(logFile, "Step %d: survey opened %s without qualification evidence; not counting as complete.", step, str(m["url"]))
				break
			}
		}

		// A challenge that asks a person to prove they are one is not something more
		// answering will get past, so the run hands back instead of burning its steps.
		if sr.NeedsHuman && !boolv(m["completion"]) && !boolv(m["termination"]) {
			report.Status = "needs_human"
			report.Errors = append(report.Errors, fmt.Sprintf("step %d: the survey is showing a human-verification challenge", step))
			logLine(logFile, "Step %d: the survey is asking for a human verification; stopping this run.", step)
			break
		}
		if boolv(m["termination"]) {
			low := strings.ToLower(str(m["bodyPreview"]))
			reason := "screenout"
			if strings.Contains(low, "quota") {
				reason = "quota"
			}
			// Whatever ended this attempt, the answers that led here must not be repeated.
			blameEnd(attemptPairs, reason, maxLearnPerRun-learnCount)
			// A profile that already carried a run deep into the survey is replayed
			// exactly, on a fresh session and a fresh test ID, before anything is varied.
			// Changing one answer of a profile that works is how a run that reached the
			// brand audit ends up back at the screener: only when the same profile has
			// been refused unchanged does the search move a single question.
			if plainAttempts < plainRetriesBeforeProbe {
				plainAttempts++
				// Pin the attempt's own answers, or "unchanged" is not unchanged: without
				// this the retry re-picks every screener answer and the run cannot tell a
				// profile that is refused from a profile that was never tried.
				same := map[string]string{}
				for _, p := range attemptPairs {
					if p.Question != "" && p.Option != "" {
						if contentAnswer(p.Question) {
							continue
						}
						same[strings.ToLower(p.Question)] = p.Option
					}
				}
				if len(same) > 0 {
					pinned = same
				}
				logLine(logFile, "Attempt %d: replaying the same %d answer(s) unchanged on a fresh session and test ID (a profile that got this far is not varied on the first try).", plainAttempts, len(same))
			} else if probeQuestion == "" {
				// The same profile has now been refused more than once: this is a rule
				// about the profile, so the next attempt changes a single question and
				// replays the rest, which is what turns it into a search.
				probeQ, probeExcl, pin := mem.nextProbe(memFP, attemptPairs)
				if len(pin) > 0 {
					pinned = pin
				}
				if probeQ != "" {
					probeQuestion, probeExclude = probeQ, probeExcl
					logLine(logFile, "Probe: replaying %d answer(s) from this profile and changing %q (was %q).", len(pin), probeQuestion, probeExclude)
				}
			} else {
				probeQ, probeExcl, pin := mem.nextProbe(memFP, attemptPairs)
				if len(pin) > 0 {
					pinned = pin
				}
				if probeQ != "" {
					probeQuestion, probeExclude = probeQ, probeExcl
				}
			}
			mem.save()
			// When a survey screens out at the same place with nothing left to try, saying so
			// is worth more than three more minutes of the same run.
			// A run screens out more than once per run, so the step number keeps moving; what has to
			// repeat is the end page itself, and that is the fingerprint that matters.
			sig := strings.ToLower(strings.Join(strings.Fields(str(m["title"])+" "+str(m["bodyPreview"])), " "))
			if len(sig) > 160 {
				sig = sig[:160]
			}
			if streak := mem.noteScreenout(memFP, step, sig, mem.probeCanLearn(memFP, probeQuestion)); streak >= maxScreenoutPatternStreak && screenoutRetries >= minRetriesBeforePatternStop {
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: screened out %d times in a row at the same point with nothing new to try; this profile cannot be qualified by answering differently", step, streak))
				report.Status = "screenout_pattern"
				logLine(logFile, "Step %d: screened out %d times in a row with no new information; stopping instead of retrying again.", step, streak)
				mem.save()
				lastPairs = nil
				break
			}
			mem.save()
			lastPairs = nil
			if reason == "screenout" && screenoutRetries < maxScreenoutRetries {
				screenoutRetries++
				noteProgress(step)
				blameEnd(attemptPairs, reason, maxLearnPerRun-learnCount)
				if newID, newNumber, idErr := nextReentryID(root, persona); idErr == nil {
					if newID != persona.ReentryID {
						persona.ReentryID = newID
						report.ReentryID = newID
					}
					logLine(logFile, "Screened out at step %d; retrying with fresh test ID %s (attempt %d of %d, sequence %d).", step, newID, screenoutRetries, maxScreenoutRetries, newNumber)
				} else {
					logLine(logFile, "Screened out at step %d and could not mint a fresh test ID: %v", step, idErr)
				}
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: screened out; retrying with test ID %s", step, persona.ReentryID))
				answeredInAttempt = 0
				attemptPairs = []answerPair{}
				lastSubmitted = nil
				lastSuspectRecorded = false
				attemptStart = step + 1
				lastSignature = ""
				repeatCount = 0
				unansweredStreak = 0
				forceNext = false
				freshSession(w, u, 4990000+step)
				staleTargets = true
				if _, ne := w.call(5000000+step, "Page.navigate", map[string]any{"url": u}); ne != nil {
					report.Status = "navigation_error"
					report.Errors = append(report.Errors, ne.Error())
					goto END_RUN
				}
				waitDocSettled(w, 5100000+step)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if reason == "quota" {
				report.Status = "quota"
			} else {
				report.Status = "screenout_or_termination"
			}
			break
		}
		// The survey flagged this test ID as a returning/duplicate session (returning=YES in the URL
		// or an "already completed / duplicate ID" message). Mint a brand new unique ID and start over.
		if boolv(m["returningSession"]) {
			if returningRetries >= maxReturningRetries {
				report.Status = "returning_session"
				report.Errors = append(report.Errors, fmt.Sprintf("Survey kept treating the test ID as returning/duplicate after %d fresh IDs.", returningRetries))
				logLine(logFile, "Still a returning/duplicate session after %d fresh test IDs at step %d; stopping.", returningRetries, step)
				goto END_RUN
			}
			returningRetries++
			report.ReturningRetries = returningRetries
			previousID := persona.ReentryID
			newID, newNumber, idErr := nextReentryID(root, persona)
			if idErr != nil {
				report.Status = "reentry_id_error"
				report.Errors = append(report.Errors, idErr.Error())
				goto END_RUN
			}
			if newID == previousID {
				report.Status = "returning_session"
				report.Errors = append(report.Errors, fmt.Sprintf("Test ID %s was pinned in config/persona.json and the survey rejected it; cannot mint a different ID.", previousID))
				goto END_RUN
			}
			persona.ReentryID = newID
			report.ReentryID = newID
			answeredInAttempt = 0
			logLine(logFile, "Test ID %s rejected at step %d; restarting survey with fresh test ID %s (retry %d of %d, sequence %d).", previousID, step, newID, returningRetries, maxReturningRetries, newNumber)
			report.Errors = append(report.Errors, fmt.Sprintf("step %d: %s was a returning/duplicate session; retried with %s", step, previousID, newID))
			freshSession(w, u, 3990000+step)
			if _, ne := w.call(4000000+step, "Page.navigate", map[string]any{"url": u}); ne != nil {
				report.Status = "navigation_error"
				report.Errors = append(report.Errors, ne.Error())
				goto END_RUN
			}
			waitDocSettled(w, 4100000+step)
			lastSignature = ""
			repeatCount = 0
			time.Sleep(500 * time.Millisecond)
			continue
		}

		sig := str(m["signature"])
		if sig == "" && str(m["url"]) == "" {
			emptySteps++
			waitDocSettled(w, 4600000+step)
			// An empty read is the one thing this runner keeps meeting and cannot explain, so
			// the page itself is looked at rather than reasoned about: written down once per
			// episode, next to the step that found nothing.
			if emptySteps == 2 {
				if v, e := js(w, 4700000+step, "(()=>{try{const b=document.body;return JSON.stringify({u:location.href,t:document.title,bl:b?(b.innerHTML||'').length:-1,ifr:document.querySelectorAll('iframe').length,html:document.documentElement?document.documentElement.outerHTML.length:-1})}catch(e){return JSON.stringify({u:'ERR '+e.message})}})()"); e == nil {
					logLine(logFile, "Empty read at step %d: %s", step, v)
				}
				if v, e := js(w, 4710000+step, "(()=>{try{return document.documentElement.outerHTML.slice(0,6000)}catch(e){return ''}})()"); e == nil && v != "" {
					_ = os.MkdirAll(filepath.Join(root, "reports"), 0755)
					_ = os.WriteFile(filepath.Join(root, "reports", fmt.Sprintf("empty-page-%03d.html", step)), []byte(v), 0644)
				}
			}
			// A survey that is ending an interview replaces the page under the engine, and
			// the replacement is briefly unreadable. That is a page in transition, not a dead
			// one: it is re-read patiently, and only a page that stays unreadable is left
			// behind and started again on a fresh session.
			logLine(logFile, "Step %d: page returned nothing to read (still loading or replaced); retrying (%d of 4).", step, emptySteps)
			// No navigation, no restart, no giving up: the page is re-read until it answers.
			// A survey that ends an interview leaves a page that never will, and the run
			// ends there on the next signal that says so - not by being walked away from.
			if emptySteps >= 4 {
				if v, e := js(w, 4720000+step, "(()=>{try{return JSON.stringify({model:(window.__apModel&&window.__apModel.kind)||'',ready:!!window.__apRendered,q:document.querySelectorAll('input[type=radio],input[type=checkbox],.cf-radio,.cf-checkbox').length,err:(window.__apLastError||'')})}catch(e){return '{}'}})()"); e == nil {
					logLine(logFile, "Page unreadable at step %d (%d steps): %s", step, emptySteps, v)
				}
				if len(v) > 900 {
					v = v[:900] + "..."
				}
				logLine(logFile, "Engine returned at step %d: %s", step, v)
			}
			if emptySteps >= maxUnreadableSteps {
				report.Status = "unreadable_page"
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: the page stayed unreadable for %d inspections; stopped without submitting", step, emptySteps))
				logLine(logFile, "Step %d: page still unreadable after %d inspections; stopping with diagnostics.", step, emptySteps)
				break
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		emptySteps = 0
		// A Next click is still transitioning within the page. This is neither
		// a new unanswered question nor a failed attempt: allow its short lease
		// to expire before counting stalls or re-answering the same controls.
		if strings.Contains(sr.Action, "next-waiting") && toInt(m["unanswered"]) == 0 {
			repeatCount = 0
			samePageSteps = 0
			unansweredStreak = 0
			time.Sleep(140 * time.Millisecond)
			continue
		}
		if strings.Contains(sr.Action, "render-waiting") && toInt(m["unanswered"]) > 0 {
			if renderWaitSince.IsZero() {
				renderWaitSince = time.Now()
			}
			if time.Since(renderWaitSince) >= maxRenderWait {
				report.Status = "needs_review"
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: answer controls did not render within %s", step, maxRenderWait))
				logLine(logFile, "Step %d: question text appeared, but no answer controls mounted; review needed.", step)
				break
			}
			unansweredStreak = 0
			repeatCount = 0
			samePageSteps = 0
			time.Sleep(160 * time.Millisecond)
			continue
		}
		renderWaitSince = time.Time{}
		if accordionAwaiting(sr, toInt(m["unanswered"]), len(stepPairs)) {
			if accordionWaitSince.IsZero() {
				accordionWaitSince = time.Now()
			}
			if strings.Contains(sr.Action, "accordion-needs-review") || time.Since(accordionWaitSince) >= maxAccordionWait {
				report.Status = "needs_review"
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: accordion choices did not render or expand within %s", step, maxAccordionWait))
				logLine(logFile, "Step %d: accordion options still unavailable; review needed.", step)
				break
			}
			// Keep the existing answers. Opening a category is not a failed answer.
			unansweredStreak = 0
			repeatCount = 0
			samePageSteps = 0
			time.Sleep(160 * time.Millisecond)
			continue
		}
		accordionWaitSince = time.Time{}
		if sig != "" && sig == lastSignature {
			samePageSteps++
			repeatCount++
		} else {
			repeatCount = 0
			validationRounds = 0
		}
		if sig != lastSignature {
			samePageSteps = 0
		}
		lastSignature = sig
		if forceUsed {
			// A forced Next is a deliberate action, not a stall: the three forced attempts
			// are the budget, not the repeat counter.
			repeatCount = 0
		}
		stuck := sig != "" && repeatCount > 0
		un := toInt(m["unanswered"])
		modelAnswered := -1
		if sr.Interaction != nil { modelAnswered = toInt(sr.Interaction["answered"]) }
		if confirmedPageProgress(lastUnanswered, un, lastModelAnswered, modelAnswered) {
			samePageSteps = 0
		}
		// The signature is the page text, and a page being worked through item by item
		// keeps exactly the same text while it changes: a matrix answered one row at a
		// time, a brand scale one brand at a time. Answering something, or leaving fewer
		// controls open than the step before, is progress the text cannot show - and
		// reading those as "no visible progress" ended runs that were working.
		if len(stepPairs) > 0 || un < lastUnanswered || strings.Contains(sr.Action, "accordion-expand") {
			repeatCount = 0
		}
		lastUnanswered = un
		sr.Unanswered = un
		// The step was appended to the log before the engine's unanswered count was read
		// back, so the entry the report carries has to be corrected in place.
		if n := len(report.StepsLog); n > 0 {
			report.StepsLog[n-1].Unanswered = un
		}
		// A carousel or a drag list reports the items it still has left, and that number
		// falls for several steps in a row while everything is going fine. Progress on
		// the model is not a page of controls we failed to manipulate, so it must not
		// spend the forced-Next escape hatch.
		if sr.Interaction != nil {
			ma := toInt(sr.Interaction["answered"])
			if lastModelAnswered >= 0 && ma > lastModelAnswered {
				unansweredStreak = 0
			}
			lastModelAnswered = ma
		}
		if un == 0 {
			unansweredStreak = 0
		} else if len(stepPairs) > 0 || strings.Contains(sr.Action, "accordion-expand") {
			// The step still answered something or exposed a hidden question, so the page is being worked through
			// rather than stuck: a grid answered one row at a time or a widget answered
			// one brand at a time reports an unanswered count the whole way. Only steps
			// that produced nothing at all are attempts that failed.
			unansweredStreak = 0
		} else {
			unansweredStreak++
			if unansweredStreak >= 3 {
				// Never submit a page that still has unresolved controls. A validator
				// cannot tell the runner which answers were missed if they are forced.
				kind := str(sr.Interaction["kind"])
				report.Status = "needs_review"
				detail := ""
				if sr.Instructions != nil && toInt(sr.Instructions["blocked"]) > 0 {
					if items := asMaps(sr.Instructions["details"]); len(items) > 0 {
						detail = fmt.Sprintf("; red instruction reason=%s, text=%q", str(items[0]["reason"]), str(items[0]["instruction"]))
					}
				}
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: %d unanswered control(s) after 3 unchanged attempts (model=%s, open=%v%s)", step, un, kind, sr.OpenQuestions, detail))
				logLine(logFile, "Step %d: %d unanswered item(s); stopping for review instead of forcing Next (model=%s, open=%v).", step, un, kind, sr.OpenQuestions)
				break
			}
		}
		// The engine ran out of safe options for a question: drop its bans so it can answer
		// again rather than dead-ending on a question with nothing left to pick.
		if exh := toStrings(m["exhausted"]); len(exh) > 0 && clearCount < 3 {
			for _, qk := range exh {
				if n := mem.clearQuestion(memFP, qk); n > 0 {
					clearCount++
					delete(reopen, qk)
					report.Learned = append(report.Learned, "cleared: "+qk)
					logLine(logFile, "Step %d: question %q exhausted its safe options; cleared %d ban(s) for it.", step, qk, n)
				}
				if clearCount >= 3 {
					break
				}
			}
		}
		// A blocking validation that survives re-checking the page, while everything on the
		// page is answered, can only come from the choices themselves: ban them for this
		// survey and re-answer the questions on the spot.
		blamed := map[string]bool{}
		if len(sr.Validation) > 0 && un == 0 && len(lastPairs) > 0 && stuck {
			logLine(logFile, "Step %d: validation observed; no individual answer was blacklisted.", step)
		}

		if len(stepPairs) > 0 {
			lastPairs = stepPairs
			for _, p := range stepPairs {
				if !blamed[p.Question] {
					delete(reopen, p.Question)
				}
			}
		}
		// A validation message blocks the run but must not end it: after two identical
		// passes over the same page we re-answer everything on it with a different set
		// and only report failure once every attempt has been used up.
		valStuck := len(sr.Validation) > 0 && un == 0
		if repeatCount >= 4 || (valStuck && repeatCount >= 2) {
			if valStuck {
				validationRounds++
				if validationRounds <= maxValidationRounds {
					for _, p := range lastPairs {
						reopen[p.Question] = true
					}
					report.Errors = append(report.Errors, fmt.Sprintf("step %d: validation rejected the answers; re-answering with a different set (attempt %d of %d)", step, validationRounds, maxValidationRounds))
					logLine(logFile, "Step %d: validation rejected the answers; re-answering this page with a different set of answers (attempt %d of %d).", step, validationRounds, maxValidationRounds)
					repeatCount = 0
					time.Sleep(250 * time.Millisecond)
					continue
				}
				// The page has refused everything it was given. The answers it turned down are
				// remembered and the run ends here: the next attempt is a fresh one, and this
				// survey is never navigated off a page that is still being answered.
				noteProgress(step)
				blameEnd(attemptPairs, "validation", maxLearnPerRun-learnCount)
				report.Errors = append(report.Errors, "Survey kept rejecting the answers with a validation message.")
				report.Status = "validation_failure"
				logLine(logFile, "Step %d: validation still rejecting after %d different answer sets; giving up.", step, maxValidationRounds)
				break
			}
			// Don't declare a stall immediately after a navigation action; wait through dynamic rendering first.
			if isNavAction(sr.Action) {
				logLine(logFile, "Step %d repeated signature after navigation; waiting for dynamic render", step)
				waitDocSettled(w, 3000000+step)
				repeatCount = 0
			} else {
				// A page with nothing on it - no text, no control, nothing to answer - is not
				// a survey that is refusing to move: it is a page the browser has not finished
				// rendering, or one the survey has already left. Leaving it and starting again
				// on a fresh session is the answer; calling it a stall ends the run over a page
				// that was never a question.
				if strings.TrimSpace(str(m["bodyPreview"])) == "" {
					if leavePage(step, "the page came back empty") {
						continue
					}
				}
				report.Errors = append(report.Errors, "No visible progress detected after multiple checks.")
				report.Status = "stalled"
				break
			}
		}
		if samePageSteps >= 6 {
			if validationRounds < maxValidationRounds {
				validationRounds++
				for _, p := range lastPairs {
					reopen[p.Question] = true
				}
				logLine(logFile, "Step %d: the page has not moved for %d steps; answering it again with a different set (attempt %d of %d).", step, samePageSteps, validationRounds, maxValidationRounds)
				report.Errors = append(report.Errors, fmt.Sprintf("step %d: the page did not move for %d steps; re-answered with a different set (attempt %d of %d)", step, samePageSteps, validationRounds, maxValidationRounds))
				samePageSteps = 0
				repeatCount = 0
				continue
			}
			// It has been answered four different ways and the page is still exactly the
			// same. Navigating away would be the old reflex; ending the attempt is the
			// honest one, and the batch's next run is a fresh attempt seconds later.
			report.Errors = append(report.Errors, fmt.Sprintf("step %d: the page has not moved for %d steps and four different sets of answers did not change it", step, samePageSteps))
			logLine(logFile, "Step %d: the page has not moved for %d steps after %d different answer sets; ending this attempt here.", step, samePageSteps, validationRounds)
			report.Status = "stalled"
			break
		}
		if isNavAction(sr.Action) {
			// The engine returns the moment Next is clicked, so wait for the document to load
			// here instead of assuming the page has already changed.
			waitDocSettled(w, 4000000+step)
			logLine(logFile, "Step %d navigation settled", step)
		}
	}
END_RUN:
	if report.Status == "running" {
		report.Status = "max_steps_or_manual_intervention"
	}
	// Whatever ended the run, the attempt that got furthest is the profile the next run
	// starts from - including the one that stopped here.
	noteProgress(lastStep)
	// What this run remembers about the survey: a run that ends on a screen-out, a quota
	// or a validation keeps every selection it made so none of them is repeated, while a
	// run that is registered as complete clears the very same selections again - the path
	// that worked must stay open for the next run.
	switch report.Status {
	case "screenout_or_termination":
		blameEnd(runPairs, "screenout", maxEndBlamePerRun)
	case "quota":
		blameEnd(runPairs, "quota", maxEndBlamePerRun)
	case "validation_failure":
		blameEnd(runPairs, "validation", maxEndBlamePerRun)
	case "completed":
		// The answers that reached the end page are stored as the profile that works,
		// and the rules that were closing them off are dropped: the next run replays
		// this profile instead of guessing it again.
		replayable := replayablePairs(attemptPairs)
		_, done := mem.noteAttempt(memFP, replayable, "completed")
		saveErr := mem.save()
		if saveErr != nil {
			report.Errors = append(report.Errors, "could not save completed-run memory: "+saveErr.Error())
			logLine(logFile, "Completed, but the study memory could not be saved: %v", saveErr)
		}
		if done && saveErr == nil {
			report.Learned = append(report.Learned, fmt.Sprintf("remembered %d answer(s) that completed this survey", len(replayable)))
			logLine(logFile, "Completed with %d unambiguous remembered selection(s) for this survey.", len(replayable))
			if n := mem.avoidCount(memFP); n > 0 {
				logLine(logFile, "%d avoid rule(s) still stand for questions this run never answered.", n)
			}
		}
	}
	report.AvoidRules = mem.avoidCount(memFP)
	// Always keep one shot of the final page (completed, screenout, stall, ...) unless
	// this step already produced one or screenshots are disabled.
	if cfg.Screenshots != "none" && len(report.StepsLog) > 0 && report.StepsLog[len(report.StepsLog)-1].Screenshot == "" {
		capture(report.Steps)
	}
	report.FinishedAt = time.Now().Format(time.RFC3339)
	saveReport(root, report)
	logLine(logFile, "Status: %s", report.Status)
	logLine(logFile, "Report: %s", filepath.Join(root, "reports", "run-report.json"))
	return report
}

// shortID renders a CDP target id compactly for log lines.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	if id == "" {
		return "-"
	}
	return id
}

// isolateRespondent clears cookies and site storage so the next complete in the same
// window looks like a brand new respondent to the survey.
func isolateRespondent(w *WS, id int, rawURL string) {
	pu, err := url.Parse(rawURL)
	if err != nil || pu.Scheme == "" || pu.Host == "" {
		return
	}
	_, _ = w.call(id, "Network.enable", nil)
	_, _ = w.call(id+1, "Network.clearBrowserCookies", nil)
	// Only what identifies the respondent: cookies and the two storages. The page cache is
	// deliberately left alone - the survey loads its own scripts from a CDN, and wiping the
	// cache is what left the next page blank and unreadable.
	_, _ = w.call(id+2, "Storage.clearDataForOrigin", map[string]any{
		"origin":       pu.Scheme + "://" + pu.Host,
		"storageTypes": "localstorage,sessionstorage,cookies",
	})
}

// promptCompletes asks how many complete interviews to collect in this session.
func promptCompletes(in *bufio.Reader, logFile *os.File) int {
	for {
		if consoleUI != nil {
			consoleUI.prompt("02   COMPLETION TARGET", "Number of completed QA runs. Press Enter for 1 (maximum 50).")
		} else {
			fmt.Print("How many completes? [1]: ")
		}
		s, err := in.ReadString('\n')
		if err != nil {
			return 1
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return 1
		}
		n, convErr := strconv.Atoi(s)
		if convErr == nil && n >= 1 {
			if n > maxCompletes {
				logLine(logFile, "Requested %d completes; capped at %d.", n, maxCompletes)
				return maxCompletes
			}
			return n
		}
		if consoleUI != nil {
			consoleUI.event("INVALID", amber, fmt.Sprintf("Enter a whole number between 1 and %d.", maxCompletes))
		} else {
			fmt.Printf("Enter a whole number between 1 and %d.\n", maxCompletes)
		}
	}
}

// mergeStepAnswers folds the answers a widget committed only on the re-read after a real
// drag into the step's own record. Without it a brand the survey did accept was missing
// from the report and from the memory, so the next run could not replay it.
func mergeStepAnswers(m map[string]any, pairs []answerPair, labels []string) {
	if len(pairs) > 0 {
		arr, _ := m["selectedPairs"].([]any)
		have := map[string]bool{}
		for _, p := range parsePairs(arr) {
			have[p.Question+"\x00"+p.Option] = true
		}
		for _, p := range pairs {
			if have[p.Question+"\x00"+p.Option] {
				continue
			}
			have[p.Question+"\x00"+p.Option] = true
			arr = append(arr, map[string]any{"question": p.Question, "option": p.Option})
		}
		m["selectedPairs"] = arr
	}
	if len(labels) > 0 {
		arr, _ := m["selected"].([]any)
		have := map[string]bool{}
		for _, v := range toStrings(arr) {
			have[v] = true
		}
		for _, v := range labels {
			if have[v] {
				continue
			}
			have[v] = true
			arr = append(arr, v)
		}
		m["selected"] = arr
	}
}

// widgetModel reports whether a page was read as one widget rather than a list of
// controls - a brand scale, a circle to move, a card board, a carousel. The runner's
// forced Next is for controls it could not click; on a widget page the same state means
// an item of the question itself is still unanswered, and Next must stay held.
func widgetModel(kind string) bool {
	switch kind {
	case "emo_resonance", "scale_drop", "timeline", "carousel_grid":
		return true
	}
	return false
}

// fatalStatus reports failures that make every further run pointless.
func fatalStatus(s string) bool {
	switch s {
	case "browser_missing", "port_error", "browser_error", "reentry_id_error", "needs_human", "needs_review", "unverified_exit", "unreadable_page", "validation_failure", "stalled", "max_steps_or_manual_intervention":
		return true
	}
	return false
}

// A card or row actually completed is progress even when the enclosing page's
// visible text and URL remain unchanged. An unchanged count is not progress.
func confirmedPageProgress(previousOpen, currentOpen, previousAnswered, currentAnswered int) bool {
	return currentOpen < previousOpen || (previousAnswered >= 0 && currentAnswered > previousAnswered)
}

// runReportName gives every run its own file. The batch number is part of the name
// because a report that the next batch overwrites is not evidence of anything: comparing
// what one run chose against the next is the only way to see that the answers differ.
var batchSeq int

func runReportName(runNo, total int) string {
	if total <= 1 {
		return fmt.Sprintf("run-report-%03d.json", batchSeq)
	}
	return fmt.Sprintf("run-b%03d-%03d.json", batchSeq, runNo)
}

func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
}

// removeProfile deletes an isolated browser profile, retrying while Chrome releases its handles.
func removeProfile(path string) {
	for i := 0; i < 8; i++ {
		if err := os.RemoveAll(path); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// cleanupOldScreenshots prunes evidence files past the retention window and removes any
// directories that become empty, so long batch sessions cannot fill the disk indefinitely.
func cleanupOldScreenshots(root string, days int) {
	if days <= 0 {
		return
	}
	dir := filepath.Join(root, "screenshots")
	if _, err := os.Stat(dir); err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var dirs []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
}

// cleanupStaleProfiles drops browser profiles left behind by earlier sessions so batch runs
// do not slowly fill the disk.
func cleanupStaleProfiles(root string) {
	entries, err := os.ReadDir(filepath.Join(root, "data"))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-2 * time.Hour)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "browser-profile-") {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, "data", e.Name()))
	}
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func boolv(v any) bool { b, _ := v.(bool); return b }

// asMaps turns the engine's array of objects into maps, skipping anything else.
func asMaps(v any) []map[string]any {
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(a))
	for _, x := range a {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func toStrings(v any) []string {
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	o := []string{}
	for _, x := range a {
		o = append(o, str(x))
	}
	return o
}
func sameProgress(a, b StepReport) bool {
	return a.URL == b.URL && a.Question == b.Question && a.Action == b.Action && len(a.Validation) == len(b.Validation)
}
func saveReport(root string, r RunReport) {
	saveReportAs(root, "run-report.json", r)
}
func saveReportAs(root, name string, r RunReport) {
	if r.FinishedAt == "" {
		r.FinishedAt = time.Now().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	_ = os.WriteFile(filepath.Join(root, "reports", name), b, 0644)
}
func saveBatchReport(root string, b BatchReport) {
	data, _ := json.MarshalIndent(b, "", "  ")
	_ = os.WriteFile(filepath.Join(root, "reports", "batch-report.json"), data, 0644)
}
