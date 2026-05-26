from app.voices import get_voice, list_voices


def test_list_voices_has_truc_ly_default():
    out = list_voices()
    ids = [v.id for v in out.voices]
    assert "truc_ly" in ids
    assert len(out.voices) >= 6


def test_list_voices_ids_are_lowercase_snake():
    for v in list_voices().voices:
        assert v.id.islower(), f"voice {v.id} not lowercase"
        assert " " not in v.id, f"voice {v.id} contains space"


def test_get_voice_known():
    v = get_voice("binh")
    assert v is not None
    assert v.name == "Bình"
    assert v.language == "vi"


def test_get_voice_unknown_returns_none():
    assert get_voice("does_not_exist") is None
