package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// A survey can reject answers the engine cannot know about in advance: screen-outs,
// quota logic, "you cannot combine A with B", options that are simply not on sale any
// more. Every selection a run makes is logged in its report; a run that ends on a
// screen-out, a quota or a validation keeps all of its selections here, keyed by
// survey, so later runs never repeat them, while a run that is registered as complete
// has the very same selections cleared again - the path that worked must stay open.

const (
	// maxAvoidPerQuestion stops a single question from being banned out of existence.
	maxAvoidPerQuestion = 6
	// maxAvoidPerSurvey caps the injected rule set so the engine expression stays small.
	maxAvoidPerSurvey = 500
	// maxLearnPerRun stops a run from spending all its steps blaming answers.
	maxLearnPerRun = 6
	// maxEndBlamePerRun bounds what one failed run remembers at the end: enough to cover
	// every selection it made, but never more than the survey-wide caps allow.
	maxEndBlamePerRun = 60
	// maxScreenoutRetries bounds how often one run restarts after a screen-out. A screen-out
	// is a question about the profile, not a reason to stop: the run starts again on a fresh
	// session and a fresh test ID, replaying what the store has learned, and only gives up
	// when the whole attempt budget is spent.
	maxScreenoutRetries = 3
	// maxRunsPerComplete is how many runs one requested complete may cost. The batch asks
	// for completes, not attempts, so a run that is screened out is simply another attempt:
	// this is the ceiling that stops a study nothing can pass from running for ever.
	maxRunsPerComplete = 6
	// maxScreenoutPatternStreak is how many screen-outs in a row at the same point, with
	// nothing new left to try, end one run. It stops the search inside a run; the batch
	// then starts another run, so a study that screens everyone out still gets attempt
	// after attempt instead of being written off after one.
	maxScreenoutPatternStreak = 8
	// minRetriesBeforePatternStop is how many times a run is allowed to be rejected and
	// start again before the "same screen over and over" rule may end it. The rule is a
	// stopping condition, never a reason to skip a retry: a run that gave up on the first
	// rejection had not tried the survey again at all.
	minRetriesBeforePatternStop = 3
	// maxPageLeaves is how many unreadable pages one run may walk away from, kept small on
	// purpose: a run that keeps restarting the same interview spends minutes on it, while
	// the batch's next run is a fresh attempt in seconds. Separate from the screen-out
	// budget so one can never starve the other.
	maxPageLeaves = 2
	// goodProtectStreak is how many bad attempts a remembered good answer survives
	// before the store may avoid it. One unlucky run must not undo what worked.
	goodProtectStreak = 2
	// The history is a steering aid, not an archive, so it stays small.
	maxAttemptsKept    = 20
	maxRejectedKept    = 200
	maxGoodPerQuestion = 3
)

type AvoidRule struct {
	Question string `json:"question"`
	Option   string `json:"option"`
	Reason   string `json:"reason"`
	Hits     int    `json:"hits"`
	Added    string `json:"added"`
	LastSeen string `json:"lastSeen"`
}

// KnownGood is one answer that has taken this survey to a real completion. It is
// replayed before anything else on the next run, so a survey is answered the way it
// was answered successfully instead of being guessed at again.
type KnownGood struct {
	Option string `json:"option"`
	// Best is the answer that either completed this survey or carried an attempt
	// further into it than any other. It is the fail-safe: the answer is replayed for
	// every later run and, unlike a merely remembered option, no later rejection is
	// allowed to rule it out - a profile that qualified once is how every later run
	// gets its way back in.
	Best string `json:"best,omitempty"`
	Wins int    `json:"wins"`
	Uses int    `json:"uses"`
	// Far counts the attempts this answer carried deeper into the survey than any
	// before it. A survey that hides its rule is searched one answer at a time, so an
	// answer that has demonstrably let a run continue is the next run's starting point
	// long before anything has completed.
	Far int `json:"far,omitempty"`
	// FailStreak counts consecutive completions-that-did-not-happen while this answer
	// was part of the attempt. A remembered good answer is never avoided while its
	// streak is below goodProtectStreak, otherwise one bad quota fills up with its own
	// mistakes and the next run repeats them.
	FailStreak int    `json:"failStreak,omitempty"`
	LastUsed   string `json:"lastUsed,omitempty"`
}

