import asyncio
import shutil
from typing import Literal

OutputFormat = Literal["wav", "mp3", "m4a", "opus"]

_FFMPEG_ARGS: dict[str, list[str]] = {
    "mp3": ["-f", "mp3", "-b:a", "64k", "-ac", "1", "-ar", "24000"],
    "m4a": ["-f", "ipod", "-c:a", "aac", "-b:a", "64k", "-ac", "1", "-ar", "16000"],
    "opus": ["-f", "ogg", "-c:a", "libopus", "-b:a", "32k", "-ac", "1", "-ar", "16000"],
}


class FFmpegMissing(RuntimeError):
    """ffmpeg binary unavailable on PATH."""


class TranscodeFailed(RuntimeError):
    """ffmpeg returned non-zero."""


def assert_ffmpeg_available() -> None:
    if shutil.which("ffmpeg") is None:
        raise FFmpegMissing("ffmpeg binary not found on PATH")


async def transcode_wav(wav_bytes: bytes, target: OutputFormat) -> bytes:
    if target == "wav":
        return wav_bytes
    args = _FFMPEG_ARGS.get(target)
    if args is None:
        raise TranscodeFailed(f"unsupported target format: {target}")

    proc = await asyncio.create_subprocess_exec(
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
        "-i",
        "pipe:0",
        *args,
        "pipe:1",
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, stderr = await proc.communicate(input=wav_bytes)
    if proc.returncode != 0:
        raise TranscodeFailed(stderr.decode("utf-8", "ignore"))
    return stdout


def mime_for(fmt: OutputFormat) -> str:
    return {
        "wav": "audio/wav",
        "mp3": "audio/mpeg",
        "m4a": "audio/mp4",
        "opus": "audio/ogg",
    }[fmt]
