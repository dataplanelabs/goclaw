import logging
import os
from contextlib import asynccontextmanager
from typing import Annotated, Optional

from fastapi import FastAPI, File, Form, Header, HTTPException, Response, UploadFile
from pythonjsonlogger import jsonlogger

from .clone import InvalidReferenceAudio, preview_reference
from .health import health
from .schemas import (
    ClonePreviewResponse,
    ErrorResponse,
    HealthResponse,
    SynthesizeRequest,
    VoicesResponse,
)
from .synth import (
    RefAudioPathInvalid,
    SynthesisError,
    _Engine,
    synthesize,
)
from .transcode import FFmpegMissing, TranscodeFailed, assert_ffmpeg_available, mime_for
from .voices import list_voices


def _configure_logging() -> None:
    handler = logging.StreamHandler()
    handler.setFormatter(jsonlogger.JsonFormatter("%(asctime)s %(levelname)s %(name)s %(message)s"))
    root = logging.getLogger()
    root.handlers = [handler]
    root.setLevel(os.environ.get("VIENEU_LOG_LEVEL", "INFO"))


@asynccontextmanager
async def lifespan(app: FastAPI):
    _configure_logging()
    assert_ffmpeg_available()
    mode = os.environ.get("VIENEU_MODE", "standard")
    engine = _Engine(mode=mode)  # type: ignore[arg-type]
    app.state.engine = engine
    await engine.load()
    yield


app = FastAPI(
    title="VieNeu-TTS sidecar",
    version="0.1.0",
    lifespan=lifespan,
    responses={400: {"model": ErrorResponse}, 500: {"model": ErrorResponse}},
)


@app.get("/healthz", response_model=HealthResponse)
async def healthz() -> HealthResponse:
    engine: _Engine = app.state.engine
    return health(model_loaded=engine.loaded)


@app.get("/voices", response_model=VoicesResponse)
async def voices() -> VoicesResponse:
    return list_voices()


@app.post("/synthesize")
async def synthesize_route(
    req: SynthesizeRequest,
    x_tenant_id: Annotated[Optional[str], Header()] = None,
) -> Response:
    engine: _Engine = app.state.engine
    try:
        audio, fmt = await synthesize(engine, req, x_tenant_id)
    except RefAudioPathInvalid as exc:
        raise HTTPException(status_code=400, detail=f"ref_audio_path: {exc}")
    except (FFmpegMissing, TranscodeFailed) as exc:
        raise HTTPException(status_code=500, detail=f"transcode: {exc}")
    except SynthesisError as exc:
        raise HTTPException(status_code=500, detail=str(exc))

    return Response(content=audio, media_type=mime_for(fmt))


_CLONE_PREVIEW_MAX_BYTES = 5 * 1024 * 1024


@app.post("/clone-preview", response_model=ClonePreviewResponse)
async def clone_preview_route(
    audio: UploadFile = File(...),
    ref_text: str = Form(...),
) -> ClonePreviewResponse:
    if not ref_text.strip():
        raise HTTPException(status_code=400, detail="ref_text required")

    # Stream-read with a hard byte cap so a hostile client can't OOM the daemon
    # before we get to the post-read size check.
    body = bytearray()
    chunk_size = 64 * 1024
    while True:
        chunk = await audio.read(chunk_size)
        if not chunk:
            break
        body.extend(chunk)
        if len(body) > _CLONE_PREVIEW_MAX_BYTES:
            raise HTTPException(status_code=400, detail="audio body > 5 MB")

    try:
        return preview_reference(bytes(body))
    except InvalidReferenceAudio as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    except FFmpegMissing as exc:  # pragma: no cover
        raise HTTPException(status_code=500, detail=str(exc))
