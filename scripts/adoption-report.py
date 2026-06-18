#!/usr/bin/env python3
"""Summarize QCore's self-serve adoption funnel from docs/adoption-tracker.csv."""

from __future__ import annotations

import argparse
import csv
import json
import statistics
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_TRACKER = ROOT / "docs" / "adoption-tracker.csv"


def truthy(value: str | None) -> bool:
    return str(value or "").strip().lower() in {"1", "true", "yes", "y", "ok", "pass", "passed", "done"}


def number(value: str | None) -> float | None:
    try:
        text = str(value or "").strip()
        if not text:
            return None
        return float(text)
    except ValueError:
        return None


def median(values: Iterable[float]) -> float | None:
    clean = [v for v in values if v is not None]
    if not clean:
        return None
    return float(statistics.median(clean))


def load_rows(path: Path) -> list[dict[str, str]]:
    if not path.exists():
        return []
    with path.open(newline="", encoding="utf-8") as f:
        return list(csv.DictReader(f))


def summarize(rows: list[dict[str, str]]) -> dict[str, object]:
    ttfc = [n for row in rows if (n := number(row.get("ttfc_s"))) is not None]
    ttrc = [n for row in rows if (n := number(row.get("ttrc_s"))) is not None]
    funnel = {
        "reached": sum(truthy(row.get("reached")) for row in rows),
        "cloned": sum(truthy(row.get("cloned")) for row in rows),
        "first_connection": sum(truthy(row.get("first_connection")) for row in rows),
        "first_diagnosis": sum(truthy(row.get("first_diagnosis")) for row in rows),
        "returned": sum(truthy(row.get("returned")) for row in rows),
        "advocated": sum(truthy(row.get("advocated")) for row in rows),
    }
    setup_counts: dict[str, int] = {}
    friction_counts: dict[str, int] = {}
    for row in rows:
        setup = (row.get("setup") or "unknown").strip() or "unknown"
        setup_counts[setup] = setup_counts.get(setup, 0) + 1
        friction = (row.get("top_friction") or "").strip()
        if friction:
            friction_counts[friction] = friction_counts.get(friction, 0) + 1

    return {
        "schema": "qcore.adoption_report.v1",
        "total_rows": len(rows),
        "funnel": funnel,
        "median_ttfc_s": median(ttfc),
        "median_ttrc_s": median(ttrc),
        "setup_counts": setup_counts,
        "top_friction": sorted(
            [{"friction": key, "count": value} for key, value in friction_counts.items()],
            key=lambda item: (-item["count"], item["friction"]),
        )[:10],
    }


def render_markdown(summary: dict[str, object]) -> str:
    funnel = summary["funnel"]
    assert isinstance(funnel, dict)
    ttfc = f"{summary['median_ttfc_s']}s" if summary["median_ttfc_s"] is not None else "-"
    ttrc = f"{summary['median_ttrc_s']}s" if summary["median_ttrc_s"] is not None else "-"
    lines = [
        "# QCore Adoption Report",
        "",
        f"- Tracker rows: {summary['total_rows']}",
        f"- Median TTFC: {ttfc}",
        f"- Median TTRC: {ttrc}",
        "",
        "## Funnel",
        "",
        "| Stage | Count |",
        "|---|---:|",
    ]
    for stage in ("reached", "cloned", "first_connection", "first_diagnosis", "returned", "advocated"):
        lines.append(f"| {stage.replace('_', ' ').title()} | {funnel.get(stage, 0)} |")

    setup_counts = summary["setup_counts"]
    assert isinstance(setup_counts, dict)
    lines += ["", "## Setup Mix", "", "| Setup | Count |", "|---|---:|"]
    if setup_counts:
        for setup, count in sorted(setup_counts.items()):
            lines.append(f"| {setup} | {count} |")
    else:
        lines.append("| - | 0 |")

    top_friction = summary["top_friction"]
    assert isinstance(top_friction, list)
    lines += ["", "## Top Friction", "", "| Friction | Count |", "|---|---:|"]
    if top_friction:
        for item in top_friction:
            lines.append(f"| {item['friction']} | {item['count']} |")
    else:
        lines.append("| - | 0 |")

    lines += [
        "",
        "## Next Operating Question",
        "",
        "What is the single biggest external-user friction point we can remove before recruiting the next batch?",
        "",
    ]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tracker", default=str(DEFAULT_TRACKER))
    parser.add_argument("--output", default="")
    parser.add_argument("--json-output", default="")
    args = parser.parse_args()

    rows = load_rows(Path(args.tracker))
    summary = summarize(rows)
    markdown = render_markdown(summary)
    print(markdown)

    if args.output:
        out = Path(args.output)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(markdown + "\n", encoding="utf-8")
    if args.json_output:
        out = Path(args.json_output)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
