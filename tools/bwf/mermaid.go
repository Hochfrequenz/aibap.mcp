package bwf

import (
	"fmt"
	"strings"
)

// renderMermaid renders a definition's nodes and edges as a Mermaid flowchart.
func renderMermaid(d *Definition) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, n := range d.Nodes {
		fmt.Fprintf(&b, "    n%s[\"%s\"]\n", n.ID, mermaidLabel(n.Label))
	}
	for _, e := range d.Edges {
		if e.Label != "" {
			fmt.Fprintf(&b, "    n%s -->|%s| n%s\n", e.From, strings.NewReplacer("|", "/", "\n", " ").Replace(e.Label), e.To)
		} else {
			fmt.Fprintf(&b, "    n%s --> n%s\n", e.From, e.To)
		}
	}
	return b.String()
}

func mermaidLabel(s string) string {
	return strings.NewReplacer(`"`, "#quot;", "\n", " ", "\r", " ").Replace(s)
}
