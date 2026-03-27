package tools_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestReadmeToolNamesMatchRegistered ensures every tool name listed in
// README.md corresponds to an actually registered MCP tool, and vice-versa.
// This prevents documentation drift (e.g. a renamed tool that is not updated
// in the README).
func TestReadmeToolNamesMatchRegistered(t *testing.T) {
	readmeTools := readmeToolNames(t)
	registeredTools := registeredToolNames(t)

	// Every README tool must be registered.
	for _, name := range readmeTools {
		if !contains(registeredTools, name) {
			t.Errorf("README lists tool %q but it is not registered", name)
		}
	}

	// Every registered tool must appear in the README.
	for _, name := range registeredTools {
		if !contains(readmeTools, name) {
			t.Errorf("registered tool %q is not listed in README.md", name)
		}
	}
}

// readmeToolNames extracts tool names from the markdown table rows in README.md.
// It looks for lines like: | `tool_name` | description |
func readmeToolNames(t *testing.T) []string {
	t.Helper()

	f, err := os.Open("../README.md")
	if err != nil {
		t.Fatalf("open README.md: %v", err)
	}
	defer f.Close()

	// Match table rows with backtick-quoted tool names: | `name` | ... |
	re := regexp.MustCompile("^\\|\\s*`([a-z_]+)`\\s*\\|")

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); m != nil {
			names = append(names, m[1])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning README.md: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("found no tool names in README.md — regex may need updating")
	}

	sort.Strings(names)
	return names
}

// registeredToolNames returns all tool names from a fully-wired test server.
func registeredToolNames(t *testing.T) []string {
	t.Helper()

	s := newTestServer(&mockClient{})

	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	resp := s.HandleMessage(context.Background(), []byte(msg))

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var envelope struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list response: %v\nraw: %s", err, string(respBytes))
	}
	if len(envelope.Result.Tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}

	var names []string
	for _, tool := range envelope.Result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func contains(sorted []string, s string) bool {
	i := sort.SearchStrings(sorted, s)
	return i < len(sorted) && sorted[i] == s
}

// TestReadmeToolCount checks that the tool count in the heading matches reality.
func TestReadmeToolCount(t *testing.T) {
	readmeTools := readmeToolNames(t)
	registeredTools := registeredToolNames(t)

	// Read the heading count from README
	f, err := os.Open("../README.md")
	if err != nil {
		t.Fatalf("open README.md: %v", err)
	}
	defer f.Close()

	re := regexp.MustCompile(`## Available tools \((\d+)\)`)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); m != nil {
			count := m[1]
			if count != strings.TrimSpace(count) {
				t.Fatal("unexpected whitespace in tool count")
			}
			expected := len(registeredTools)
			actual := len(readmeTools)
			if actual != expected {
				t.Errorf("README lists %d tools but %d are registered", actual, expected)
			}
			headingCount := 0
			for _, c := range count {
				headingCount = headingCount*10 + int(c-'0')
			}
			if headingCount != expected {
				t.Errorf("README heading says %d tools but %d are registered", headingCount, expected)
			}
			return
		}
	}
	t.Error("did not find '## Available tools (N)' heading in README.md")
}
