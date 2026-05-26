import asyncio
import logging
import os
import time
from pathlib import Path
from typing import Any, Optional

from .schemas import Emotion, Mode, SynthesizeRequest
from .transcode import OutputFormat, transcode_wav
from .voices import get_voice

logger = logging.getLogger("vieneu_sidecar")


class RefAudioPathInvalid(ValueError):
    """ref_audio_path failed prefix or tenant-subdir validation."""


class SynthesisError(RuntimeError):
    """Wrap inference failures from the vieneu SDK."""


class _Engine:
    """Thin wrapper that lazy-loads the vieneu SDK and serializes inference.

    The vieneu SDK loads a ~500MB ONNX model the first time it's instantiated.
    We do that once in `lifespan`, then guard every `infer()` call with the
    lock so the model state doesn't tear across concurrent requests.
    """

    def __init__(self, mode: Mode = "standard") -> None:
        self._mode = mode
        self._tts: Any = None
        self._lock = asyncio.Lock()
        self._loaded = asyncio.Event()

    @property
    def loaded(self) -> bool:
        return self._loaded.is_set()

    async def load(self) -> None:
        # Imported lazily so unit tests can run without the heavy dep installed.
        from vieneu import Vieneu  # type: ignore[import-not-found]

        # Vieneu() is CPU-bound and blocking; offload to a thread.
        self._tts = await asyncio.to_thread(Vieneu, mode=self._mode)
        self._loaded.set()
        logger.info("vieneu_sidecar: model loaded", extra={"mode": self._mode})

    async def infer(
        self,
        text: str,
        voice_id: Optional[str],
        ref_audio_path: Optional[str],
        ref_text: Optional[str],
        speed: float,
        emotion: Emotion,
    ) -> bytes:
        if not self.loaded:
            raise SynthesisError("model not loaded")

        wait_start = time.monotonic()
        async with self._lock:
            lock_wait_ms = int((time.monotonic() - wait_start) * 1000)
            t0 = time.monotonic()

            kwargs: dict[str, Any] = {"text": text, "speed": speed, "emotion": emotion}
            if ref_audio_path:
                kwargs["ref_audio"] = ref_audio_path
                kwargs["ref_text"] = ref_text
            elif voice_id:
                preset = get_voice(voice_id)
                if preset is None:
                    raise SynthesisError(f"unknown voice_id: {voice_id}")
                try:
                    kwargs["voice"] = self._tts.get_preset_voice(preset.id)
                except (ValueError, KeyError) as exc:
                    raise SynthesisError(f"vieneu preset lookup failed for {preset.id!r}: {exc}") from exc

            try:
                wav = await asyncio.to_thread(self._tts.infer, **kwargs)
            except Exception as exc:
                raise SynthesisError(f"vieneu infer failed: {exc}") from exc

            synth_ms = int((time.monotonic() - t0) * 1000)
            logger.info(
                "vieneu_sidecar.synth",
                extra={
                    "synth_ms": synth_ms,
                    "lock_wait_ms": lock_wait_ms,
                    "text_len": len(text),
                    "cloned": ref_audio_path is not None,
                    "voice_id": voice_id,
                },
            )
            return wav


def validate_ref_audio_path(path: str, tenant_id: Optional[str]) -> Path:
    """Enforce that path is under the workspace's vieneu-refs subdirectory
    and (if X-Tenant-ID was supplied) under the requesting tenant's subdir.
    """
    if not path:
        raise RefAudioPathInvalid("ref_audio_path is empty")

    root_env = os.environ.get("VIENEU_REFS_ROOT")
    if not root_env:
        raise RefAudioPathInvalid("VIENEU_REFS_ROOT env not configured")
    root = Path(root_env).resolve()

    try:
        resolved = Path(path).resolve(strict=False)
    except OSError as exc:
        raise RefAudioPathInvalid(f"path resolution failed: {exc}") from exc

    try:
        rel = resolved.relative_to(root)
    except ValueError as exc:
        raise RefAudioPathInvalid(
            f"path {resolved} not under VIENEU_REFS_ROOT={root}"
        ) from exc

    if tenant_id:
        parts = rel.parts
        if not parts or parts[0] != tenant_id:
            raise RefAudioPathInvalid(
                f"path {resolved} not under tenant subdir {tenant_id}"
            )
    return resolved


async def synthesize(
    engine: _Engine,
    req: SynthesizeRequest,
    tenant_id: Optional[str],
) -> tuple[bytes, OutputFormat]:
    if req.ref_audio_path:
        if not req.ref_text:
            raise SynthesisError("ref_text required when ref_audio_path set")
        validate_ref_audio_path(req.ref_audio_path, tenant_id)

    wav = await engine.infer(
        text=req.text,
        voice_id=req.voice_id,
        ref_audio_path=req.ref_audio_path,
        ref_text=req.ref_text,
        speed=req.speed,
        emotion=req.emotion,
    )
    out = await transcode_wav(wav, req.format)
    return out, req.format
