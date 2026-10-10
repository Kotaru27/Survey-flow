from survey_flow.memory import TerminateMemory


def test_mark_and_avoid(tmp_path):
    path = tmp_path / "mem.json"
    m = TerminateMemory(str(path))
    m.mark_end(
        "q1", "Are you employed?",
        [{"label": "No", "value": "1", "identity": "l:no"}],
        "terminate", "http://x/test", "you do not qualify",
    )
    assert m.is_fatal("q1", "l:no", label="No", value="1")
    assert not m.is_fatal("q1", "l:yes", label="Yes", value="2")
    assert m.stats["terminates"] == 1


def test_persistence_roundtrip(tmp_path):
    path = tmp_path / "mem.json"
    m = TerminateMemory(str(path))
    m.mark_end("q1", "text", [{"label": "No", "value": "1"}], "terminate")
    m.bump("completes", 2)
    m.save()

    m2 = TerminateMemory(str(path))
    assert m2.is_fatal("q1", "l:no", label="No")
    assert m2.stats["completes"] == 2
    assert m2.data["questions"]["q1"]["text"] == "text"


def test_quota_respects_avoid_setting(tmp_path):
    path = tmp_path / "mem.json"
    only_term = TerminateMemory(str(path), avoid=("terminate",))
    only_term.mark_end("q4", "plan", [{"label": "Decline"}], "quota_full")
    assert not only_term.is_fatal("q4", "l:decline to answer", label="Decline")

    both = TerminateMemory(str(path), avoid=("terminate", "quota_full"))
    both.mark_end("q4", "plan", [{"label": "Decline"}], "quota_full")
    assert both.is_fatal("q4", "l:decline to answer", label="Decline")


def test_clear(tmp_path):
    path = tmp_path / "mem.json"
    m = TerminateMemory(str(path))
    m.mark_end("q1", "t", [{"label": "No"}], "terminate")
    m.clear()
    assert not m.data["questions"]
    assert not m.is_fatal("q1", "l:no", label="No")
