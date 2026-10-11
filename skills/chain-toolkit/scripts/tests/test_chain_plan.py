import json
import pathlib
import subprocess
import sys
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "chain_plan.py"


def _run(payload):
    p = subprocess.run(
        [sys.executable, "-I", str(SCRIPT)],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
    )
    assert p.returncode == 0, p.stderr
    return json.loads(p.stdout)


class ChainPlanTest(unittest.TestCase):
    def test_known_class_yields_candidates(self):
        out = _run({"findings": [{"id": 1, "vulnclass": "open-redirect", "severity": "low"}]})
        c = out["candidates"][0]
        self.assertEqual(c["finding_id"], 1)
        self.assertGreaterEqual(len(c["next"]), 1)
        self.assertTrue(all("hop" in h and "toward" in h for h in c["next"]))

    def test_unknown_class_yields_empty_next(self):
        out = _run({"findings": [{"id": 9, "vulnclass": "no-such-class", "severity": "low"}]})
        self.assertEqual(out["candidates"][0]["next"], [])

    def test_empty_findings(self):
        out = _run({"findings": []})
        self.assertEqual(out["candidates"], [])


if __name__ == "__main__":
    unittest.main()
