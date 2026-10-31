package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBlameValidationPinsOnlyTheNamedOption(t *testing.T) {
	pairs := []answerPair{
		{Question: "city question", Option: "london"},
		{Question: "features question", Option: "offline mode"},
		{Question: "frequency question", Option: "daily"},
	}
	got := blameValidation([]string{"Offline mode is not available for this study."}, pairs)
	if len(got) != 1 || got[0].Option != "offline mode" {
		t.Fatalf("expected only the offline mode pair, got %#v", got)
	}
}

func TestBlameValidationIgnoresMissingAnswerMessages(t *testing.T) {
	pairs := []answerPair{{Question: "city question", Option: "london"}}
	for _, msg := range []string{
		"Please select at least one option.",
		"This field is required.",
		"Please enter an answer for question 3.",
		"You must select an answer.",
	} {
		if got := blameValidation([]string{msg}, pairs); got != nil {
			t.Errorf("%q should not blame an answer, got %#v", msg, got)
		}
	}
}

func TestBlameValidationFallsBackToEveryChoice(t *testing.T) {
	pairs := []answerPair{
		{Question: "city question", Option: "london"},
		{Question: "frequency question", Option: "daily"},
	}
	got := blameValidation([]string{"Your answers could not be accepted."}, pairs)
	if len(got) != 2 {
		t.Fatalf("expected the whole selection set, got %#v", got)
	}
}

func TestBlameValidationSurvivesLineWraps(t *testing.T) {
	pairs := []answerPair{{Question: "features question", Option: "offline mode"}}
	got := blameValidation([]string{"Offline\r\n  mode is not available."}, pairs)
	if len(got) != 1 {
		t.Fatalf("wrapped message should still match, got %#v", got)
	}
}

func TestAddAvoidCapsThenClearsQuestion(t *testing.T) {
	dir := t.TempDir()
	s := openMemory(dir)
	fp := "example.com/survey"

	for i := 0; i < maxAvoidPerQuestion+3; i++ {
		ok, _ := s.addAvoid(fp, "q1", "option "+string(rune('a'+i)), "validation")
		if i < maxAvoidPerQuestion && !ok {
			t.Fatalf("rule %d should have been stored", i)
		}
		if i >= maxAvoidPerQuestion && ok {
			t.Fatalf("rule %d should have been refused by the per question cap", i)
		}
	}
	if n := s.avoidCount(fp); n != maxAvoidPerQuestion {
		t.Fatalf("expected %d rules, got %d", maxAvoidPerQuestion, n)
	}
	ok, isNew := s.addAvoid(fp, "q1", "option a", "validation")
	if !ok || isNew {
		t.Fatalf("re-adding a known rule should bump its hits, got ok=%v isNew=%v", ok, isNew)
	}
	if removed := s.clearQuestion(fp, "q1"); removed != maxAvoidPerQuestion {
		t.Fatalf("expected %d rules cleared, got %d", maxAvoidPerQuestion, removed)
	}
	if n := s.avoidCount(fp); n != 0 {
		t.Fatalf("expected an empty rule set, got %d", n)
	}
}

func TestMemoryPersistsBetweenRuns(t *testing.T) {
 dir := t.TempDir()
 first := openMemory(dir)
 u := "http://example.com/s?token=private"
 fp := memoryFingerprint(u)
 first.forURL(u)
 first.addAvoid(fp, "q", "bad option", "screenout")
 first.noteAttempt(fp, []answerPair{{Question:"q", Option:"good option"}, {Question:"text|id", Option:"private-id"}}, "completed")
 first.save()
 second := openMemory(dir)
 if got:=second.preferFor(fp)["q"]; got!="good option" { t.Fatalf("completed option not restored: %q", got) }
 if got:=second.avoidCount(fp); got!=0 { t.Fatalf("inferred avoid rules must not persist: %d", got) }
 b, err:=os.ReadFile(second.path); if err!=nil { t.Fatal(err) }
 if strings.Contains(string(b), "private-id") || strings.Contains(string(b), "token=private") { t.Fatal("memory retained private input or raw URL") }
}

