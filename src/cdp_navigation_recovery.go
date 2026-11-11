package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "net"
    "net/url"
    "strings"
    "time"
    "os"
)

// navigationMatchesSurvey is evidence only that a target reached the same survey
// document (or a verified same-origin end page); unrelated tabs cannot satisfy it.
func navigationMatchesSurvey(actual, requested string) bool {
    a, ae := url.Parse(strings.TrimSpace(actual))
    r, re := url.Parse(strings.TrimSpace(requested))
    if ae != nil || re != nil || a.Host == "" || r.Host == "" ||
        !strings.EqualFold(a.Scheme, r.Scheme) || !strings.EqualFold(a.Host, r.Host) {
        return false
    }
    if strings.EqualFold(a.EscapedPath(), r.EscapedPath()) { return true }
    return isQualifiedCompletionForSurvey(actual, requested) ||
        (isTerminatedURL(actual) && leftSurvey(actual, requested))
}

func isNavigationSocketTimeout(err error) bool {
    var ne net.Error
    return errors.As(err, &ne) && ne.Timeout()
}

// reconnectAfterNavigationTimeout repairs a lost Chrome DevTools acknowledgement.
// Page.navigate can time out even after the browser actually opened the page. We
// reconnect to the EXACT tab, verify the live URL, and continue without submitting
// another navigation. If the old tab is still blank, retry navigation once over
// the new socket. Never mistake a different tab or error page for the survey.
func reconnectAfterNavigationTimeout(sess *BrowserSession, original Target, requested string, requestID int, logFile *os.File) (*WS, Target, error) {
    if sess == nil { return nil, original, errors.New("browser session unavailable") }
    if sess.ws != nil {
        _ = sess.ws.c.Close()
        sess.ws = nil
    }
    var lastErr error
    lastURL := ""
    for attempt:=0; attempt<2; attempt++ {
        nt, err := findCurrentTarget(sess.port, "", original.Id)
        if err != nil {
            lastErr = fmt.Errorf("could not find original survey tab: %w", err)
            break
        }
        lastURL = nt.URL
        if nt.Id != original.Id {
            lastErr = errors.New("the original survey tab is no longer available")
            break
        }
        nw, err := wsConnect(nt.WebSocketDebuggerURL)
        if err != nil { lastErr = fmt.Errorf("reconnecting to survey tab: %w", err); continue }
        if navigationMatchesSurvey(nt.URL, requested) {
            // /json/list alone is not enough: Chrome can report a URL while the
            // page's renderer is unusable. Require a live CDP evaluation.
            if value, probeErr := js(nw, requestID+attempt*10+1, "String(location.href)"); probeErr == nil {
                var liveURL string
                if json.Unmarshal([]byte(value), &liveURL) == nil && navigationMatchesSurvey(liveURL, requested) {
                    logLine(logFile, "Navigation acknowledgement timed out, but the original browser tab is at the verified survey URL; CDP reconnected.")
                    sess.ws = nw
                    tc:=nt
                    sess.current=&tc
                    return nw, nt, nil
                }
                lastErr = fmt.Errorf("survey tab URL changed during recovery (target %s)", nt.URL)
            } else {
                lastErr = fmt.Errorf("survey tab did not respond to a read-only probe: %w", probeErr)
            }
        } else {
            lastErr = fmt.Errorf("survey tab is at %q, not the requested study", nt.URL)
        }
        if attempt == 0 {
            // No responses have been entered yet on this run. One retry cannot
            // resubmit a questionnaire; this retry is forbidden after the start.
            logLine(logFile, "Initial navigation not confirmed after CDP timeout; retrying once on the original survey tab.")
            if _, retryErr := nw.call(requestID+1000, "Page.navigate", map[string]any{"url":requested}); retryErr == nil {
                sess.ws = nw
                tc:=nt
                sess.current=&tc
                return nw, nt, nil
            } else {
                lastErr = fmt.Errorf("one navigation retry failed: %w", retryErr)
            }
        }
        _=nw.c.Close()
        // Allow Chrome's target-list URL to catch up to an in-flight navigation.
        time.Sleep(250*time.Millisecond)
    }
    return nil, original, fmt.Errorf("could not recover initial survey navigation (last browser URL %q): %w", lastURL, lastErr)
}
