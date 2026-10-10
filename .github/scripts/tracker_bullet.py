"""Parse and sanitize one checklist bullet of the adtler tracking issue.

Used by .github/workflows/adtler-bump-template.yml, which copies the tracking
issue's checklist into a public dependabot PR body. The summary is redacted
for anything that could name our internal environment (hosts, IPs, transport
numbers, registered namespaces) and for @-mentions.

Invoked with the bullet in the LINE environment variable; prints
"<checkbox>\\x1f<issue>\\x1f<summary>\\x1f<adtler ref>".
"""

import os
import re

BULLET_RE = re.compile(r"- \[(?P<mark>.)\] (?:\*\*)?(?P<issue>#[0-9]+)(?:\*\*)?(?: —|:)?(?P<rest>.*)$")
ADTLER_RE = re.compile(r"\s*\(adtler: ((?:\[[^\]]*\]\([^)]*\)|[^()])+)\)")

IPV4_RE = re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?\b")
DOTTED_RE = re.compile(r"\b[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+(?::\d+)?\b")
VERSION_RE = re.compile(r"v?\d+(?:\.\d+)+")
TRANSPORT_RE = re.compile(r"\b[A-Z0-9]{3}K9\d+\b")
NAMESPACE_RE = re.compile(r"/[A-Za-z0-9_]+/[A-Za-z0-9_/-]+\b")

# A two-label token whose second label is one of these is a file or repository
# name (object.go, aibap.mcp), not a host. None of them is a top-level domain;
# extensions that are (md, sh, py) are left out on purpose, so a two-label host
# under one of those TLDs stays redacted. Three or more labels, or a port, are
# always treated as a host.
NON_HOST_SUFFIXES = {"go", "mod", "sum", "yml", "yaml", "json", "txt", "xml", "abap", "mcp"}

# Markdown files are kept by exact name, because md is a top-level domain.
NON_HOST_NAMES = {"README.md", "CLAUDE.md", "AGENTS.md", "CHANGELOG.md", "CONTRIBUTING.md"}


def _is_harmless(token):
    if ":" in token:
        return False
    if VERSION_RE.fullmatch(token) or token in NON_HOST_NAMES:
        return True
    labels = token.split(".")
    # Abbreviations such as e.g / i.e: every label a single letter.
    if all(len(label) == 1 and label.isalpha() for label in labels):
        return True
    return len(labels) == 2 and labels[1].lower() in NON_HOST_SUFFIXES


def sanitize_summary(summary):
    summary = " ".join(summary.split())
    summary = IPV4_RE.sub("<redacted>", summary)
    summary = DOTTED_RE.sub(lambda m: m.group(0) if _is_harmless(m.group(0)) else "<redacted>", summary)
    summary = TRANSPORT_RE.sub("<redacted>", summary)
    summary = NAMESPACE_RE.sub("<redacted>", summary)
    return summary.replace("@", "@ ")


def parse_bullet(line):
    match = BULLET_RE.match(line)
    rest = (match.group("rest") or "").strip()
    adtler_match = ADTLER_RE.search(rest)
    adtler = adtler_match.group(1) if adtler_match else ""
    summary = sanitize_summary(ADTLER_RE.sub("", rest).strip())
    return "- [{}]".format(match.group("mark")), match.group("issue"), summary, adtler


if __name__ == "__main__":
    print("\x1f".join(parse_bullet(os.environ["LINE"])))
