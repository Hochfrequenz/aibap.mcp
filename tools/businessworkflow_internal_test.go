package tools

import "testing"

func TestWorkflowLanguage(t *testing.T) {
	cases := map[string]string{
		"EN": "E", "DE": "D", "en": "E", "de": "D", "": "E", "F": "F", "fr": "F",
	}
	for in, want := range cases {
		if got := workflowLanguage(in); got != want {
			t.Errorf("workflowLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}
