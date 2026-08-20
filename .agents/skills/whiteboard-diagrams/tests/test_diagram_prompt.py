from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "diagram_prompt.py"
SPEC = importlib.util.spec_from_file_location("diagram_prompt", MODULE_PATH)
assert SPEC and SPEC.loader
diagram_prompt = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(diagram_prompt)


class DiagramPromptTests(unittest.TestCase):
    def test_example_is_valid_and_compiles_all_lanes(self) -> None:
        spec = diagram_prompt.load_spec(ROOT / "examples" / "text-to-image-model.json")
        self.assertEqual(diagram_prompt.validate_spec(spec), [])
        prompt = diagram_prompt.build_prompt(spec)
        self.assertIn('Lane "TRAINING":', prompt)
        self.assertIn('Lane "GENERATING":', prompt)
        self.assertIn("100M U-NET: this learns", prompt)
        self.assertIn("Every word, arrowhead, doodle, and line must be fully visible", prompt)

    def test_missing_goal_and_lanes_are_rejected(self) -> None:
        errors = diagram_prompt.validate_spec({"title": "A TITLE"})
        self.assertIn("goal must be a non-empty string", errors)
        self.assertIn("lanes must be a non-empty list", errors)

    def test_flow_requires_direction(self) -> None:
        spec = {
            "title": "CACHE",
            "goal": "Explain a cache hit.",
            "lanes": [{"label": "FLOW", "flows": ["request and response"]}],
        }
        errors = diagram_prompt.validate_spec(spec)
        self.assertTrue(any("directional arrow" in error for error in errors))

    def test_cli_writes_compiled_prompt(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "prompt.txt"
            code = diagram_prompt.main(
                ["build", str(ROOT / "examples" / "text-to-image-model.json"), "--output", str(output)]
            )
            self.assertEqual(code, 0)
            self.assertIn("HOW MY TEXT-TO-IMAGE MODEL WORKS", output.read_text(encoding="utf-8"))

    def test_invalid_json_returns_error_code(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "broken.json"
            path.write_text(json.dumps([]), encoding="utf-8")
            self.assertEqual(diagram_prompt.main(["validate", str(path)]), 2)


if __name__ == "__main__":
    unittest.main()
