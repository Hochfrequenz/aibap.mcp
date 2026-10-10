"""Tests for tracker_bullet.py. Run: python3 -m unittest discover -s .github/scripts"""

import unittest

from tracker_bullet import parse_bullet, sanitize_summary


class SanitizeSummaryTest(unittest.TestCase):
    def test_keeps_harmless_dotted_words(self):
        cases = {
            "fix in aibap.mcp consumer": "fix in aibap.mcp consumer",
            "see tools/object.go, e.g. here": "see tools/object.go, e.g. here",
            "i.e. the README.md and config.yml": "i.e. the README.md and config.yml",
            "go.mod and go.sum change": "go.mod and go.sum change",
            "bump to v0.8.1": "bump to v0.8.1",
            "since 0.12.0": "since 0.12.0",
        }
        for summary, want in cases.items():
            with self.subTest(summary=summary):
                self.assertEqual(sanitize_summary(summary), want)

    def test_redacts_hosts(self):
        cases = {
            "get_source on sap.example.internal:44300": "get_source on <redacted>",
            "host sap.example.internal fails": "host <redacted> fails",
            "two-label host example.internal": "two-label host <redacted>",
            "file-like name with port object.go:8080": "file-like name with port <redacted>",
            "three labels ending in an extension a.b.go": "three labels ending in an extension <redacted>",
            "ip 10.1.2.3:8000 down": "ip <redacted> down",
            "ip 10.1.2.3 down": "ip <redacted> down",
        }
        for summary, want in cases.items():
            with self.subTest(summary=summary):
                self.assertEqual(sanitize_summary(summary), want)

    def test_redacts_transports_namespaces_and_mentions(self):
        self.assertEqual(sanitize_summary("request ABCK900123 stuck"), "request <redacted> stuck")
        self.assertEqual(sanitize_summary("class /ABC/CL_EXAMPLE breaks"), "class <redacted> breaks")
        self.assertEqual(sanitize_summary("ping @someone"), "ping @ someone")

    def test_collapses_whitespace(self):
        self.assertEqual(sanitize_summary("  a   b  "), "a b")


class ParseBulletTest(unittest.TestCase):
    def test_splits_fields(self):
        line = "- [ ] #560 — verify_source leaks in tools/verify.go (adtler: [#210](https://example.invalid/pull/210))"
        self.assertEqual(
            parse_bullet(line),
            ("- [ ]", "#560", "verify_source leaks in tools/verify.go", "[#210](https://example.invalid/pull/210)"),
        )

    def test_bold_issue_and_no_summary(self):
        self.assertEqual(parse_bullet("- [x] **#12**"), ("- [x]", "#12", "", ""))


if __name__ == "__main__":
    unittest.main()
