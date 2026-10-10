"""Data models shared across analyzer, decider, engines and runner."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class Outcome(str, Enum):
    """Classification of the page the tool is currently on."""

    QUESTION = "question"
    INTERSTITIAL = "interstitial"      # start / continue-only page
    VALIDATION = "validation"          # question page carrying validation errors
    COMPLETE = "complete"
    TERMINATE = "terminate"
    QUOTA_FULL = "quota_full"
    UNKNOWN = "unknown"


@dataclass
class Field:
    """A single form control, in DOM order."""

    dom_index: int                    # index among input/select/textarea tags
    name: str
    type: str                         # radio|checkbox|select-one|select-multiple|text|
                                      # textarea|number|range|date|time|hidden|submit|...
    label: str = ""
    value: str = ""                   # current/original value (options: their value)
    checked: bool = False             # original checked state
    row_label: str = ""               # for grids: the row header text
    min: Optional[str] = None
    max: Optional[str] = None
    step: Optional[str] = None
    maxlength: Optional[int] = None
    minlength: Optional[int] = None
    placeholder: Optional[str] = None
    pattern: Optional[str] = None
    required: bool = False
    exclusive: bool = False           # "None of these", "Don't know", ...
    consent: bool = False             # consent/agree checkbox
    is_placeholder_option: bool = False  # "-- Select --" style dropdown option
    options: list["Field"] = field(default_factory=list)  # for selects: their options
    # decision output ------------------------------------------------------
    selected: bool = False            # checkbox/radio: should be selected
    new_value: Optional[str] = None   # scalar/select: value to send

    @property
    def identity(self) -> str:
        """Stable identity used by the terminate-memory."""
        from .instructions import norm_label

        if self.label:
            return "l:" + norm_label(self.label)
        if self.value:
            return "v:" + self.value
        return "n:" + self.name

    def chosen_identity(self) -> Optional[str]:
        """Identity of this field's chosen answer (None when not chosen)."""
        if self.type in ("radio", "checkbox"):
            return self.identity if self.selected else None
        if self.type == "select-one" and self.new_value is not None:
            opt = next((o for o in self.options if o.value == self.new_value), None)
            if opt is not None:
                return opt.identity
            return "v:" + self.new_value
        if self.type == "select-multiple":
            return None
        return None

    @property
    def chosen_option(self) -> Optional["Field"]:
        """For a select: the option Field that was chosen."""
        if self.type != "select-one" or self.new_value is None:
            return None
        return next((o for o in self.options if o.value == self.new_value), None)


@dataclass
class Question:
    """One question (cluster of fields) on the current page."""

    sig: str                          # stable signature (container id / name base / text hash)
    text: str
    kind: str                         # single|multi|dropdown|text|number|slider|date|time|
                                      # grid_single|grid_multi|ranking|constant_sum|unknown
    fields: list[Field] = field(default_factory=list)
    instructions: list[str] = field(default_factory=list)   # red(-and-bold) instruction texts
    notes: list[str] = field(default_factory=list)          # plain instruction texts
    qid: str = ""
    min_select: int = 1
    max_select: Optional[int] = None
    total: Optional[int] = None       # constant-sum total
    validation: list[str] = field(default_factory=list)     # validation messages for this page

    @property
    def is_choice(self) -> bool:
        return self.kind in (
            "single", "multi", "dropdown", "grid_single",
            "grid_multi", "ranking", "rating", "nps",
        )

    @property
    def choices(self) -> list[Field]:
        return [f for f in self.fields if f.type in ("radio", "checkbox")
                or f.type in ("select-one", "select-multiple")]

    @property
    def scalars(self) -> list[Field]:
        return [f for f in self.fields if f.type in
                ("text", "textarea", "number", "range", "date", "time", "datetime-local")]

    @property
    def chosen_options(self) -> list[Field]:
        """The option-level fields actually chosen (selects resolve to their option)."""
        out: list[Field] = []
        for f in self.fields:
            if f.type in ("radio", "checkbox") and f.selected:
                out.append(f)
            elif f.type == "select-one" and f.new_value is not None:
                opt = f.chosen_option
                out.append(opt if opt is not None else f)
        return out

    @property
    def value_texts(self) -> list[str]:
        """Human-readable descriptions of what was chosen."""
        out = []
        for f in self.chosen_options:
            out.append(f.label or f.new_value or f.value)
        for f in self.scalars:
            if f.new_value not in (None, ""):
                out.append(f"{f.label or f.name}={f.new_value}")
        return out


@dataclass
class SubmitControl:
    name: str
    value: str
    label: str
    dom_index: int
    tag: str = "input"                # input|button


@dataclass
class Page:
    """Fully analyzed page."""

    url: str
    outcome: Outcome
    questions: list[Question] = field(default_factory=list)
    messages: list[str] = field(default_factory=list)     # end-status / validation messages
    hiddens: list[tuple[str, str]] = field(default_factory=list)
    submits: list[SubmitControl] = field(default_factory=list)
    form_action: str = ""
    form_method: str = "POST"
    js_heavy: bool = False            # page looks JS-driven (no classic form)

    @property
    def signature(self) -> str:
        """Cheap identity of this page state, used for stall detection."""
        return "|".join(q.sig for q in self.questions) or f"{self.outcome.value}:{self.url}"