// AttemptRecord is one set of selections and how it ended. It is what makes the store
// a history rather than a blacklist: the same profile is never submitted twice.
type AttemptRecord struct {
	At     string       `json:"at"`
	Status string       `json:"status"`
	Hash   string       `json:"hash"`
	Pairs  []answerPair `json:"pairs,omitempty"`
}

// AnswerStat is the record of one answer to one question: how often it was submitted
// and how it turned out. It is what turns the store from a blacklist into something
// that works out which answer carries the disqualification.
type AnswerStat struct {
	Tries int `json:"tries"`
	Good  int `json:"good"`
	Bad   int `json:"bad"`
}

func (a *AnswerStat) score() int { return a.Good - a.Bad }

// ProbeState is the question the tool is currently varying. A survey that keeps
// screening out is answering a hidden rule over the whole profile, so the fast way
// through it is to replay the last profile exactly and change one answer at a time
// until something else is learned.
type ProbeState struct {
	Question string         `json:"question"`
	Tried    []string       `json:"tried,omitempty"`
	Since    int            `json:"since"`
	Varied   map[string]int `json:"varied,omitempty"`
}

// ScreenoutState is the "nothing new is happening" detector. A survey that keeps
// screening out at the same step with the same profile is not going to yield to more
// retries, so the streak is counted here and the run stops instead of burning minutes.
type ScreenoutState struct {
	Step      int    `json:"step"`
	Signature string `json:"signature,omitempty"`
	Streak    int    `json:"streak"`
	Learned   bool   `json:"learned,omitempty"`
	LastAt    string `json:"lastAt,omitempty"`
}

type SurveyMemory struct {
	URL       string                `json:"url"`
	Key       string                `json:"key"`
	Updated   string                `json:"updated"`
	Completed int                   `json:"completed,omitempty"`
	Failed    int                   `json:"failed,omitempty"`
	Good      map[string]*KnownGood `json:"good,omitempty"`
	Avoid     []AvoidRule           `json:"avoid"`
	// Rejected holds the fingerprints of the answer sets that ended a run badly, so
	// the engine can steer off a profile it has already tried instead of rebuilding it.
	Rejected []string        `json:"rejectedCombos,omitempty"`
	Attempts []AttemptRecord `json:"attempts,omitempty"`
	// BlameTurn counts how many times a rejection has been read off this survey. The
	// answers closest to the end are the first suspects, but the disqualifier is not
	// necessarily among them: each rejection moves the window one step further back
	// through the profile, so the whole profile is searched instead of its tail.
	BlameTurn int `json:"blameTurn,omitempty"`
	// Deepest is how far into the survey any attempt has ever got. It is the only honest
	// measure of progress on a study that screens people out rather than completing
	// them: the run that reached furthest is the profile worth replaying.
	Deepest int                               `json:"deepest,omitempty"`
	Stats   map[string]map[string]*AnswerStat `json:"stats,omitempty"`
	Probe   *ProbeState                       `json:"probe,omitempty"`
	Screen  *ScreenoutState                   `json:"screenout,omitempty"`
}

// answerPair is one selection the engine made: the question it belongs to and the option.
type answerPair struct {
	Question string `json:"question"`
	Option   string `json:"option"`
}

type MemoryStore struct {
	mu      sync.Mutex
	path    string
	Surveys map[string]*SurveyMemory `json:"surveys"`
	readFailed bool
}

func openMemory(root string) *MemoryStore {
	s := &MemoryStore{path: filepath.Join(root, "data", "survey-success-memory.json"), Surveys: map[string]*SurveyMemory{}}
	if b, err := os.ReadFile(s.path); err == nil {
		var raw struct { Surveys map[string]*SurveyMemory `json:"surveys"` }
		if err := json.Unmarshal(b, &raw); err != nil { s.readFailed = true
		} else if raw.Surveys != nil { s.Surveys = raw.Surveys }
	} else if !os.IsNotExist(err) { s.readFailed = true }
	return s
}

