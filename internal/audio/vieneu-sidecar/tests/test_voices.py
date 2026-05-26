from types import SimpleNamespace

from app.voices import get_voice, list_voices, reload_from_sdk


def _fake_sdk(presets: dict) -> SimpleNamespace:
    return SimpleNamespace(_preset_voices=presets)


def test_list_empty_before_sdk_load():
    reload_from_sdk(_fake_sdk({}))
    assert list_voices().voices == []


def test_reload_parses_description():
    reload_from_sdk(_fake_sdk({
        "Binh": {"description": "Thanh Bình (nam miền Bắc)"},
        "Doan": {"description": "Thục Đoan (nữ miền Nam)"},
    }))
    voices = {v.id: v for v in list_voices().voices}
    assert voices["Binh"].name == "Thanh Bình"
    assert voices["Binh"].gender == "male"
    assert voices["Binh"].accent == "north"
    assert voices["Doan"].gender == "female"
    assert voices["Doan"].accent == "south"


def test_reload_fallback_when_no_parseable_description():
    reload_from_sdk(_fake_sdk({"X": {"description": "weird format"}}))
    v = get_voice("X")
    assert v is not None
    assert v.name == "weird format"
    assert v.gender is None


def test_get_voice_unknown_returns_none():
    reload_from_sdk(_fake_sdk({"Ly": {"description": "Trúc Ly (nữ miền Bắc)"}}))
    assert get_voice("does_not_exist") is None
