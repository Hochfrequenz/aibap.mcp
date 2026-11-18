package tools

import "testing"

func TestWorkflowLanguage(t *testing.T) {
	cases := map[string]string{
		"EN": "E", "DE": "D", "en": "E", "de": "D", "": "E", " ": "E",
		"F": "F", "f": "F", "S": "S", "1": "1",
	}
	for in, want := range cases {
		got, err := workflowLanguage(in)
		if err != nil || got != want {
			t.Errorf("workflowLanguage(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"fr", "ES", "da", "eng", "deu"} {
		if got, err := workflowLanguage(in); err == nil {
			t.Errorf("workflowLanguage(%q) = %q, want error", in, got)
		}
	}
}