// Save only confirmed successes. A corrupt/unreadable prior file is never overwritten.
func (s *MemoryStore) save() error {
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.readFailed { return errors.New("existing survey memory could not be read; not overwriting it") }
 clean := map[string]*SurveyMemory{}
 for key, m := range s.Surveys {
  if m == nil { continue }
  good := map[string]*KnownGood{}
  for q, g := range m.Good {
   if g != nil && g.Wins > 0 && g.Best != "" && !strings.HasPrefix(q, "text|") {
    good[q] = &KnownGood{Option:g.Best, Best:g.Best, Wins:g.Wins}
   }
  }
  avoid := []AvoidRule{}
  for _, rule := range m.Avoid { if rule.Reason == "screenout-suspect" { avoid = append(avoid, rule) } }
  if m.Completed < 1 && len(avoid) == 0 { continue }
  clean[key] = &SurveyMemory{Key:key, Updated:m.Updated, Completed:m.Completed, Good:good, Avoid:avoid}
 }
 if len(clean)==0 { return nil }
 b, err := json.MarshalIndent(struct{Surveys map[string]*SurveyMemory `json:"surveys"`}{clean}, "", "  ")
 if err != nil { return err }
 if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil { return err }
 f, err := os.CreateTemp(filepath.Dir(s.path), ".survey-memory-*")
 if err != nil { return err }
 name := f.Name()
 defer os.Remove(name)
 if _, err = f.Write(b); err != nil { f.Close(); return err }
 if err = f.Sync(); err != nil { f.Close(); return err }
 if err = f.Close(); err != nil { return err }
 return os.Rename(name, s.path)
}

// memoryFingerprint is intentionally conservative: two test links must not share
// answer history merely because a survey platform reuses its /x.aspx endpoint.
// Only demonstrably session-local fields are ignored. In particular __etk and
// unrecognised query parameters are retained, even if that means less reuse.
func memoryFingerprint(raw string) string {
	p, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || p.Host == "" {
		digest := sha256.Sum256([]byte(strings.TrimSpace(raw)))
		return "sha256:" + hex.EncodeToString(digest[:])
	}
	q := p.Query()
	for k := range q {
		switch strings.ToLower(k) {
		case "session", "sessionid", "returning", "cachebuster", "_", "timestamp":
			q.Del(k)
		}
	}
	key := strings.ToLower(p.Scheme) + "://" + strings.ToLower(p.Host) + p.EscapedPath()
	if encoded := q.Encode(); encoded != "" {
		key += "?" + encoded
	}
	digest := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s *MemoryStore) forURL(u string) *SurveyMemory {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := memoryFingerprint(u)
	m := s.Surveys[k]
	if m == nil {
		m = &SurveyMemory{Key: k, Updated: time.Now().Format(time.RFC3339), Avoid: []AvoidRule{}}
		s.Surveys[k] = m
	}
	return m
}

func (s *MemoryStore) avoidCount(fp string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.Surveys[fp]; m != nil {
		return len(m.Avoid)
	}
	return 0
}

// avoidFor returns a copy of termination suspects for this survey only.
func (s *MemoryStore) avoidFor(fp string) []AvoidRule {
 s.mu.Lock()
 defer s.mu.Unlock()
 m := s.Surveys[fp]
 if m == nil { return []AvoidRule{} }
 out := []AvoidRule{}
 for _, r := range m.Avoid { if r.Reason == "screenout-suspect" { out = append(out,r) } }
 return out
}

// A screenout identifies the last unambiguous submitted choice as a suspect,
// not a proven cause. Quotas and ambiguous pages never create a rule.
func (s *MemoryStore) recordScreenoutSuspect(fp string, pairs []answerPair, reason string) (bool, error) {
 if reason != "screenout" || len(pairs) != 1 { return false,nil }
 p := pairs[0]
 if strings.HasPrefix(strings.ToLower(p.Question), "text|") { return false,nil }
 ok,_ := s.addAvoid(fp,p.Question,p.Option,"screenout-suspect")
 if !ok { return false,nil }
 return true,s.save()
}

