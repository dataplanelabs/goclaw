import pytest

from app.transcode import TranscodeFailed, mime_for, transcode_wav


def test_mime_for_known_formats():
    assert mime_for("wav") == "audio/wav"
    assert mime_for("mp3") == "audio/mpeg"
    assert mime_for("m4a") == "audio/mp4"
    assert mime_for("opus") == "audio/ogg"


async def test_transcode_wav_passthrough():
    out = await transcode_wav(b"RIFF\x00\x00\x00\x00WAVE", "wav")
    assert out == b"RIFF\x00\x00\x00\x00WAVE"


async def test_transcode_invalid_target_raises():
    with pytest.raises(TranscodeFailed, match="unsupported"):
        await transcode_wav(b"", "flac")  # type: ignore[arg-type]
