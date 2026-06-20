#!/usr/bin/env python3
"""Upsert a tester outcome from a GitHub issue-form report into the adoption tracker.

Closes the "record every outcome" step of the loop: when someone files a Real RAN
report / Attach failure issue, this parses the rendered form body and writes (or
updates) one row in docs/adoption-tracker.csv, keyed by the issue URL so re-runs and
later edits are idempotent.

It records only product evidence + the public issue link — no private contact data,
per docs/adoption-plan.md. All free-text cells are sanitized against CSV / formula
injection because the body is untrusted external input.
"""
from __future__ import annotations

import argparse
import csv
import os
import re
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]

COLUMNS = [
    "date", "tester", "setup", "source", "reached", "cloned", "first_connection",
    "ttfc_s", "first_diagnosis", "ttrc_s", "diagnosis_nailed_it", "returned",
    "advocated", "top_friction", "action_taken", "evidence",
]


def parse_form(body: str) -> dict[str, str]:
    """Split a GitHub issue-form body (`### Label\\n\\nvalue`) into {label: value}."""
    sections: dict[str, str] = {}
    label = None
    buf: list[str] = []
    for line in body.splitlines():
        if line.startswith("### "):
            if label is not None:
                sections[label] = "\n".join(buf).strip()
            label = line[4:].strip().lower()
            buf = []
        elif label is not None:
            buf.append(line)
    if label is not None:
        sections[label] = "\n".join(buf).strip()
    # GitHub writes "_No response_" for skipped optional fields.
    return {k: ("" if v.strip() == "_No response_" else v) for k, v in sections.items()}


def sanitize(value: str, limit: int = 140) -> str:
    """One-line, length-capped, formula-injection-safe cell."""
    s = re.sub(r"\s+", " ", value).strip()[:limit]
    if s and s[0] in "=+-@":
        s = "'" + s
    return s


def first_number(value: str) -> str:
    m = re.search(r"\d+(?:\.\d+)?", value)
    return m.group(0) if m else ""


def yes_if(cond: bool) -> str:
    return "yes" if cond else ""


def build_row(sections: dict[str, str], *, created: str, number: str, url: str, labels: set[str]) -> dict[str, str]:
    ttfc = first_number(sections.get("ttfc", ""))
    ttrc = first_number(sections.get("ttrc", ""))
    if "real-ran" in labels:
        setup = "real-ran " + sanitize(sections.get("mode", ""), 8)
    elif "attach-failure" in labels:
        setup = "sim " + sanitize(sections.get("mode", ""), 8)
    else:
        setup = sanitize(sections.get("mode", ""), 16) or "other"
    friction = sections.get("where reality diverged") or sections.get("what happened") \
        or sections.get("what went wrong") or ""
    return {
        "date": created,
        "tester": f"issue-#{number}",  # anonymous slot; public link in `evidence`
        "setup": setup.strip(),
        "source": "github-issue",
        "reached": "yes",
        "cloned": "yes",
        "first_connection": yes_if(bool(ttfc)),
        "ttfc_s": ttfc,
        "first_diagnosis": yes_if(bool(ttrc)),
        "ttrc_s": ttrc,
        "diagnosis_nailed_it": "",  # human judges from the report
        "returned": "",
        "advocated": "",
        "top_friction": sanitize(friction),
        "action_taken": "",
        "evidence": url,
    }


def upsert(tracker: Path, row: dict[str, str]) -> str:
    rows: list[dict[str, str]] = []
    if tracker.exists():
        with tracker.open(newline="") as fh:
            rows = list(csv.DictReader(fh))
    for i, existing in enumerate(rows):
        if existing.get("evidence") == row["evidence"]:
            rows[i] = row
            action = "updated"
            break
    else:
        rows.append(row)
        action = "added"
    with tracker.open("w", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=COLUMNS)
        w.writeheader()
        for r in rows:
            w.writerow({c: r.get(c, "") for c in COLUMNS})
    return action


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--tracker", default=str(REPO / "docs/adoption-tracker.csv"))
    ap.add_argument("--number", default=os.environ.get("ISSUE_NUMBER", ""))
    ap.add_argument("--url", default=os.environ.get("ISSUE_URL", ""))
    ap.add_argument("--created", default=os.environ.get("ISSUE_CREATED", ""))
    ap.add_argument("--labels", default=os.environ.get("ISSUE_LABELS", ""))
    args = ap.parse_args()

    body = os.environ.get("ISSUE_BODY", "")
    labels = {l.strip() for l in args.labels.split(",") if l.strip()}
    adoption_labels = {"real-ran", "attach-failure", "diagnosis-gap"}
    if not labels & adoption_labels:
        print("not an adoption report (labels:", labels, ") — skipping")
        return 0

    created = (args.created or "")[:10]
    row = build_row(parse_form(body), created=created, number=args.number, url=args.url, labels=labels)
    action = upsert(Path(args.tracker), row)
    print(f"tracker row {action}: {row['tester']} ({row['setup']}) -> {row['evidence']}")
    # Expose the action to the workflow.
    if gh_out := os.environ.get("GITHUB_OUTPUT"):
        with open(gh_out, "a") as fh:
            fh.write(f"action={action}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
