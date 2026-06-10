from typing import Literal, Optional

from pydantic import BaseModel, Field

AudioFormat = Literal["wav", "mp3", "m4a", "opus"]
Emotion = Literal["natural", "storytelling"]
Mode = Literal["standard", "turbo"]


class SynthesizeRequest(BaseModel):
    text: str = Field(min_length=1, max_length=1500)
    voice_id: Optional[str] = None
    ref_audio_path: Optional[str] = None
    ref_text: Optional[str] = None
    format: AudioFormat = "mp3"
    speed: float = Field(default=1.0, ge=0.5, le=2.0)
    emotion: Emotion = "natural"
    mode: Mode = "standard"


class VoiceOption(BaseModel):
    id: str
    name: str
    language: str = "vi"
    gender: Optional[str] = None
    accent: Optional[str] = None


class VoicesResponse(BaseModel):
    voices: list[VoiceOption]


class HealthResponse(BaseModel):
    status: Literal["ok", "loading", "error"]
    model_loaded: bool
    uptime_s: int


class ClonePreviewResponse(BaseModel):
    valid: bool
    duration_s: float
    sample_rate: int
    channels: int


class ErrorResponse(BaseModel):
    error: str
    detail: Optional[str] = None