func TestStudyMemoryUsesPIDFilesAndMigratesMatchingLegacy(t *testing.T) {
 root := t.TempDir()
 a := "https://survey.example.test/x?study=alpha&sname=p123456789"
 b := "https://survey.example.test/x?study=beta&sname=p123456789"
 fa, fb := memoryFingerprint(a), memoryFingerprint(b)
 legacy := openMemory(root)
 legacy.addAvoid(fa, "first", "a", "screenout-suspect")
 legacy.addAvoid(fb, "second", "b", "screenout-suspect")
 if err := legacy.save(); err != nil { t.Fatal(err) }
 one := openMemory(root, a)
 if len(one.avoidFor(fa)) != 1 || len(one.avoidFor(fb)) != 0 { t.Fatal("legacy migration was not scoped to the matching study") }
 if err := one.save(); err != nil { t.Fatal(err) }
 two := openMemory(root, b)
 if len(two.avoidFor(fb)) != 1 || len(two.avoidFor(fa)) != 1 { t.Fatal("link-specific memory was not retained") }
 if err := two.save(); err != nil { t.Fatal(err) }
 if one.path != two.path || one.path != filepath.Join(root,"Memory","p123456789.json") { t.Fatalf("incorrect PID path: %s / %s", one.path, two.path) }
 if _, err := os.Stat(one.path); err != nil { t.Fatal(err) }
 if _, err := os.Stat(two.path); err != nil { t.Fatal(err) }
 if len(openMemory(root, a).avoidFor(fa)) != 1 { t.Fatal("scoped memory did not reload") }
 if got:=openMemory(root,a).avoidFor(fb); len(got)!=1 || got[0].Question!="second" { t.Fatal("link keys were mixed") }
 if len(openMemory(root).avoidFor(fb)) != 1 { t.Fatal("legacy memory was changed") }
}

func TestPIDMemoryImportsPreviousPerLinkFile(t *testing.T) {
 root:=t.TempDir();u:="https://survey.example.test/x?sname=p123456789";fp:=memoryFingerprint(u)
 old:=&MemoryStore{path:previousStudyMemoryPath(root,u),Surveys:map[string]*SurveyMemory{}}
 old.addAvoid(fp,"q","old","screenout-suspect");if err:=old.save();err!=nil { t.Fatal(err) }
 newStore:=openMemory(root,u);if len(newStore.avoidFor(fp))!=1 { t.Fatal("previous memory not migrated") }
 if err:=newStore.save();err!=nil { t.Fatal(err) }
 if _,err:=os.Stat(old.path);err!=nil { t.Fatal("old memory was removed") }
 if _,err:=os.Stat(filepath.Join(root,"Memory","p123456789.json"));err!=nil { t.Fatal(err) }
}

func TestMemoryFingerprintKeepsStableIdentity(t *testing.T) {
	a := memoryFingerprint("https://survey.example.com/extwix/x.aspx?__etk=BOX&session=1")
	b := memoryFingerprint("https://survey.example.com/extwix/x.aspx?__etk=OTHER&session=2")
	if a == b {
		t.Fatalf("different __etk entry tokens must not leak memory between studies: %q", a)
	}
	same := memoryFingerprint("https://survey.example.com/extwix/x.aspx?__etk=BOX&session=OTHER")
	if a != same {
		t.Fatalf("session-only changes should not fragment a study: %q != %q", a, same)
	}
	c := memoryFingerprint("https://survey.example.com/extwix/x.aspx?sname=projectA")
	if a == c {
		t.Fatal("a different sname is a different survey")
	}
}

func TestPairsKeyIgnoresOrder(t *testing.T) {
	x := []answerPair{{Question: "q1", Option: "a"}, {Question: "q2", Option: "b"}}
	y := []answerPair{{Question: "q2", Option: "b"}, {Question: "q1", Option: "a"}}
	if pairsKey(x) != pairsKey(y) {
		t.Fatal("pair order must not matter")
	}
	if pairsKey(nil) != "" {
		t.Fatal("no pairs should fingerprint as empty")
	}
}

func TestVaryPersonaRollsButKeepsIdentityFields(t *testing.T) {
	base := Persona{Gender: "Male", Age: 30, Country: "United Kingdom", City: "London", YesNo: "Yes", Rating: "middle", MultiSelectCount: 2, OpenText: "This is a test response.", Number: 30, ReentryTemplate: "MT{uid}", ReentryStart: 1, AnswerMode: "persona", VariedPerRun: true}
	genders, yesNos := map[string]bool{}, map[string]bool{}
	for i := 0; i < 40; i++ {
		p := base
		varyPersona(&p)
		genders[p.Gender] = true
		yesNos[p.YesNo] = true
		if p.Country != base.Country || p.OpenText != base.OpenText || p.Rating != base.Rating ||
			p.ReentryTemplate != base.ReentryTemplate || p.ReentryStart != base.ReentryStart || p.Name != base.Name {
			t.Fatalf("identity fields must stay as configured, got %#v", p)
		}
		if p.Gender != "Male" && p.Gender != "Female" && p.Gender != "Other" {
			t.Fatalf("unexpected gender %q", p.Gender)
		}
		if p.YesNo != "Yes" && p.YesNo != "No" {
			t.Fatalf("unexpected yesNo %q", p.YesNo)
		}
		if p.Age < 18 || p.Age > 60 {
			t.Fatalf("age out of range: %d", p.Age)
		}
		if p.MultiSelectCount < 2 || p.MultiSelectCount > 6 {
			t.Fatalf("multiSelectCount out of range: %d", p.MultiSelectCount)
		}
		if p.Number != p.Age {
			t.Fatalf("free-text number should follow the rolled age, got %d vs %d", p.Number, p.Age)
		}
	}
	if len(genders) < 2 || len(yesNos) < 2 {
		t.Fatalf("every run must roll a different answer set, got genders=%v yesNos=%v", genders, yesNos)
	}
}

