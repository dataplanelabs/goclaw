import time

from .schemas import HealthResponse

_started_at = time.monotonic()


def health(model_loaded: bool) -> HealthResponse:
    return HealthResponse(
        status="ok" if model_loaded else "loading",
        model_loaded=model_loaded,
        uptime_s=int(time.monotonic() - _started_at),
    )
