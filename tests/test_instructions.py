from survey_flow.instructions import (
    match_targets_to_options,
    norm_label,
    parse_instruction,
)


class FakeOpt:
    def __init__(self, label, value=""):
        self.label = label
        self.value = value
        self.type = "radio"
        self.selected = False
        self.new_value = None


def test_norm_label():
    assert norm_label("  None of these! ") == "none of these"
    assert norm_label("Don't know") == norm_label("dont know")


def test_parse_select_quoted():
    h = parse_instruction('Select: "Yes"')
    assert h.kind == "options"
    assert "Yes" in h.targets


def test_parse_choose_option_b():
    h = parse_instruction("Choose option B")
    assert any("B" == t or t.endswith(" B") or t == "B" for t in h.targets) or "B" in h.targets


def test_parse_enter_number():
    h = parse_instruction("Enter: 25")
    assert 25.0 in h.numbers or (h.targets and h.targets[0] == "25")


def test_guidance_is_not_a_target():
    h = parse_instruction("Avoid selecting No here")
    assert h.kind == "none"


def test_parse_select_multiple_targets():
    h = parse_instruction("Select Apple and Banana")
    assert "Apple" in h.targets
    assert "Banana" in h.targets


def test_match_exact_and_case_insensitive():
    opts = [FakeOpt("Yes"), FakeOpt("No")]
    hits = match_targets_to_options(["yes"], [], opts, "single")
    assert [h.label for h in hits] == ["Yes"]


def test_match_by_number_position_nps():
    opts = [FakeOpt(str(i)) for i in range(11)]
    hits = match_targets_to_options([], [7.0], opts, "nps")
    assert [h.label for h in hits] == ["7"]


def test_match_fuzzy():
    opts = [FakeOpt("Full time employement")]  # sic
    hits = match_targets_to_options(["Full time employment"], [], opts, "single")
    assert len(hits) == 1


def test_match_multiple_targets():
    opts = [FakeOpt("Apple"), FakeOpt("Banana"), FakeOpt("Cherry")]
    hits = match_targets_to_options(["apple", "cherry"], [], opts, "multi")
    assert sorted(h.label for h in hits) == ["Apple", "Cherry"]
