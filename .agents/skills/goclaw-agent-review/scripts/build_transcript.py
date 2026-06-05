#!/usr/bin/env python3
"""Convert session JSONL (one message obj per line) into a readable transcript."""
import json, sys, re, glob, os

RAW = os.path.join(os.path.dirname(__file__), "raw", "sessions")
OUT = os.path.join(os.path.dirname(__file__), "transcripts")
os.makedirs(OUT, exist_ok=True)

def clean_ts(ts):
    if not ts: return "??"
    return ts.replace("T", " ")[:19]

def trunc(s, n):
    s = s or ""
    if len(s) <= n: return s
    return s[:n] + f" …[+{len(s)-n} chars]"

def fmt_msg(m):
    role = m.get("role")
    ts = clean_ts(m.get("created_at"))
    if role == "user":
        c = m.get("content", "")
        # extract sender
        sender = "?"
        msend = re.search(r"\[From: ([^\]]+)\]", c)
        if msend: sender = msend.group(1)
        # strip the metadata prefix lines but keep reply context note
        reply = ""
        mr = re.search(r"\[Replying to your message: \"(.*?)\"\]", c, re.S)
        if mr: reply = " (reply→ \"" + trunc(mr.group(1).replace("\n"," "), 80) + "\")"
        # remove the [ts][From][Replying] prefixes to get actual text
        text = re.sub(r"^\[.*?\]\n?", "", c)
        text = re.sub(r"\[From: [^\]]+\]\n?", "", text)
        text = re.sub(r"\[Replying to your message: \".*?\"\]\n?", "", text, flags=re.S)
        return f"[{ts}] 👤 {sender}{reply}:\n  {text.strip()}\n"
    if role == "assistant":
        parts = []
        c = (m.get("content") or "").strip()
        if c:
            parts.append(f"[{ts}] 🤖 Mai Hà Lan:\n  {c}")
        for tc in (m.get("tool_calls") or []):
            args = tc.get("arguments", {})
            args_s = json.dumps(args, ensure_ascii=False)
            parts.append(f"[{ts}] 🔧 → {tc.get('name')}({trunc(args_s,300)})")
        if not c and not (m.get("tool_calls")):
            parts.append(f"[{ts}] 🤖 (empty)")
        return "\n".join(parts) + "\n"
    if role == "tool":
        c = m.get("content", "")
        # strip security wrapper
        mm = re.search(r"<<<EXTERNAL_UNTRUSTED_CONTENT>>>(.*?)<<<END_EXTERNAL", c, re.S)
        body = mm.group(1) if mm else c
        return f"[{ts}] 📥 tool_result: {trunc(body.strip(), 400)}\n"
    return f"[{ts}] {role}: {trunc(json.dumps(m, ensure_ascii=False), 200)}\n"

for path in sorted(glob.glob(os.path.join(RAW, "*.jsonl"))):
    name = os.path.basename(path).replace(".jsonl", ".txt")
    out_lines = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line: continue
            try:
                m = json.loads(line)
            except Exception as e:
                out_lines.append(f"[PARSE ERROR] {trunc(line,120)}")
                continue
            out_lines.append(fmt_msg(m))
    with open(os.path.join(OUT, name), "w") as f:
        f.write("\n".join(out_lines))
    print(f"{name}: {len(out_lines)} msgs -> {os.path.getsize(os.path.join(OUT,name))} bytes")
