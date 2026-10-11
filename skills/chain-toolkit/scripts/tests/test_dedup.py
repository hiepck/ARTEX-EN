import json
import pathlib
import subprocess
import sys
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "dedup.py"


def _run(payload):
    p = subprocess.run(
        [sys.executable, "-I", str(SCRIPT)],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
    )
    assert p.returncode == 0, p.stderr
    return json.loads(p.stdout)


class DedupTest(unittest.TestCase):
    def test_empty_corpus_not_duplicate(self):
        out = _run({
            "candidate": {"vulnclass": "xss", "endpoint": "/search", "param": "q", "evidence": "alert(1)"},
            "corpus": [],
            "threshold": 3,
        })
        self.assertEqual(out, {"duplicate": False, "nearest_hamming": None})

    def test_near_duplicate_detected(self):
        # Same IDOR route, different numeric id — the systemic-dedup case
        # simhash is built for: the id is normalized to {id}, so the two
        # collapse to one bug (Hamming 0) at a tight threshold.
        cand = {"vulnclass": "idor", "endpoint": "/api/users/123", "param": "id", "evidence": "leaked other user"}
        prior = {"vulnclass": "idor", "endpoint": "/api/users/999", "param": "id", "evidence": "leaked other user"}
        out = _run({"candidate": cand, "corpus": [prior], "threshold": 3})
        self.assertTrue(out["duplicate"])
        self.assertEqual(out["nearest_hamming"], 0)

    def test_distinct_not_duplicate(self):
        cand = {"vulnclass": "xss", "endpoint": "/a", "param": "x", "evidence": "alert(1)"}
        prior = {"vulnclass": "sqli", "endpoint": "/z", "param": "id", "evidence": "union select"}
        out = _run({"candidate": cand, "corpus": [prior], "threshold": 3})
        self.assertFalse(out["duplicate"])


if __name__ == "__main__":
    unittest.main()