func TestClearPairsDropsOnlyTheCompletedSelections(t *testing.T) {
	s := openMemory(t.TempDir())
	fp := memoryFingerprint("http://example.com/s")
	s.addAvoid(fp, "city question", "london", "screenout")
	s.addAvoid(fp, "city question", "norwich", "screenout")
	s.addAvoid(fp, "frequency question", "daily", "validation")

	// A run that registered as complete answered london and daily: those come back.
	removed := s.clearPairs(fp, []answerPair{
		{Question: "city question", Option: "London"},
		{Question: "frequency question", Option: "daily"},
	})
	if removed != 2 {
		t.Fatalf("expected 2 rules cleared for the completed run, got %d", removed)
	}
	if got := s.avoidCount(fp); got != 1 {
		t.Fatalf("the untouched rule must stay, got %d rule(s)", got)
	}
	if m := s.forURL("http://example.com/s"); len(m.Avoid) != 1 || m.Avoid[0].Option != "norwich" {
		t.Fatalf("only the completed selections should be cleared, got %#v", m.Avoid)
	}
	if n := s.clearPairs(fp, []answerPair{{Question: "city question", Option: "london"}}); n != 0 {
		t.Fatalf("nothing left to clear on a second pass, got %d", n)
	}
	if n := s.clearPairs(fp, nil); n != 0 {
		t.Fatalf("an empty run must clear nothing, got %d", n)
	}
}

// A screen-out is remembered for good: when a question runs out of safe options the
// engine clears its bans so it can answer again, but the selections that ended a run
// must survive that cleanup and stay avoided in the next response.
func TestClearQuestionKeepsScreenoutRules(t *testing.T) {
	dir := t.TempDir()
	s := openMemory(dir)
	fp := "example.com/survey"
	s.addAvoid(fp, "gender question", "male", "screenout")
	s.addAvoid(fp, "gender question", "other", "validation")
	if removed := s.clearQuestion(fp, "gender question"); removed != 1 {
		t.Fatalf("expected only the validation rule to be cleared, got %d", removed)
	}
	if n := s.avoidCount(fp); n != 1 {
		t.Fatalf("expected the screen-out rule to survive, got %d rules", n)
	}
	m := s.Surveys[fp]
	if m == nil || len(m.Avoid) != 1 || m.Avoid[0].Reason != "screenout" {
		t.Fatalf("screen-out rule was dropped by clearQuestion: %#v", m)
	}
}

// A run that ends badly must be remembered as a whole profile, so the next run does
// not rebuild the same answers, and each of its answers is avoided within budget.
func TestNoteAttemptRemembersFailedProfile(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	pairs := []answerPair{
		{Question: "city", Option: "norwich"},
		{Question: "housing", Option: "no"},
	}
	s.noteAttempt(fp, pairs, "screenout")

	m := s.forURL("http://example.com/s")
	if len(m.Attempts) != 1 || m.Attempts[0].Status != "screenout" {
		t.Fatalf("attempt not recorded: %#v", m.Attempts)
	}
	if m.Failed != 1 {
		t.Fatalf("failed counter = %d, want 1", m.Failed)
	}
	if got := s.rejectedCombos(fp); len(got) != 1 {
		t.Fatalf("rejected combo not remembered: %#v", got)
	}
	want := "city\x00norwich\x00housing\x00no"
	if got := s.rejectedCombos(fp)[0]; got != want {
		t.Fatalf("rejected trail %q, want %q", got, want)
	}
	// The same answers in another order are the same profile.
	same := []answerPair{pairs[1], pairs[0]}
	if comboFingerprint(same) != comboFingerprint(pairs) {
		t.Fatal("fingerprint must not depend on answer order")
	}
}

