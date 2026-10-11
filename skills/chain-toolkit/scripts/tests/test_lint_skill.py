import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

LINT = pathlib.Path(__file__).resolve().parent / "_lint_skill.py"


def _lint(text, name):
    d = tempfile.mkdtemp()
    path = os.path.join(d, name)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    try:
        return subprocess.run(
            [sys.executable, "-I", str(LINT), path], capture_output=True, text=True
        )
    finally:
        os.unlink(path)
        os.rmdir(d)


def _skill(text):
    return _lint(text, "SKILL.md")


def _ref(text, name="chain-table.md"):
    return _lint(text, name)


class LintSkillTest(unittest.TestCase):
    # --- SKILL.md: frontmatter required ---
    def test_valid_skill_passes(self):
        r = _skill("---\nname: hunt-x\ndescription: hunts x\n---\n# body\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_missing_description_fails(self):
        r = _skill("---\nname: hunt-x\n---\n# body\n")
        self.assertEqual(r.returncode, 1)

    # --- SKILL.md: penred sub-agent names rejected ---
    def test_skill_rejects_hunter_agent(self):
        r = _skill("---\nname: hunt-x\ndescription: d\n---\ndispatch cors-hunter here\n")
        self.assertEqual(r.returncode, 1)
        self.assertIn("cors-hunter", r.stdout + r.stderr)

    def test_skill_rejects_non_hunter_penred_agent(self):
        r = _skill("---\nname: hunt-x\ndescription: d\n---\nhand to auth-tester\n")
        self.assertEqual(r.returncode, 1)

    def test_skill_url_with_hunters_word_allowed(self):
        # "most-hunters-miss" in a URL must NOT trip the \b..-hunter\b check.
        r = _skill("---\nname: hunt-x\ndescription: d\n---\nsee /the-exploit-most-hunters-miss-2026\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_legit_vuln_class_name_allowed(self):
        # subdomain-takeover / privilege-escalation are real vuln classes, not agents.
        r = _skill("---\nname: hunt-x\ndescription: d\n---\n### subdomain-takeover; then privilege-escalation\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    # --- references: agent labels allowed (data), frontmatter not required ---
    def test_reference_allows_agent_label(self):
        r = _ref("plain text mentioning oauth-hunter as a methodology label\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    # --- any file: penred paths rejected everywhere ---
    def test_any_file_rejects_penred_path(self):
        r = _ref("run tools/brain.py to record\n")
        self.assertEqual(r.returncode, 1)
        self.assertIn("tools/", r.stdout + r.stderr)

    def test_skill_rejects_penred_path(self):
        r = _skill("---\nname: x\ndescription: d\n---\nsee findings.json\n")
        self.assertEqual(r.returncode, 1)


if __name__ == "__main__":
    unittest.main()