// trimRule guards the rule key without ever rewriting it: the engine matches on the exact
// string it produced, so collapsing whitespace or slicing UTF-8 here would break lookups.
func trimRule(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// addAvoid records that an option of a question is unsafe for this survey.
// ok=false means the rule was refused (empty, or the question is already fully banned).
// isNew tells the caller to log it as a fresh finding.
func (s *MemoryStore) addAvoid(fp, question, option, reason string) (ok, isNew bool) {
	// The engine already caps both keys; these bounds are a last-ditch guard only, set far
	// above the engine's caps so Go never re-slices a string the engine will re-compare.
	question = trimRule(question, 1024)
	option = trimRule(option, 1024)
	if question == "" || option == "" {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		m = &SurveyMemory{Key: fp, Updated: time.Now().Format(time.RFC3339)}
		s.Surveys[fp] = m
	}
	now := time.Now().Format(time.RFC3339)
	sameQuestion := 0
	for i := range m.Avoid {
		r := &m.Avoid[i]
		if !strings.EqualFold(r.Question, question) {
			continue
		}
		sameQuestion++
		if strings.EqualFold(r.Option, option) {
			r.Hits++
			r.LastSeen = now
			if r.Reason != reason && reason != "" {
				r.Reason = reason
			}
			m.Updated = now
			return true, false
		}
	}
	if sameQuestion >= maxAvoidPerQuestion || len(m.Avoid) >= maxAvoidPerSurvey {
		return false, false
	}
	m.Avoid = append(m.Avoid, AvoidRule{Question: question, Option: option, Reason: reason, Hits: 1, Added: now, LastSeen: now})
	m.Updated = now
	return true, true
}

// terminalReason reports whether a rule was learned because a run ended for good. A
// screen-out or a full quota is a property of the survey, not of one attempt, so those
// selections have to survive every cleanup and stay avoided in the next response.
func terminalReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "screenout", "quota", "termination", "screenout-suspect":
		return true
	}
	return false
}

// clearQuestion drops every rule for one question. The engine asks for this when it has
// exhausted the options of a question, so a survey can never be banned into a dead end -
// except for terminal rules, which are kept no matter what.
func (s *MemoryStore) clearQuestion(fp, question string) int {
	question = trimRule(question, 1024)
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil || question == "" {
		return 0
	}
	kept := m.Avoid[:0]
	removed := 0
	for _, r := range m.Avoid {
		if strings.EqualFold(r.Question, question) && !terminalReason(r.Reason) {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	m.Avoid = kept
	if removed > 0 {
		m.Updated = time.Now().Format(time.RFC3339)
	}
	return removed
}

// clearPairs forgets the rules that match a completed run's selections. Those answers
// led to a real completion, so they have to stay available instead of being remembered
// as something to avoid; rules this run never touched are left alone.
func (s *MemoryStore) clearPairs(fp string, pairs []answerPair) int {
	if len(pairs) == 0 {
		return 0
	}
	want := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		if p.Question == "" || p.Option == "" {
			continue
		}
		want[strings.ToLower(p.Question)+"||"+strings.ToLower(p.Option)] = true
	}
	if len(want) == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		return 0
	}
	kept := m.Avoid[:0]
	removed := 0
	for _, r := range m.Avoid {
		if want[strings.ToLower(r.Question)+"||"+strings.ToLower(r.Option)] {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	m.Avoid = kept
	if removed > 0 {
		m.Updated = time.Now().Format(time.RFC3339)
	}
	return removed
}

func (s *MemoryStore) noteCompleted(fp string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.Surveys[fp]; m != nil {
		m.Completed++
		m.Updated = time.Now().Format(time.RFC3339)
	}
}

// comboFingerprint identifies one set of selections. It is order independent: the same
// answers submitted in a different order are the same profile.
func comboFingerprint(pairs []answerPair) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p.Question == "" || p.Option == "" {
			continue
		}
		parts = append(parts, strings.ToLower(p.Question)+"="+strings.ToLower(p.Option))
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts)
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// comboTrail spells one set of selections out in the order it was answered. It is what
// the engine checks against while it answers: a profile that is being rebuilt is caught
// at the question where it diverges from the one that already failed.
func comboTrail(pairs []answerPair) string {
	parts := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		q := strings.ToLower(strings.TrimSpace(p.Question))
		o := strings.ToLower(strings.TrimSpace(p.Option))
		if q == "" || o == "" {
			continue
		}
		parts = append(parts, q, o)
	}
	return strings.Join(parts, "\x00")
}

