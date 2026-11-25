#!/usr/bin/env python3
"""Posts the notes of a release to the Releases topic of the Telegram group.

Usage: telegram-release.py VERSION NOTES_FILE RELEASE_URL

The bot token comes from the environment (TELEGRAM_BOT_TOKEN), never from the command line. TELEGRAM_CHAT and
TELEGRAM_TOPIC name the group and the topic. DRY_RUN=1 prints the message instead of sending it.

The notes are the version's section of CHANGELOG.md (English). Telegram takes a small part of HTML, so the Markdown of
the changelog is turned into it here: headings become bold lines, **bold** and `code` keep their meaning, list items
start with a bullet and links become <a>. A message holds at most 4096 characters; longer notes are cut at a line and
end with a pointer to the release page.
"""
import html
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

LIMIT = 4096


def inline(text: str) -> str:
    """Converts the inline Markdown of one line to Telegram HTML, escaping everything else."""
    out = []
    # Code spans first, so that what is inside them is not read as Markdown.
    for i, part in enumerate(re.split(r"(`[^`]*`)", text)):
        if i % 2:
            out.append("<code>" + html.escape(part[1:-1], quote=False) + "</code>")
            continue
        part = html.escape(part, quote=False)
        part = re.sub(r"\[([^\]]+)\]\((https?://[^)\s]+)\)", lambda m: f'<a href="{html.escape(m.group(2))}">{m.group(1)}</a>', part)
        part = re.sub(r"\*\*(.+?)\*\*", r"<b>\1</b>", part)
        part = re.sub(r"(?<![\w*])\*(?!\s)(.+?)(?<!\s)\*(?![\w*])", r"<i>\1</i>", part)
        out.append(part)
    return "".join(out)


def to_html(notes: str) -> list:
    """Returns the notes as lines of Telegram HTML."""
    lines = []
    for raw in notes.splitlines():
        line = raw.rstrip()
        if not line.strip():
            if lines and lines[-1] != "":
                lines.append("")
            continue
        m = re.match(r"^#{1,6}\s+(.*)$", line)
        if m:
            if lines and lines[-1] != "":
                lines.append("")
            lines.append("<b>" + inline(m.group(1)) + "</b>")
            continue
        m = re.match(r"^(\s*)[-*+]\s+(.*)$", line)
        if m:
            indent = "  " * (len(m.group(1)) // 2)
            lines.append(indent + "• " + inline(m.group(2)))
            continue
        lines.append(inline(line.strip()))
    while lines and lines[-1] == "":
        lines.pop()
    return lines


def message(version: str, notes: str, url: str) -> str:
    head = f"🚀 <b>Mini Panel v{html.escape(version)}</b>\n\n"
    foot = f'\n\n<a href="{html.escape(url)}">Release on GitHub</a> · <code>mpn update -apply</code>'
    cut = f'\n\n… <a href="{html.escape(url)}">Read the rest on GitHub</a> · <code>mpn update -apply</code>'
    body = to_html(notes)
    text = head + "\n".join(body) + foot
    if len(text) <= LIMIT:
        return text
    # Drop whole lines from the end until the message fits; a tag is never cut in half this way.
    while body and len(head + "\n".join(body) + cut) > LIMIT:
        body.pop()
    while body and body[-1] == "":
        body.pop()
    return head + "\n".join(body) + cut


def send(token: str, chat: str, topic: str, text: str) -> None:
    payload = {"chat_id": chat, "text": text, "parse_mode": "HTML", "link_preview_options": {"is_disabled": True}}
    if topic:
        payload["message_thread_id"] = int(topic)
    data = json.dumps(payload).encode()
    req = urllib.request.Request(f"https://api.telegram.org/bot{token}/sendMessage", data=data, headers={"Content-Type": "application/json"})
    last = None
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                answer = json.load(r)
            if answer.get("ok"):
                print(f"posted message {answer['result']['message_id']}")
                return
            last = answer.get("description")
        except urllib.error.HTTPError as e:
            # Telegram's answer says what is wrong (a bad tag, a missing topic); it never contains the token.
            last = e.read().decode(errors="replace")
            if e.code < 500 and e.code != 429:
                break
        except OSError as e:
            last = str(e)
        time.sleep(5 * (attempt + 1))
    sys.exit(f"telegram: {last}")


def main() -> None:
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    version, notes_file, url = sys.argv[1:]
    with open(notes_file, encoding="utf-8") as f:
        text = message(version, f.read(), url)
    if os.environ.get("DRY_RUN") == "1":
        print(text)
        print(f"\n({len(text)} characters)")
        return
    token = os.environ.get("TELEGRAM_BOT_TOKEN", "")
    if not token:
        print("TELEGRAM_BOT_TOKEN is not set; nothing posted")
        return
    send(token, os.environ.get("TELEGRAM_CHAT", ""), os.environ.get("TELEGRAM_TOPIC", ""), text)


if __name__ == "__main__":
    main()
