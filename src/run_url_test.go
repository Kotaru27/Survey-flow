package main

import "testing"

func TestParseRunURL(t *testing.T) {
 tests := []struct { input, want string; visible, invalid bool }{
  {"https://example.test/start?a=---pop&b=1#x", "https://example.test/start?a=---pop&b=1#x", false, false},
  {"https://example.test/start?a=1#x---pop", "https://example.test/start?a=1#x", true, false},
  {"https://example.test/start---pop---pop", "https://example.test/start---pop", true, false},
  {"https://", "", false, true},
  {"file:///tmp/test---pop", "", true, true},
 }
 for _, tc := range tests {
  got, visible, err := parseRunURL(tc.input)
  if got != tc.want || visible != tc.visible || (err != nil) != tc.invalid {
   t.Errorf("parseRunURL(%q) = %q, %v, %v", tc.input, got, visible, err)
  }
 }
}

func TestBatchAttemptLimit(t *testing.T) {
 if got := batchAttemptLimit(50, 0); got != 50 * maxRunsPerComplete { t.Fatalf("default limit = %d", got) }
 if got := batchAttemptLimit(50, 25); got != 25 { t.Fatalf("configured limit = %d", got) }
 if got := batchAttemptLimit(50, 75); got != 75 { t.Fatalf("configured limit = %d", got) }
}
