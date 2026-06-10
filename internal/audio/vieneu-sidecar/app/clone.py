import io

import soundfile as sf

from .schemas import ClonePreviewResponse


class InvalidReferenceAudio(ValueError):
    """Reference WAV failed validation."""


_MIN_DURATION_S = 3.0
_MAX_DURATION_S = 6.0
_MIN_SAMPLE_RATE = 16000
_MAX_CHANNELS = 2


def preview_reference(audio_bytes: bytes) -> ClonePreviewResponse:
    try:
        info = sf.info(io.BytesIO(audio_bytes))
    except Exception as exc:
        raise InvalidReferenceAudio(f"unreadable audio: {exc}") from exc

    duration = float(info.frames) / float(info.samplerate)
    if not (_MIN_DURATION_S <= duration <= _MAX_DURATION_S):
        raise InvalidReferenceAudio(
            f"duration {duration:.2f}s outside {_MIN_DURATION_S}-{_MAX_DURATION_S}s"
        )
    if info.samplerate < _MIN_SAMPLE_RATE:
        raise InvalidReferenceAudio(f"sample rate {info.samplerate} < {_MIN_SAMPLE_RATE}")
    if info.channels > _MAX_CHANNELS:
        raise InvalidReferenceAudio(f"channels {info.channels} > {_MAX_CHANNELS}")

    return ClonePreviewResponse(
        valid=True,
        duration_s=duration,
        sample_rate=info.samplerate,
        channels=info.channels,
    )
