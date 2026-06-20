#!/usr/bin/env python3
"""Post a single, already-approved post to X (Twitter) via the API v2.

Tier 3 of the growth loop's publish path. This is the ONLY step that produces public
output, and it is invoked by hand (the post-to-x workflow_dispatch), never on a
schedule. The text passed in is whatever a human approved in the social queue.

Requires four secrets from an X developer app (OAuth 1.0a user context):
  X_API_KEY, X_API_SECRET, X_ACCESS_TOKEN, X_ACCESS_SECRET

NOTE: this path is unverified by the QCore maintainers — it needs live X credentials
to exercise, which we do not hold. Treat the first real dispatch as the test.
"""
from __future__ import annotations

import os
import sys

REQUIRED = ["X_API_KEY", "X_API_SECRET", "X_ACCESS_TOKEN", "X_ACCESS_SECRET"]


def main() -> int:
    text = sys.stdin.read().strip() if not sys.argv[1:] else " ".join(sys.argv[1:]).strip()
    if not text:
        print("error: empty post text", file=sys.stderr)
        return 2
    if len(text) > 280:
        print(f"error: post is {len(text)} chars (>280). Trim before posting.", file=sys.stderr)
        return 2

    missing = [k for k in REQUIRED if not os.environ.get(k)]
    if missing:
        print("X credentials not configured. Missing secrets: " + ", ".join(missing), file=sys.stderr)
        print("Add them under repo Settings → Secrets and variables → Actions.", file=sys.stderr)
        return 1

    try:
        import requests
        from requests_oauthlib import OAuth1
    except ImportError:
        print("error: pip install requests requests_oauthlib first", file=sys.stderr)
        return 1

    auth = OAuth1(
        os.environ["X_API_KEY"], os.environ["X_API_SECRET"],
        os.environ["X_ACCESS_TOKEN"], os.environ["X_ACCESS_SECRET"],
    )
    resp = requests.post(
        "https://api.twitter.com/2/tweets",
        json={"text": text}, auth=auth, timeout=30,
    )
    if resp.status_code >= 300:
        print(f"X API error {resp.status_code}: {resp.text}", file=sys.stderr)
        return 1
    print("posted:", resp.json())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
