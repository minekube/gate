#!/usr/bin/env python3
"""Forward Gate's console output to a Discord webhook.

Gate logs to stderr; nothing posts it anywhere. This reads that stream and
relays it to a Discord webhook, batching lines into code-blocked messages so a
busy proxy does not hit Discord's webhook rate limit (5 requests per 2 seconds
per webhook, 2000 characters per message).

Usage:
    ./gate 2>&1 | python3 discord-log-forward.py https://discord.com/api/webhooks/...

Environment:
    DISCORD_WEBHOOK_URL    webhook URL if no argument is given
    DISCORD_LOG_LEVEL      only forward lines containing this text (e.g. ERROR)
    DISCORD_LOG_MAX_LINES  buffered lines before the oldest are dropped (default 500)

Notes:
    - Output is buffered, not streamed line by line: one message per burst.
    - A failed post never kills the pipeline; Gate keeps running and the error is
      printed on stderr.
    - Lines are truncated per message; nothing is re-ordered.
"""

from __future__ import annotations

import json
import os
import queue
import sys
import threading
import time
import urllib.error
import urllib.request

# Discord allows 5 webhook requests per 2 seconds; stay just under it.
MIN_SPACING_S = 0.45
# Flush a partial batch after this much quiet, so a single line still arrives.
IDLE_FLUSH_S = 1.0
# Leave room for the ``` fences inside the 2000-character message limit.
MAX_CONTENT = 1900
MAX_MESSAGES_PER_FLUSH = 4


def _reader(stream, out: queue.Queue) -> None:
    try:
        for line in stream:
            out.put(line.rstrip("\n"))
    finally:
        out.put(None)


def _sanitize(line: str) -> str:
    # A line must not be able to close the code block and inject markdown.
    return line.replace("```", "'''").replace("\r", "")


def _post(url: str, content: str) -> None:
    body = json.dumps(
        {
            "content": content,
            # Never ping: @everyone inside a kicked player name must stay text.
            "allowed_mentions": {"parse": []},
        }
    ).encode()
    request = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json", "User-Agent": "gate-log-forward/1"},
        method="POST",
    )
    for attempt in (1, 2):
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                response.read()
            return
        except urllib.error.HTTPError as err:
            if err.code == 429 and attempt == 1:
                retry_after = 1.0
                try:
                    retry_after = float(json.loads(err.read() or b"{}").get("retry_after", 1.0))
                except Exception:
                    pass
                time.sleep(min(max(retry_after, 0.2), 30.0))
                continue
            if err.code >= 500 and attempt == 1:
                time.sleep(1.0)
                continue
            raise
        except OSError:
            if attempt == 1:
                time.sleep(1.0)
                continue
            raise


def main() -> int:
    url = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("DISCORD_WEBHOOK_URL", "")
    if not url.startswith("https://discord.com/api/webhooks/"):
        print("usage: gate 2>&1 | discord-log-forward.py https://discord.com/api/webhooks/...", file=sys.stderr)
        return 2
    level = os.environ.get("DISCORD_LOG_LEVEL", "").upper()
    max_lines = max(1, int(os.environ.get("DISCORD_LOG_MAX_LINES", "500")))

    lines: queue.Queue[str | None] = queue.Queue()
    threading.Thread(target=_reader, args=(sys.stdin, lines), daemon=True).start()

    pending: list[str] = []
    chars = 0
    dropped = 0
    eof = False
    last_post = 0.0
    last_line = time.monotonic()

    def flush() -> None:
        nonlocal pending, chars, dropped, last_post
        sent = 0
        while pending and sent < MAX_MESSAGES_PER_FLUSH:
            chunk: list[str] = []
            size = 0
            while pending and size + len(pending[0]) + 1 <= MAX_CONTENT:
                line = pending.pop(0)
                chunk.append(line)
                size += len(line) + 1
            if not chunk:
                chunk = [pending.pop(0)[:MAX_CONTENT]]
            if dropped:
                chunk.insert(0, f"[{dropped} earlier lines dropped: buffer full]")
                dropped = 0
            time.sleep(max(0.0, MIN_SPACING_S - (time.monotonic() - last_post)))
            try:
                _post(url, "```\n" + "\n".join(chunk) + "\n```")
            except Exception as err:  # noqa: BLE001 - never kill the pipeline
                print(f"discord-log-forward: post failed: {err}", file=sys.stderr)
            last_post = time.monotonic()
            sent += 1
        chars = sum(len(line) + 1 for line in pending)

    while True:
        timeout = min(IDLE_FLUSH_S, MIN_SPACING_S)
        try:
            line = lines.get(timeout=timeout)
            if line is None:
                eof = True
            elif not level or level in line:
                pending.append(_sanitize(line))
                chars += len(line) + 1
                last_line = time.monotonic()
                while len(pending) > max_lines:
                    pending.pop(0)
                    dropped += 1
        except queue.Empty:
            pass

        if not pending:
            if eof:
                return 0
            continue

        now = time.monotonic()
        ready = now - last_post >= MIN_SPACING_S
        saturated = chars >= MAX_CONTENT
        idle = now - last_line >= IDLE_FLUSH_S
        if ready and (saturated or idle or eof):
            flush()
        if eof and not pending:
            return 0


if __name__ == "__main__":
    raise SystemExit(main())