// Only remember unambiguous single selections. Multi-answer sets and changed
// answers require a verified page snapshot before they can be replayed safely.
func replayablePairs(pairs []answerPair) []answerPair {
 options:=map[string]map[string]bool{}
 for _, p:=range pairs {
  q:=strings.ToLower(strings.TrimSpace(p.Question)); v:=strings.TrimSpace(p.Option)
  if q=="" || v=="" || strings.HasPrefix(q,"text|") {continue}
  if options[q]==nil {options[q]=map[string]bool{}}
  options[q][v]=true
 }
 out:=[]answerPair{}
 for q, values:=range options {if len(values)==1 {for v:=range values {out=append(out,answerPair{Question:q,Option:v})}}}
 return out
}

// noteAttempt is the one place the store learns from a finished attempt. A completed
// set of selections is remembered as the profile that worked and is replayed next
// time; a set that ended the run badly is remembered as a profile never to rebuild,
// and its answers are avoided within the caller's budget.
func (s *MemoryStore) noteAttempt(fp string, pairs []answerPair, status string) (learned []AvoidRule, completed bool) {
	if fp == "" || len(pairs) == 0 {
		return nil, false
	}
	now := time.Now().Format(time.RFC3339)
	hash := comboFingerprint(pairs)
	trail := comboTrail(pairs)
	completed = strings.EqualFold(strings.TrimSpace(status), "completed")

	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		m = &SurveyMemory{Key: fp, Updated: now}
		s.Surveys[fp] = m
	}
	if m.Good == nil {
		m.Good = map[string]*KnownGood{}
	}
	m.Updated = now

	// The history: what was tried, and how it ended.
	if hash != "" {
		m.Attempts = append(m.Attempts, AttemptRecord{At: now, Status: status, Hash: hash, Pairs: pairs})
		if len(m.Attempts) > maxAttemptsKept {
			m.Attempts = m.Attempts[len(m.Attempts)-maxAttemptsKept:]
		}
	}

	for _, p := range pairs {
		if p.Question == "" || p.Option == "" {
			continue
		}
		key := strings.ToLower(p.Question)
		opt := strings.ToLower(p.Option)
		if m.Stats == nil {
			m.Stats = map[string]map[string]*AnswerStat{}
		}
		if m.Stats[key] == nil {
			m.Stats[key] = map[string]*AnswerStat{}
		}
		st := m.Stats[key][opt]
		if st == nil {
			st = &AnswerStat{}
			m.Stats[key][opt] = st
		}
		st.Tries++
		if completed {
			st.Good++
		} else if terminalReason(status) {
			st.Bad++
		}
		g := m.Good[key]
		if g == nil {
			g = &KnownGood{Option: p.Option}
			m.Good[key] = g
		}
		g.Uses++
		g.LastUsed = now
		if completed {
			// This answer is part of a run that reached the end page.
			g.Wins++
			g.Best = p.Option
			g.FailStreak = 0
			if !strings.EqualFold(g.Option, p.Option) {
				g.Option = p.Option
			}
		} else if terminalReason(status) {
			g.FailStreak++
		}
		if g.Option == "" {
			g.Option = p.Option
		}
	}

	if completed {
		m.Completed++
		// The winning profile has to stay selectable: whatever the store learned while
		// the run was still in progress must not close the path that just worked.
		for _, p := range pairs {
			if p.Question == "" || p.Option == "" {
				continue
			}
			want := strings.ToLower(p.Question) + "||" + strings.ToLower(p.Option)
			kept := m.Avoid[:0]
			for _, r := range m.Avoid {
				if strings.ToLower(r.Question)+"||"+strings.ToLower(r.Option) == want {
					continue
				}
				kept = append(kept, r)
			}
			m.Avoid = kept
		}
		// A profile that just worked is not a rejected one.
		m.Rejected = dropString(m.Rejected, trail)
		s.pruneGood(m)
		return nil, true
	}

	m.Failed++
	if trail != "" && terminalReason(status) {
		m.Rejected = append(m.Rejected, trail)
		if len(m.Rejected) > maxRejectedKept {
			m.Rejected = m.Rejected[len(m.Rejected)-maxRejectedKept:]
		}
	}
	return nil, false
}

// nextBlameTurn counts one more rejection read off this survey and returns which window
// through the profile it should be read from. The window starts at the end, where the
// answers nearest the rejection are, and walks backwards one window at a time.
func (s *MemoryStore) nextBlameTurn(fp string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		return 0
	}
	turn := m.BlameTurn
	m.BlameTurn++
	return turn
}

