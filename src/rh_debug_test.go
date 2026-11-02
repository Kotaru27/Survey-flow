package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedStudyKey(t *testing.T) {
	key := []byte(`{"studyUrl":"https://survey.example.test/demo?__etk=EXAMPLE","rules":[{"contains":"Synthetic check?","answer":"Alpha","strict":true}]}`)
	footer := make([]byte, 16)
	copy(footer, "SFAQKEY1")
	binary.LittleEndian.PutUint64(footer[8:], uint64(len(key)))
	path := filepath.Join(t.TempDir(), "candidate.exe")
	data := append(append([]byte("MZ fixture"), key...), footer...)
	if err := os.WriteFile(path, data, 0600); err != nil { t.Fatal(err) }
	got := readEmbeddedRedHerring(path)
	if string(got) != string(key) { t.Fatal("embedded key not recovered") }
	if len(parseRedHerring(got, "https://survey.example.test/demo?__etk=EXAMPLE")) != 1 { t.Fatal("matching study key not loaded") }
	if len(parseRedHerring(got, "https://survey.example.test/other")) != 0 { t.Fatal("key leaked to another study") }
	binary.LittleEndian.PutUint64(data[len(data)-8:], 99999999)
	if err := os.WriteFile(path, data, 0600); err != nil { t.Fatal(err) }
	if readEmbeddedRedHerring(path) != nil { t.Fatal("invalid footer accepted") }
}

func TestEmbeddedGlobalChecksRequireNarrowMultiRules(t *testing.T) {
	b:=[]byte(`{"studyUrl":"https://survey.example.test/one","rules":[{"contains":"local rule"}],"globalRules":[{"contains":"This synthetic verification prompt is deliberately longer than forty characters.","kind":"multi_exact","strict":true,"answers":["Alpha"],"never":["Beta"]},{"contains":"short","kind":"multi_exact","strict":true,"answers":["X"],"never":["Y"]}]}`)
	if len(parseEmbeddedGlobalChecks(b))!=1 { t.Fatal("expected one narrow embedded global rule") }
	if len(parseRedHerring(b,"https://survey.example.test/other"))!=0 { t.Fatal("study-specific rules leaked to another link") }
	if len(parseRedHerring(b,"https://survey.example.test/one"))!=1 { t.Fatal("study-specific rules were lost") }
}

func TestPrivateStudyConfiguration(t *testing.T) {
 root:=t.TempDir();dir:=filepath.Join(root,"config");if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 fixture:=`{"studyUrl":"https://survey.example.test/one","rules":[{"contains":"local synthetic rule"}],"globalRules":[{"contains":"This synthetic verification prompt is deliberately longer than forty characters.","kind":"multi_exact","strict":true,"answers":["Alpha"],"never":["Beta"]}]}`
 if err:=os.WriteFile(filepath.Join(dir,"study-key.json"),[]byte(fixture),0600);err!=nil{t.Fatal(err)}
 if got:=readRedHerring(root,"https://survey.example.test/one");len(got)!=2{t.Fatalf("matching study: got %d rules, want 2",len(got))}
 if got:=readRedHerring(root,"https://survey.example.test/other");len(got)!=1{t.Fatalf("different study: got %d rules, want 1 shared rule",len(got))}
}

// repoRoot is the application root as it sits next to this package: src/ is the module,
// config/ and data/ are one level up.
func repoRoot(t *testing.T) string {
 t.Helper()
 root := t.TempDir()
 dir := filepath.Join(root, "config")
 if err := os.MkdirAll(dir, 0755); err != nil { t.Fatal(err) }
 fixture := `{"studyUrl":"https://survey.example.test/demo?__etk=EXAMPLE","rules":[{"text":"attention check one","answer":"alpha"},{"when":"attention check two","never":["beta"]}]}`
 if err := os.WriteFile(filepath.Join(dir,"red-herring.json"), []byte(fixture), 0600); err != nil { t.Fatal(err) }
 return root
}

// TestRedHerringPayload proves the attention-check key actually reaches the engine. The
// page is handed one injected string per step and a failed marshal would silently hand it
// an empty object - every rule dropped, every check left to chance - with nothing in the
// log to say so, which is exactly how the key stopped being read.
func TestRedHerringPayload(t *testing.T) {
	root := repoRoot(t)
	u := "https://survey.example.test/demo?__etk=EXAMPLE"
	mem := openMemory(root)
	fp := memoryFingerprint(u)
	sm := mem.forURL(u)
	avoidList := sm.Avoid
	if avoidList == nil {
		avoidList = []AvoidRule{}
	}
	payload := map[string]any{
		"avoid": avoidList, "reopen": []string{}, "forceNext": false,
		"prefer": mem.preferFor(fp), "banned": mem.rejectedCombos(fp), "needOpen": false,
		"score": mem.scoreFor(fp), "pin": map[string]string{}, "probeQ": "", "probeExclude": "",
		"redHerring": readRedHerring(root, u),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("the memory payload will not marshal, so the page gets nothing: %v", err)
	}
	var back struct {
		RedHerring []map[string]any `json:"redHerring"`
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("payload does not read back: %v", err)
	}
	// The engine keeps a rule when it names a question - id, id prefix, text or contains -
	// OR when it is a bare trap list, which names no question and protects every check.
	kept := 0
	for _, r := range back.RedHerring {
		if r["id"] != nil || r["idStarts"] != nil || r["text"] != nil || r["contains"] != nil {
			kept++
			continue
		}
		if list, ok := r["never"].([]any); ok && len(list) > 0 {
			kept++
		}
	}
	t.Logf("rules in payload=%d kept by the engine=%d", len(back.RedHerring), kept)
	if len(back.RedHerring) == 0 {
		t.Fatal("no attention-check rules reached the engine")
	}
	if kept < 2 {
		t.Fatalf("only %d of %d rules survive the engine's own filter", kept, len(back.RedHerring))
	}
	// The engine reads `const MEM=<payload>;`, so MEMORY has to land as an object literal.
	expr := strings.NewReplacer("MEMORY", string(b)).Replace("const MEM=MEMORY;MEM.redHerring")
	if !strings.HasPrefix(expr, "const MEM={") {
		t.Fatalf("MEMORY did not land as an object literal: %.60q", expr)
	}
}

// TestTrapListSurvives is the regression for the trap list being filtered away: a rule that
// only says which option codes end the interview names no question, so a filter that keeps
// only rules with text/id drops it and the trap answer goes in like any other.
func TestTrapListSurvives(t *testing.T) {
	root := repoRoot(t)
	rules := readRedHerring(root, "https://survey.example.test/demo?__etk=EXAMPLE")
	traps := 0
	for _, r := range rules {
		if list, ok := r["never"].([]any); ok && len(list) > 0 && r["answer"] == nil && r["index"] == nil {
			traps++
			if r["when"] == nil {
				t.Errorf("trap list %v has no `when`, so it will be applied to every question that reads like a check", r["note"])
			}
		}
	}
	if traps == 0 {
		t.Skip("this study's key has no trap list")
	}
	t.Logf("%d trap list(s) in the key", traps)
}

func TestStudyScopedRuleIsolation(t *testing.T) {
	root := repoRoot(t)
	original := "https://survey.example.test/demo?__etk=EXAMPLE"
	if len(readRedHerring(root, original)) == 0 {
		t.Fatal("expected original study's rules")
	}
	other := strings.Replace(original, "__etk=EXAMPLE", "__etk=ANOTHER_STUDY", 1)
	if got := readRedHerring(root, other); len(got) > 0 {
		t.Fatalf("previous study rules leaked: %v", got)
	}
}
