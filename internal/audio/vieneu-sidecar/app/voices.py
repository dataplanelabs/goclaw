from .schemas import VoiceOption, VoicesResponse

# Preset voices documented by VieNeu-TTS (HF model card pnnbao-ump/VieNeu-TTS-v2).
# IDs use lowercase ASCII slugs to match the canonical Voice.ID convention in
# `internal/audio/voices.go`; the display Name keeps the original Vietnamese spelling.
_PRESET_VOICES: list[VoiceOption] = [
    VoiceOption(id="truc_ly", name="Trúc Ly", language="vi", gender="female", accent="north"),
    VoiceOption(id="binh", name="Bình", language="vi", gender="male", accent="north"),
    VoiceOption(id="tuyen", name="Tuyên", language="vi", gender="male", accent="south"),
    VoiceOption(id="nguyen", name="Nguyên", language="vi", gender="male", accent="north"),
    VoiceOption(id="huong", name="Hương", language="vi", gender="female", accent="north"),
    VoiceOption(id="ngoc", name="Ngọc", language="vi", gender="female", accent="north"),
    VoiceOption(id="doan", name="Đoan", language="vi", gender="female", accent="central"),
]


def list_voices() -> VoicesResponse:
    return VoicesResponse(voices=list(_PRESET_VOICES))


def get_voice(voice_id: str) -> VoiceOption | None:
    for v in _PRESET_VOICES:
        if v.id == voice_id:
            return v
    return None