// noteFurthest records that an attempt carried this profile deeper into the survey than
// any attempt before it, and returns true when it did. Every answer in that attempt is
// then the profile the next run starts from: on a study that screens people out, "this
// got us past that question" is the knowledge worth keeping, and it is worth keeping for
// one question at a time rather than only at the end page.
func (s *MemoryStore) noteFurthest(fp string, pairs []answerPair, depth int) bool {
	if fp == "" || len(pairs) == 0 || depth <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		m = &SurveyMemory{Key: fp, Updated: time.Now().Format(time.RFC3339)}
		s.Surveys[fp] = m
	}
	// A profile that gets exactly as far as the best one ever has is as good a starting
	// point as the one that got there first, and it is the only way the search moves:
	// refusing ties left the memory with nothing to replay but a list of what not to
	// answer, which is a search that cannot start.
	if depth < m.Deepest {
		return false
	}
	m.Deepest = depth
	// Getting further is the opposite of being stuck at the same end page, so the memory
	// of that screen starts again: otherwise a single bad patch of screen-outs keeps
	// ending runs that are now getting through.
	if m.Screen != nil {
		m.Screen = nil
	}
	m.Updated = time.Now().Format(time.RFC3339)
	if m.Good == nil {
		m.Good = map[string]*KnownGood{}
	}
	for _, p := range pairs {
		if p.Question == "" || p.Option == "" {
			continue
		}
		key := strings.ToLower(p.Question)
		g := m.Good[key]
		if g == nil {
			g = &KnownGood{Option: p.Option}
			m.Good[key] = g
		}
		g.Far++
		g.Option = p.Option
		g.Best = p.Option
		g.LastUsed = m.Updated
	}
	return true
}

// noteFurthest reports how far the furthest attempt on this survey got, which is what a
// new attempt has to beat for its answers to be worth remembering.
func (s *MemoryStore) furthest(fp string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.Surveys[fp]; m != nil {
		return m.Deepest
	}
	return 0
}

// rememberAvoid adds an avoid rule unless it would close off an answer that has taken
// this survey to a completion. Returns the rule that was actually written.
func (s *MemoryStore) rememberAvoid(fp, question, option, reason string) (AvoidRule, bool) {
	question = trimRule(question, 1024)
	option = trimRule(option, 1024)
	if question == "" || option == "" {
		return AvoidRule{}, false
	}
	s.mu.Lock()
	g := (*KnownGood)(nil)
	if m := s.Surveys[fp]; m != nil && m.Good != nil {
		g = m.Good[strings.ToLower(question)]
	}
	s.mu.Unlock()
	if g != nil {
		// The answer that qualified is the way back into this survey, so it is never
		// ruled out - not even after a run in which it was part of a profile that got
		// rejected. Ruling it out is what lost a profile that had already got a
		// respondent most of the way through.
		if best := g.Best; best != "" && strings.EqualFold(best, option) {
			return AvoidRule{}, false
		}
		if strings.EqualFold(g.Option, option) && g.FailStreak < goodProtectStreak {
			return AvoidRule{}, false
		}
	}
	ok, _ := s.addAvoid(fp, question, option, reason)
	if !ok {
		return AvoidRule{}, false
	}
	return AvoidRule{Question: question, Option: option, Reason: reason}, true
}

// preferFor is the map the engine reads before it answers anything: question to the
// option that has already completed this survey.
func (s *MemoryStore) preferFor(fp string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	m := s.Surveys[fp]
	if m == nil {
		return out
	}
	for q, g := range m.Good {
		if g == nil {
			continue
		}
		if g.Wins > 0 && g.Best != "" {
			out[q] = g.Best
		}
	}
	return out
}

// rejectedCombos is the list of answer sets that ended a run badly.
func (s *MemoryStore) rejectedCombos(fp string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil || len(m.Rejected) == 0 {
		return nil
	}
	return append([]string(nil), m.Rejected...)
}

