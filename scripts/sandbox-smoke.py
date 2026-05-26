#!/usr/bin/env python3
"""Sandbox runtime smoke test.

Exercises native-library bindings that have failed in production traces.
Fails fast with a non-zero exit code so CI catches sandbox image
regressions before they hit agents.

Run from the built sandbox image:
    docker run --rm goclaw-sandbox python3 /usr/local/bin/sandbox-smoke.py
"""

from __future__ import annotations

import io
import sys
import traceback


def check_pillow_native() -> None:
    """Trace 019e62ff: `from PIL import _imaging` failed with ImportError.

    Verify Pillow's C extension loads AND can encode/decode common formats
    (jpeg/png/webp/tiff/freetype). These are the formats the design and
    create_image flows depend on.
    """
    from PIL import Image, ImageDraw, ImageFont

    img = Image.new("RGB", (32, 32), color=(20, 60, 200))
    draw = ImageDraw.Draw(img)
    # Default font path covers the freetype native dep.
    draw.text((2, 2), "ok", fill=(255, 255, 255))

    for fmt in ("PNG", "JPEG", "WEBP", "TIFF"):
        buf = io.BytesIO()
        img.save(buf, format=fmt)
        if buf.tell() == 0:
            raise RuntimeError(f"Pillow {fmt} encode produced 0 bytes")
        buf.seek(0)
        decoded = Image.open(buf)
        decoded.load()
        if decoded.size != (32, 32):
            raise RuntimeError(f"Pillow {fmt} round-trip size mismatch: {decoded.size}")


CHECKS = [
    ("pillow-native", check_pillow_native),
]


def main() -> int:
    failures: list[tuple[str, str]] = []
    for name, check in CHECKS:
        try:
            check()
            print(f"  [OK] {name}")
        except Exception as exc:  # noqa: BLE001 — surface anything
            failures.append((name, f"{exc.__class__.__name__}: {exc}"))
            print(f"  [FAIL] {name}: {exc}", file=sys.stderr)
            traceback.print_exc()

    if failures:
        print(f"\n{len(failures)} sandbox smoke check(s) failed:", file=sys.stderr)
        for name, msg in failures:
            print(f"  - {name}: {msg}", file=sys.stderr)
        return 1

    print(f"\nAll {len(CHECKS)} sandbox smoke check(s) passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