// A completed run becomes the profile the store replays next time.
func TestCompletedAttemptBecomesPreferredProfile(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	pairs := []answerPair{
		{Question: "city", Option: "london"},
		{Question: "housing", Option: "yes"},
	}
	if _, done := s.noteAttempt(fp, pairs, "completed"); !done {
		t.Fatal("completion not recorded")
	}
	prefer := s.preferFor(fp)
	if prefer["city"] != "london" || prefer["housing"] != "yes" {
		t.Fatalf("winning answers not preferred: %#v", prefer)
	}
	m := s.forURL("http://example.com/s")
	if m.Completed != 1 {
		t.Fatalf("completed = %d, want 1", m.Completed)
	}
	// An answer nobody has won with yet must not be handed to the engine as a preference.
	s.noteAttempt(fp, []answerPair{{Question: "age", Option: "30"}}, "screenout")
	if _, ok := s.preferFor(fp)["age"]; ok {
		t.Fatal("an answer that never completed a run must not be preferred")
	}
}

// A completion has to open the path it used: rules learned while it was in flight must
// not close the answers that just worked.
func TestCompletionClearsAvoidForWinningAnswers(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	pairs := []answerPair{{Question: "housing", Option: "yes"}}
	s.addAvoid(fp, "housing", "yes", "validation")
	s.noteAttempt(fp, pairs, "completed")
	if m := s.forURL("http://example.com/s"); len(m.Avoid) != 0 {
		t.Fatalf("winning answer still avoided: %#v", m.Avoid)
	}
}

// An answer that completed this survey is protected: it survives a couple of bad
// attempts before the store is allowed to avoid it.
func TestGoodAnswerSurvivesBadAttempts(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	good := []answerPair{{Question: "housing", Option: "yes"}}
	s.noteAttempt(fp, good, "completed")

	// One bad run that also used the good answer must not blacklist it.
	s.noteAttempt(fp, good, "screenout")
	if _, ok := s.rememberAvoid(fp, "housing", "yes", "screenout"); ok {
		t.Fatal("a remembered good answer must stay selectable after one bad attempt")
	}
	// Keep failing with it: an answer that took this survey to its end page is still the
	// way back in, so later rejections are never allowed to rule it out. Losing it is how
	// a profile that had already qualified stopped being replayed.
	s.noteAttempt(fp, good, "screenout")
	s.noteAttempt(fp, good, "screenout")
	if _, ok := s.rememberAvoid(fp, "housing", "yes", "screenout"); ok {
		t.Fatal("the answer that completed this survey must stay selectable as the fail-safe")
	}
	if got := s.preferFor(fp)["housing"]; got != "yes" {
		t.Fatalf("the qualifying answer must be replayed, got %q", got)
	}
}

// An answer that never got anywhere is not a fail-safe: once it has failed often enough
// the store steers away from it, which is how the search moves.
func TestUnprovenAnswerIsEventuallyAvoided(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	p := []answerPair{{Question: "housing", Option: "no"}}
	s.noteAttempt(fp, p, "screenout")
	s.noteAttempt(fp, p, "screenout")
	s.noteAttempt(fp, p, "screenout")
	if _, ok := s.rememberAvoid(fp, "housing", "no", "screenout"); !ok {
		t.Fatal("an answer that never got past its first question should eventually be avoided")
	}
}

// The history is a steering aid, not an archive.
func TestAttemptHistoryIsBounded(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	for i := 0; i < maxAttemptsKept*3; i++ {
		s.noteAttempt(fp, []answerPair{{Question: "city", Option: "city" + strconv.Itoa(i)}}, "screenout")
	}
	m := s.forURL("http://example.com/s")
	if len(m.Attempts) != maxAttemptsKept {
		t.Fatalf("attempts kept = %d, want %d", len(m.Attempts), maxAttemptsKept)
	}
	if len(m.Rejected) > maxRejectedKept {
		t.Fatalf("rejected kept = %d, want at most %d", len(m.Rejected), maxRejectedKept)
	}
}

// Legacy unscoped keys are readable but never applied to a new strict study key.
func TestLegacyMemoryStillLoads(t *testing.T) {
 dir:=t.TempDir()
 if err:=os.MkdirAll(filepath.Join(dir,"data"),0755); err!=nil { t.Fatal(err) }
 legacy:=[]byte(`{"surveys":{"example.com/s":{"completed":2}}}`)
 path:=filepath.Join(dir,"data","survey-memory.json")
 if err:=os.WriteFile(path,legacy,0644); err!=nil { t.Fatal(err) }
 s:=openMemory(dir)
 if len(s.Surveys)!=0 { t.Fatal("old unscoped memory must not be silently replayed") }
 b,err:=os.ReadFile(path); if err!=nil || string(b)!=string(legacy) { t.Fatal("old file should remain untouched") }
}

