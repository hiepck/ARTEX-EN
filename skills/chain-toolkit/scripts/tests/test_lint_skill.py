import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

LINT = pathlib.Path(__file__).resolve().parent / "_lint_skill.py"


def _lint(text):
    with tempfile.NamedTemporaryFile("w", suffix=".md", delete=False) as f:
        f.write(text)
        path = f.name
    try:
        return subprocess.run(
            [sys.executable, "-I", str(LINT), path], capture_output=True, text=True
        )
    finally:
        os.unlink(path)


class LintSkillTest(unittest.TestCase):
    def test_valid_skill_passes(self):
        r = _lint("---\nname: hunt-x\ndescription: hunts x\n---\n# body\n")
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_missing_description_fails(self):
        r = _lint("---\nname: hunt-x\n---\n# body\n")
        self.assertEqual(r.returncode, 1)

    def test_stale_penred_token_fails(self):
        r = _lint("---\nname: hunt-x\ndescription: d\n---\nuse oauth-hunter agent\n")
        self.assertEqual(r.returncode, 1)
        self.assertIn("oauth-hunter", r.stdout + r.stderr)

    def test_legit_vuln_class_name_allowed(self):
        # subdomain-takeover is a real vuln class (H1 #145), not only a penred
        # sub-agent — it must be allowed in ported content.
        r = _lint("---\nname: hunt-x\ndescription: d\n---\n### subdomain-takeover anchors\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)


if __name__ == "__main__":
    unittest.main()
