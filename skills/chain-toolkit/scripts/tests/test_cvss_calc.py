import json
import pathlib
import subprocess
import sys
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "cvss_calc.py"


def _run(payload):
    p = subprocess.run(
        [sys.executable, "-I", str(SCRIPT), "json"],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
    )
    assert p.returncode == 0, p.stderr
    return json.loads(p.stdout)


class CvssCalcTest(unittest.TestCase):
    def test_known_class_scores(self):
        out = _run({"vulnclass": "sqli"})
        self.assertIn(out["severity"], {"critical", "high", "medium", "low"})
        self.assertIsInstance(out["score"], (int, float))
        self.assertGreater(out["score"], 0)
        self.assertTrue(out["vector"].startswith("AV:"))

    def test_unknown_class_safe_default(self):
        out = _run({"vulnclass": "made-up-class"})
        self.assertIn(out["severity"], {"critical", "high", "medium", "low"})
        self.assertTrue(out["vector"].startswith("AV:"))


if __name__ == "__main__":
    unittest.main()
