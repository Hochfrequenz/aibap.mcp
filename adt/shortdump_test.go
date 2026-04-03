package adt

import (
	"strings"
	"testing"
)

func TestParseShortDumpFeed(t *testing.T) {
	feed := []byte(`<?xml version="1.0" encoding="utf-8"?>
<atom:feed xmlns:atom="http://www.w3.org/2005/Atom">
  <atom:entry>
    <atom:author><atom:name>DEVELOPER</atom:name></atom:author>
    <atom:category term="UNCAUGHT_EXCEPTION" label="ABAP-Laufzeitfehler"/>
    <atom:category term="CL_MY_CLASS=====CP" label="Beendetes ABAP-Programm"/>
    <atom:published>2026-04-02T21:41:04Z</atom:published>
    <atom:summary type="html">&lt;h4 id="HEADER"&gt;Kopfinformation&lt;/h4&gt;&lt;table&gt;&lt;tr&gt;&lt;td&gt;Laufzeitfehler&lt;/td&gt;&lt;td&gt;UNCAUGHT_EXCEPTION&lt;/td&gt;&lt;/tr&gt;&lt;tr&gt;&lt;td&gt;Ausnahme&lt;/td&gt;&lt;td&gt;CX_MY_ERROR&lt;/td&gt;&lt;/tr&gt;&lt;/table&gt;&lt;h4 id="WHATHAPPENED"&gt;Was ist passiert?&lt;/h4&gt;&lt;p&gt;Die Ausnahme wurde ausgelöst.&lt;br&gt;Reason: test error&lt;/p&gt;&lt;h4 id="ERROR"&gt;Fehleranalyse&lt;/h4&gt;&lt;p&gt;Es ist eine Ausnahme aufgetreten.&lt;/p&gt;&lt;h4 id="TERMINATION"&gt;Abbruchstelle&lt;/h4&gt;&lt;p&gt;Include: CL_MY_CLASS=====CCIMP, Line 42&lt;/p&gt;&lt;h4 id="SOURCE"&gt;Quelltext&lt;/h4&gt;&lt;pre&gt;code here&lt;/pre&gt;&lt;h4 id="STACK"&gt;Aufrufe&lt;/h4&gt;&lt;table&gt;&lt;tr&gt;&lt;td&gt;METHOD_A&lt;/td&gt;&lt;td&gt;CL_MY_CLASS=====CCIMP&lt;/td&gt;&lt;td&gt;42&lt;/td&gt;&lt;/tr&gt;&lt;tr&gt;&lt;td&gt;METHOD_B&lt;/td&gt;&lt;td&gt;CL_CALLER=====CCIMP&lt;/td&gt;&lt;td&gt;10&lt;/td&gt;&lt;/tr&gt;&lt;/table&gt;</atom:summary>
  </atom:entry>
</atom:feed>`)

	dumps, err := parseShortDumpFeed(feed, "")
	if err != nil {
		t.Fatalf("parseShortDumpFeed: %v", err)
	}
	if len(dumps) != 1 {
		t.Fatalf("expected 1 dump, got %d", len(dumps))
	}

	d := dumps[0]
	if d.RuntimeError != "UNCAUGHT_EXCEPTION" {
		t.Errorf("RuntimeError: got %q", d.RuntimeError)
	}
	if d.Program != "CL_MY_CLASS=====CP" {
		t.Errorf("Program: got %q", d.Program)
	}
	if d.User != "DEVELOPER" {
		t.Errorf("User: got %q", d.User)
	}
	if d.Timestamp != "2026-04-02T21:41:04Z" {
		t.Errorf("Timestamp: got %q", d.Timestamp)
	}
	if !strings.Contains(d.Header, "UNCAUGHT_EXCEPTION") {
		t.Errorf("Header should contain error type, got %q", d.Header)
	}
	if !strings.Contains(d.WhatHappened, "Ausnahme") {
		t.Errorf("WhatHappened: got %q", d.WhatHappened)
	}
	if !strings.Contains(d.AbortLocation, "Line 42") {
		t.Errorf("AbortLocation: got %q", d.AbortLocation)
	}
	if !strings.Contains(d.CallStack, "METHOD_A") {
		t.Errorf("CallStack: got %q", d.CallStack)
	}
}

func TestParseShortDumpFeed_FilterUser(t *testing.T) {
	feed := []byte(`<?xml version="1.0" encoding="utf-8"?>
<atom:feed xmlns:atom="http://www.w3.org/2005/Atom">
  <atom:entry>
    <atom:author><atom:name>USER_A</atom:name></atom:author>
    <atom:category term="ERROR_A" label="ABAP-Laufzeitfehler"/>
    <atom:published>2026-04-02T10:00:00Z</atom:published>
    <atom:summary type="html"></atom:summary>
  </atom:entry>
  <atom:entry>
    <atom:author><atom:name>USER_B</atom:name></atom:author>
    <atom:category term="ERROR_B" label="ABAP-Laufzeitfehler"/>
    <atom:published>2026-04-02T11:00:00Z</atom:published>
    <atom:summary type="html"></atom:summary>
  </atom:entry>
</atom:feed>`)

	dumps, err := parseShortDumpFeed(feed, "USER_A")
	if err != nil {
		t.Fatalf("parseShortDumpFeed: %v", err)
	}
	if len(dumps) != 1 {
		t.Fatalf("expected 1 dump after filter, got %d", len(dumps))
	}
	if dumps[0].RuntimeError != "ERROR_A" {
		t.Errorf("expected ERROR_A, got %q", dumps[0].RuntimeError)
	}
}

func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"br tags", "line1<br>line2", "line1\nline2"},
		{"p tags", "<p>hello</p><p>world</p>", "hello\nworld"},
		{"entities", "&amp; &lt; &gt; &quot;", "& < > \""},
		{"table", "<tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr>", "A | B | \nC | D |"},
		{"nested tags", "<b><i>bold italic</i></b>", "bold italic"},
		{"nbsp", "a&nbsp;&nbsp;b", "a b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToText(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractSection(t *testing.T) {
	html := `<h4 id="A">Title A</h4><p>content a</p><h4 id="B">Title B</h4><p>content b</p>`
	got := extractSection(html, "A")
	if !strings.Contains(got, "content a") {
		t.Errorf("section A: got %q", got)
	}
	got = extractSection(html, "B")
	if !strings.Contains(got, "content b") {
		t.Errorf("section B: got %q", got)
	}
	got = extractSection(html, "C")
	if got != "" {
		t.Errorf("section C should be empty, got %q", got)
	}
}
