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