// nextProbe decides what to change on the next attempt. Every question from the last
// failed attempt is a suspect; the one whose answers have the worst record and still
// has an answer nobody has tried becomes the next variable. Replaying the rest of the
// profile unchanged is what makes this a search instead of a guess.
func (s *MemoryStore) nextProbe(fp string, lastFailed []answerPair) (question string, exclude string, pin map[string]string) {
	pin = map[string]string{}
	for _, p := range lastFailed {
		if p.Question == "" || p.Option == "" {
			continue
		}
		// The brands, the grids, the pictures and the sliders are not part of the profile
		// and are never pinned: a run that replays them unchanged is the same respondent
		// every time, which is the one thing a panel of ten must not be.
		if contentAnswer(p.Question) {
			continue
		}
		pin[strings.ToLower(p.Question)] = p.Option
	}
	if len(lastFailed) == 0 {
		return "", "", pin
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil || m.Stats == nil {
		return "", "", pin
	}
	type cand struct {
		q     string
		bad   float64
		times int
	}
	var best *cand
	for _, p := range lastFailed {
		q := strings.ToLower(p.Question)
		// Varying a brand or a grid cell cannot get past a screener: the suspect has to be
		// a question the screener actually asked.
		if contentAnswer(p.Question) {
			continue
		}
		o := strings.ToLower(p.Option)
		stats := m.Stats[q]
		if stats == nil {
			continue
		}
		st := stats[o]
		if st == nil || st.Tries == 0 {
			continue
		}
		times := 0
		if m.Probe != nil && m.Probe.Varied != nil {
			times = m.Probe.Varied[q]
		}
		// Breadth first: cover every suspect once before trying a second answer for
		// any of them, and prefer the answers with the worst record. Which answer is
		// actually available is the engine's business - it knows the whole option list.
		weight := float64(times)*10 - float64(st.Bad)/float64(st.Tries)
		if best == nil || weight < best.bad {
			best = &cand{q: q, bad: weight, times: times}
		}
	}
	if best == nil {
		return "", "", pin
	}
	if m.Probe == nil {
		m.Probe = &ProbeState{}
	}
	if m.Probe.Question != best.q {
		m.Probe.Question = best.q
		m.Probe.Since = 0
	}
	if m.Probe.Varied == nil {
		m.Probe.Varied = map[string]int{}
	}
	m.Probe.Varied[best.q]++
	m.Probe.Since++
	exclude = lastOptionFor(m, best.q, pin)
	m.Probe.Tried = appendBounded(m.Probe.Tried, exclude, maxAvoidPerQuestion)
	m.Updated = time.Now().Format(time.RFC3339)
	return best.q, exclude, pin
}

// lastOptionFor is the answer the failed attempt used for this question.
func lastOptionFor(m *SurveyMemory, q string, pin map[string]string) string {
	if o, ok := pin[q]; ok {
		return o
	}
	if n := len(m.Attempts); n > 0 {
		for i := len(m.Attempts[n-1].Pairs) - 1; i >= 0; i-- {
			if strings.EqualFold(m.Attempts[n-1].Pairs[i].Question, q) {
				return strings.ToLower(m.Attempts[n-1].Pairs[i].Option)
			}
		}
	}
	return ""
}

func appendBounded(list []string, v string, max int) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	list = append(list, v)
	if len(list) > max {
		list = list[len(list)-max:]
	}
	return list
}

// scoreFor is handed to the engine: every answer it knows, with how well it went.
func (s *MemoryStore) scoreFor(fp string) map[string]map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]map[string]int{}
	m := s.Surveys[fp]
	if m == nil || m.Stats == nil {
		return out
	}
	for q, opts := range m.Stats {
		row := map[string]int{}
		for o, st := range opts {
			if st != nil {
				row[o] = st.score()
			}
		}
		out[q] = row
	}
	return out
}

// lastFailedPairs is the profile of the most recent attempt that ended badly. A new run
// starts by replaying it, so the search continues across runs instead of restarting.
func (s *MemoryStore) lastFailedPairs(fp string) []answerPair {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		return nil
	}
	for i := len(m.Attempts) - 1; i >= 0; i-- {
		if terminalReason(m.Attempts[i].Status) && len(m.Attempts[i].Pairs) > 0 {
			return m.Attempts[i].Pairs
		}
	}
	return nil
}

