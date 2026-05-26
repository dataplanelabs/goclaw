import os
from pathlib import Path

import pytest

from app.synth import RefAudioPathInvalid, validate_ref_audio_path


@pytest.fixture
def refs_root(tmp_path, monkeypatch):
    monkeypatch.setenv("VIENEU_REFS_ROOT", str(tmp_path))
    return tmp_path


def _make_ref(root: Path, tenant: str, name: str) -> Path:
    p = root / tenant / name
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_bytes(b"RIFF")  # not a real WAV; we only test path resolution
    return p


def test_valid_tenant_scoped_path(refs_root):
    p = _make_ref(refs_root, "tenant-a", "voice.wav")
    out = validate_ref_audio_path(str(p), tenant_id="tenant-a")
    assert out == p.resolve()


def test_cross_tenant_rejected(refs_root):
    p = _make_ref(refs_root, "tenant-a", "voice.wav")
    with pytest.raises(RefAudioPathInvalid, match="not under tenant subdir"):
        validate_ref_audio_path(str(p), tenant_id="tenant-b")


def test_path_outside_refs_root_rejected(tmp_path, monkeypatch):
    monkeypatch.setenv("VIENEU_REFS_ROOT", str(tmp_path / "refs"))
    outside = tmp_path / "elsewhere" / "voice.wav"
    outside.parent.mkdir(parents=True, exist_ok=True)
    outside.write_bytes(b"RIFF")
    with pytest.raises(RefAudioPathInvalid, match="not under VIENEU_REFS_ROOT"):
        validate_ref_audio_path(str(outside), tenant_id="tenant-a")


def test_traversal_attempt_rejected(refs_root):
    _make_ref(refs_root, "tenant-a", "voice.wav")
    traversal = str(refs_root / "tenant-a" / ".." / "tenant-b" / "voice.wav")
    with pytest.raises(RefAudioPathInvalid):
        validate_ref_audio_path(traversal, tenant_id="tenant-a")


def test_missing_env_rejected(tmp_path, monkeypatch):
    monkeypatch.delenv("VIENEU_REFS_ROOT", raising=False)
    with pytest.raises(RefAudioPathInvalid, match="VIENEU_REFS_ROOT env not configured"):
        validate_ref_audio_path(str(tmp_path / "x.wav"), tenant_id="t")


def test_empty_path_rejected():
    with pytest.raises(RefAudioPathInvalid, match="empty"):
        validate_ref_audio_path("", tenant_id="t")
