"""Voice catalog sourced from the loaded VieNeu SDK at runtime.

The SDK reads voices.json from the model repo on load; IDs and descriptions
live there and change across model revisions. Keeping a parallel hardcoded
list here drifts (caused 'voice not found' errors when SDK used PascalCase
keys like 'Binh' vs our slug 'binh'). Single source of truth: the SDK.
"""

import re
from typing import Any, Optional

from .schemas import VoiceOption, VoicesResponse

_voices: list[VoiceOption] = []

# "Thanh Bình (nam miền Bắc)" → name + gender + accent
_DESC_RE = re.compile(r"^(.*?)\s*\((nam|nữ)\s+miền\s+(Bắc|Nam|Trung)\)\s*$")
_GENDER = {"nam": "male", "nữ": "female"}
_ACCENT = {"Bắc": "north", "Nam": "south", "Trung": "central"}


def _parse(key: str, raw: Any) -> VoiceOption:
    description = raw.get("description", key) if isinstance(raw, dict) else str(raw)
    m = _DESC_RE.match(description)
    if m:
        return VoiceOption(
            id=key,
            name=m.group(1),
            language="vi",
            gender=_GENDER.get(m.group(2)),
            accent=_ACCENT.get(m.group(3)),
        )
    return VoiceOption(id=key, name=description, language="vi")


def reload_from_sdk(tts: Any) -> None:
    """Replace the cached list from the SDK's loaded preset map."""
    global _voices
    presets = getattr(tts, "_preset_voices", None) or {}
    _voices = [_parse(k, v) for k, v in presets.items()]


def list_voices() -> VoicesResponse:
    return VoicesResponse(voices=list(_voices))


def get_voice(voice_id: str) -> Optional[VoiceOption]:
    for v in _voices:
        if v.id == voice_id:
            return v
    return None