// The attempt that carried a profile furthest is remembered, replayed and protected:
// it is the fail-safe a later run falls back to when a fresh draw is rejected.
func TestFurthestAttemptIsRememberedAndProtected(t *testing.T) {
	s := &MemoryStore{path: t.TempDir() + "/survey-memory.json", Surveys: map[string]*SurveyMemory{}}
	fp := memoryFingerprint("http://example.com/s")
	pairs := []answerPair{{Question: "income", Option: "35-50k"}, {Question: "region", Option: "London"}}

	if !s.noteFurthest(fp, pairs, 120) {
		t.Fatal("the first attempt to get anywhere must be recorded")
	}
	if s.noteFurthest(fp, pairs, 40) {
		t.Fatal("a shallower attempt must not replace what got furthest")
	}
	if got := s.preferFor(fp)["income"]; got != "" {
		t.Fatalf("an incomplete path must not be replayed as a success, got %q", got)
	}
	// Later rejections may not rule out the answer that got a respondent through.
	for i := 0; i < 6; i++ {
		s.noteAttempt(fp, pairs, "screenout")
	}
	if _, ok := s.rememberAvoid(fp, "income", "35-50k", "screenout"); ok {
		t.Fatal("the answer that carried the profile furthest must stay selectable")
	}
	if _, ok := s.rememberAvoid(fp, "unrelated", "nope", "screenout"); !ok {
		t.Fatal("an unknown answer should still be ruleable")
	}
	if got := s.furthest(fp); got != 120 {
		t.Fatalf("furthest depth = %d, want 120", got)
	}
}

func TestReplayablePairsSkipsTextAndAmbiguousAnswers(t *testing.T) {
 p:=[]answerPair{{"q1","yes"},{"q1","yes"},{"q2","red"},{"q2","blue"},{"text|id","private"}}
 got:=replayablePairs(p)
 if len(got)!=1 || got[0].Question!="q1" || got[0].Option!="yes" {t.Fatalf("unsafe replay pairs: %#v",got)}
}

func TestTerminationSuspectsPersistWithoutSuccessAndRemainSurveyScoped(t *testing.T) {
 root := t.TempDir()
 link := "https://survey.example.test/form?study=alpha"
 first := openMemory(root)
 fp := memoryFingerprint(link)
 first.forURL(link)
 if ok,_ := first.addAvoid(fp,"preference","choice a","screenout-suspect"); !ok { t.Fatal("suspect not stored") }
 first.save()
 second := openMemory(root)
 got := second.avoidFor(fp)
 if len(got)!=1 || got[0].Question!="preference" || got[0].Option!="choice a" {t.Fatalf("suspect not restored: %#v",got)}
 if len(second.avoidFor(memoryFingerprint("https://survey.example.test/form?study=beta")))!=0 {t.Fatal("avoid rule leaked to another study")}
}

func TestQuotaAndUncertainValidationAreNotPersistedAsSuspects(t *testing.T) {
 root:=t.TempDir()
 first:=openMemory(root)
 fp:=memoryFingerprint("https://survey.example.test/form?study=alpha")
 first.addAvoid(fp,"a","x","quota")
 first.addAvoid(fp,"b","y","validation")
 first.save()
 second:=openMemory(root)
 if len(second.avoidFor(fp))!=0 {t.Fatal("non-termination outcomes were stored as suspects")}
}

func TestRecordTerminationSuspectPersistsAndSkipsQuotaOrAmbiguity(t *testing.T) {
 root:=t.TempDir();s:=openMemory(root);fp:=memoryFingerprint("https://survey.example.test/one")
 one:=[]answerPair{{Question:"product", Option:"alpha"}}
 if ok,err:=s.recordScreenoutSuspect(fp,one,"quota");ok||err!=nil {t.Fatal("quota must not blacklist a choice")}
 if ok,err:=s.recordScreenoutSuspect(fp,append(one,answerPair{Question:"q2",Option:"beta"}),"screenout");ok||err!=nil {t.Fatal("ambiguous pages must not blacklist a choice")}
 if ok,err:=s.recordScreenoutSuspect(fp,one,"screenout");!ok||err!=nil {t.Fatalf("screenout did not save suspect: %v",err)}
 reloaded:=openMemory(root);got:=reloaded.avoidFor(fp)
 if len(got)!=1||got[0].Question!="product"||got[0].Option!="alpha" {t.Fatalf("bad persisted suspect: %#v",got)}
}
