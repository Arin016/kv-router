#!/usr/bin/env python3
"""Validate a diagram spec and compile it into an image-generation prompt."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


class SpecError(ValueError):
    """Raised when a diagram spec is incomplete or malformed."""


def load_spec(path: Path) -> dict[str, Any]:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise SpecError(f"spec file not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise SpecError(f"invalid JSON at line {exc.lineno}, column {exc.colno}: {exc.msg}") from exc
    if not isinstance(payload, dict):
        raise SpecError("the spec root must be a JSON object")
    return payload


def _required_text(value: Any, path: str, errors: list[str]) -> None:
    if not isinstance(value, str) or not value.strip():
        errors.append(f"{path} must be a non-empty string")


def _optional_string_list(spec: dict[str, Any], key: str, errors: list[str]) -> None:
    value = spec.get(key)
    if value is None:
        return
    if not isinstance(value, list) or any(not isinstance(item, str) or not item.strip() for item in value):
        errors.append(f"{key} must be a list of non-empty strings")


def validate_spec(spec: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    _required_text(spec.get("title"), "title", errors)
    _required_text(spec.get("goal"), "goal", errors)

    lanes = spec.get("lanes")
    if not isinstance(lanes, list) or not lanes:
        errors.append("lanes must be a non-empty list")
    else:
        for index, lane in enumerate(lanes):
            prefix = f"lanes[{index}]"
            if not isinstance(lane, dict):
                errors.append(f"{prefix} must be an object")
                continue
            _required_text(lane.get("label"), f"{prefix}.label", errors)
            flows = lane.get("flows")
            if not isinstance(flows, list) or not flows:
                errors.append(f"{prefix}.flows must be a non-empty list")
                continue
            for flow_index, flow in enumerate(flows):
                flow_path = f"{prefix}.flows[{flow_index}]"
                _required_text(flow, flow_path, errors)
                if isinstance(flow, str) and "->" not in flow and "→" not in flow:
                    errors.append(f"{flow_path} must contain a directional arrow (-> or →)")

    for key in ("annotations", "loops", "exact_text", "output_doodles", "avoid"):
        _optional_string_list(spec, key, errors)

    for key in ("audience", "format", "footer"):
        if key in spec and spec[key] is not None:
            _required_text(spec[key], key, errors)

    return errors


def _bullet_block(label: str, items: list[str] | None) -> list[str]:
    if not items:
        return []
    return [f"{label}:", *(f"- {item}" for item in items)]


def build_prompt(spec: dict[str, Any]) -> str:
    errors = validate_spec(spec)
    if errors:
        raise SpecError("\n".join(errors))

    audience = spec.get("audience", "general nontechnical audience")
    output_format = spec.get("format", "16:9 landscape")
    lines = [
        "Use case: scientific-educational",
        f"Asset type: one {output_format} whiteboard explainer image",
        f"Teaching goal: {spec['goal']}",
        f"Audience: {audience}",
        "",
        f'Title, verbatim: "{spec["title"]}"',
        "",
        "Causal structure:",
    ]

    for lane in spec["lanes"]:
        lines.append(f'Lane "{lane["label"]}":')
        lines.extend(f"- {flow}" for flow in lane["flows"])

    sections = (
        ("Annotations", spec.get("annotations")),
        ("Feedback loops", spec.get("loops")),
        ("Required exact text", spec.get("exact_text")),
        ("Literal output doodles", spec.get("output_doodles")),
    )
    for label, items in sections:
        block = _bullet_block(label, items)
        if block:
            lines.extend(("", *block))

    footer = spec.get("footer")
    if footer:
        lines.extend(("", f'Footer takeaway, verbatim: "{footer}"'))

    lines.extend(
        (
            "",
            "Visual direction:",
            "- Authentic loose dry-erase marker sketch on a clean warm-white physical whiteboard.",
            "- Human handwriting, slight pressure variation, imperfect confident arrows, and sparse helpful doodles.",
            "- Black for titles, structure, and primary labels; blue for flow and data; red for one critical or trainable component.",
            "- Use color together with text labels so the meaning does not depend on color alone.",
            "- Use simple rectangles only for real components or boundaries; use plus signs where inputs join.",
            "- Keep one clear left-to-right reading direction within each lane and avoid crossed arrows.",
            "- Preserve at least 7% empty margin on every edge. Every word, arrowhead, doodle, and line must be fully visible.",
            "- Causal accuracy and exact spelling outrank decoration.",
            "",
            "Do not use gradients, glow, shadows, glass, UI cards, icon tiles, pills, a background grid, app chrome, or a watermark.",
        )
    )
    avoid = spec.get("avoid")
    if avoid:
        lines.append("Also avoid: " + "; ".join(avoid) + ".")
    return "\n".join(lines) + "\n"


def create_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    validate = subparsers.add_parser("validate", help="validate a JSON diagram spec")
    validate.add_argument("spec", type=Path)

    build = subparsers.add_parser("build", help="compile a JSON diagram spec into a prompt")
    build.add_argument("spec", type=Path)
    build.add_argument("--output", type=Path, help="write the prompt to this file instead of stdout")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = create_parser().parse_args(argv)
    try:
        spec = load_spec(args.spec)
        errors = validate_spec(spec)
        if errors:
            raise SpecError("\n".join(errors))
        if args.command == "validate":
            print(f"valid: {args.spec}")
            return 0

        prompt = build_prompt(spec)
        if args.output:
            args.output.write_text(prompt, encoding="utf-8")
            print(f"wrote: {args.output}")
        else:
            sys.stdout.write(prompt)
        return 0
    except (OSError, SpecError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