// noteScreenout records where a run ended and whether the retry after it could
// possibly have learned anything new. It returns the streak of identical outcomes.
func (s *MemoryStore) noteScreenout(fp string, step int, signature string, learned bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil {
		m = &SurveyMemory{Key: fp, Updated: time.Now().Format(time.RFC3339)}
		s.Surveys[fp] = m
	}
	if m.Screen == nil {
		m.Screen = &ScreenoutState{}
	}
	// The end page has to repeat, not the step count: one run screens out several times, so
	// the step number moves while the outcome stays exactly the same.
	same := m.Screen.Signature == signature && !learned && signature != ""
	if same {
		m.Screen.Streak++
	} else {
		m.Screen = &ScreenoutState{Step: step, Signature: signature, Streak: 1, Learned: learned}
	}
	m.Screen.LastAt = time.Now().Format(time.RFC3339)
	m.Updated = m.Screen.LastAt
	return m.Screen.Streak
}

// probeCanLearn reports whether the question being varied still has an answer the store
// has never submitted. Once every answer it knows about has been tried, another retry is
// only a different roll of the dice.
func (s *MemoryStore) probeCanLearn(fp, question string) bool {
	if question == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil || m.Stats == nil {
		return true
	}
	q := strings.ToLower(question)
	distinct := len(m.Stats[q])
	varied := 0
	if m.Probe != nil && m.Probe.Varied != nil {
		varied = m.Probe.Varied[q]
	}
	return varied < distinct
}

// screenoutStreak is how many screen-outs in a row taught the store nothing.
func (s *MemoryStore) screenoutStreak(fp string) (int, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.Surveys[fp]
	if m == nil || m.Screen == nil {
		return 0, 0, ""
	}
	return m.Screen.Streak, m.Screen.Step, m.Screen.Signature
}

// pruneGood keeps only the answers that carry weight: ones that completed a run, plus
// a couple of runners-up so the engine can still steer when the winner goes bad.
func (s *MemoryStore) pruneGood(m *SurveyMemory) {
	if m == nil || len(m.Good) == 0 {
		return
	}
	for q, g := range m.Good {
		if g == nil || (g.Wins == 0 && g.Uses <= 1) {
			delete(m.Good, q)
		}
	}
}

// dropString removes the first matching entry.
func dropString(list []string, want string) []string {
	if want == "" {
		return list
	}
	for i, v := range list {
		if v == want {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}

// blameValidation picks the selections a blocking validation message can be pinned on.
// A message that names one of the chosen options ("Offline mode is not available") blames
// only those; a message that is really about missing answers blames nothing; anything else
// falls back to the whole selection set of the step.
func blameValidation(messages []string, pairs []answerPair) []answerPair {
	if len(pairs) == 0 {
		return nil
	}
	// Validation text is re-spaced and lower-cased so an option label still matches when the
	// survey wraps it across lines.
	joined := strings.Join(strings.Fields(strings.ToLower(strings.Join(messages, " "))), " ")
	if joined == "" {
		return nil
	}
	if looksUnanswered(joined) {
		return nil
	}
	var named []answerPair
	for _, p := range pairs {
		o := strings.ToLower(strings.TrimSpace(p.Option))
		if len(o) >= 4 && strings.Contains(joined, o) {
			named = append(named, p)
		}
	}
	if len(named) > 0 {
		return named
	}
	return pairs
}

// looksUnanswered reports validation text that is about missing answers rather than a
// rejected choice: blaming an answer for those would poison the rule set.
func looksUnanswered(lowerJoined string) bool {
	for _, re := range []string{
		"please select", "please choose", "please answer", "please enter", "please provide",
		"is required", "are required", "must be selected", "must select", "must be answered",
		"required field", "cannot be blank", "cannot be empty", "is mandatory", "answer this question",
		"you have not answered", "select at least one", "choose at least one", "enter a value",
	} {
		if strings.Contains(lowerJoined, re) {
			return true
		}
	}
	return false
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, e := n.Int64(); e == nil {
			return int(i)
		}
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		if f, e := n.Float64(); e == nil {
			return f
		}
	}
	return 0
}

func parsePairs(v any) []answerPair {
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []answerPair{}
	for _, x := range a {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		q, o := str(m["question"]), str(m["option"])
		if q != "" && o != "" {
			out = append(out, answerPair{Question: q, Option: o})
		}
	}
	return out
}

// pairsKey is an order independent fingerprint of a selection set, used to tell "answered
// something new" apart from "answered the same rejected thing again".
func pairsKey(ps []answerPair) string {
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, p.Question+"="+p.Option)
	}
	sort.Strings(parts)
	return strings.Join(parts, "&")
}
