"""Analyze a rendered survey page into structured Questions (no AI).

Strategy:
  * collect form controls in DOM order (stable indices shared with the
    browser engine),
  * cluster them into questions by base field name (q3_1_2 -> q3),
  * classify each cluster (SA/MA/dropdown/grid/ranking/scalar/...),
  * pull out red(-and-bold) instruction texts, plain notes and validation
    messages,
  * classify the page outcome (question / validation / interstitial /
    complete / terminate / quota-full).
"""

from __future__ import annotations

import hashlib
import re
from collections import defaultdict
from dataclasses import dataclass

from bs4 import BeautifulSoup, NavigableString, Tag

from .instructions import (
    CONSENT_RE,
    EXCLUSIVE_RE,
    PLACEHOLDER_OPTION_RE,
    VALIDATIONISH_RE,
    element_color_bold,
    norm_label,
)
from .models import Field, Outcome, Page, Question, SubmitControl
from .outcomes import ERROR_ELEM_RE, classify_text, find_validation

_NAME_BASE_RE = re.compile(r"(_\d+)+$")
_QTEXT_CLASS_RE = re.compile(
    r"(question[-_ ]?text|questiontext|prompt|qtext|survey[-_ ]?title|legend|"
    r"question[-_ ]?header|title)",
    re.I,
)
_INSTRUCTION_CLASS_RE = re.compile(r"(instruction|helper|hint|guidance|note)", re.I)
_RANK_LABEL_RE = re.compile(r"^(rank\s*)?\d{1,2}$", re.I)

_MIN_SEL_RE = re.compile(r"(?:at\s+least|minimum\s+of|min(?:imum)?\.?)\s+(\d+)", re.I)
_MAX_SEL_RE = re.compile(r"(?:no\s+more\s+than|at\s+most|up\s+to|maximum\s+of|max(?:imum)?\.?)\s+(\d+)", re.I)
_TOTAL_RE = re.compile(r"(?:allocate|total(?:\s+of)?|add(?:\s+up)?\s+to|sum\s+to|enter\s+points?\s+totalling)\s+(\d{1,4})", re.I)
_POINTS_RE = re.compile(r"\b(\d{1,4})\s*points?\b", re.I)


# --------------------------------------------------------------------------- #
# small HTML helpers
# --------------------------------------------------------------------------- #

def html_parser() -> str:
    """Best available BeautifulSoup parser.

    lxml (C extension) when installed; otherwise the stdlib html.parser so the
    tool keeps working in slim / single-file installs. Override with
    SURVEY_FLOW_HTML_PARSER=lxml|html.parser.
    """
    import os

    forced = (os.environ.get("SURVEY_FLOW_HTML_PARSER") or "").strip()
    if forced in ("lxml", "html.parser"):
        return forced
    try:
        import lxml  # noqa: F401
        return "lxml"
    except Exception:
        return "html.parser"


def _clean_text(s: str, limit: int = 600) -> str:
    s = re.sub(r"\s+", " ", s or "").strip()
    return s[:limit]


def _node_text(node) -> str:
    if isinstance(node, NavigableString):
        if getattr(node.parent, "name", None) in ("script", "style"):
            return ""
        return _clean_text(str(node))
    if isinstance(node, Tag):
        if node.name in ("script", "style", "input", "select", "textarea", "button"):
            return ""
        return _clean_text(node.get_text(" ", strip=True))
    return ""


def _label_map(soup: BeautifulSoup) -> dict[str, str]:
    out = {}
    for lab in soup.find_all("label"):
        fid = lab.get("for")
        if fid:
            out[fid] = _clean_text(lab.get_text(" ", strip=True), 200)
    return out


def _field_label(tag: Tag, for_map: dict[str, str]) -> str:
    # 1) wrapping <label>
    wrap = tag.find_parent("label")
    if wrap is not None:
        txt = _clean_text(wrap.get_text(" ", strip=True), 200)
        if txt:
            return txt
    # 2) <label for=id>
    tid = tag.get("id")
    if tid and tid in for_map:
        return for_map[tid]
    # 3) nearby sibling text (common answer patterns)
    for sib in list(tag.next_siblings)[:4]:
        txt = _node_text(sib)
        if 0 < len(txt) <= 120:
            return txt
    # 4) sibling cell of the parent (td/row patterns)
    parent = tag.parent
    for _ in range(2):
        if parent is None or not isinstance(parent, Tag):
            break
        for sib in list(parent.next_siblings)[:3]:
            txt = _node_text(sib)
            if 0 < len(txt) <= 120:
                return txt
        parent = parent.parent
    # 5) aria/title/placeholder
    for attr in ("aria-label", "title", "placeholder"):
        v = tag.get(attr)
        if v:
            return _clean_text(v, 200)
    # 6) value for buttons
    if tag.get("value"):
        return _clean_text(tag.get("value"), 200)
    return ""


def _collect_fields(soup: BeautifulSoup):
    """Return list of (dom_index, tag, type_str)."""
    out = []
    for i, tag in enumerate(soup.find_all(["input", "select", "textarea"])):
        if tag.name == "input":
            typ = (tag.get("type") or "text").lower()
        elif tag.name == "select":
            typ = "select-multiple" if tag.has_attr("multiple") else "select-one"
        else:
            typ = "textarea"
        out.append((i, tag, typ))
    return out


def _common_ancestor(tags: list[Tag]) -> Tag | None:
    """Deepest proper ancestor shared by all tags (parent-level, so a lone
    tag resolves to its container, not to itself)."""
    if not tags:
        return None

    def path(t: Tag):
        p = []
        cur = t.parent          # proper ancestors only
        while cur is not None:
            p.append(cur)
            cur = cur.parent
        return p[::-1]

    common = path(tags[0])
    for t in tags[1:]:
        p = path(t)
        common = [a for a, b in zip(common, p) if a is b]
        if not common:
            break
    return common[-1] if common else None


# containers we may climb out of to reach the question wrapper
_CLIMBABLE = {"label", "td", "th", "tr", "li", "span", "font", "b", "strong", "table", "p", "ul", "tbody", "div"}


def _climb_to_question(nca: Tag | None, cluster_ids: set, all_field_ids: set) -> Tag | None:
    """Climb out of option-level wrappers (labels/tds/tables) until the
    container that holds the whole question - but never into a container
    that holds fields from a *different* question (avoids instruction
    bleed between questions)."""
    guard = 0
    while (
        nca is not None
        and nca.name in _CLIMBABLE
        and nca.parent is not None
        and nca.parent.name not in ("form", "body", "html")
        and guard < 5
    ):
        parent = nca.parent
        foreign = [
            t for t in parent.find_all(["input", "select", "textarea"])
            if id(t) in all_field_ids and id(t) not in cluster_ids
        ]
        if foreign:
            break
        nca = parent
        guard += 1
    return nca


def _name_base(name: str) -> str:
    return _NAME_BASE_RE.sub("", name or "")


def _question_text(nca: Tag | None, first_tag: Tag, exclude: list[str]) -> str:
    """Best-effort question wording for a field cluster."""
    cands: list[str] = []
    if nca is not None:
        leg = nca.find("legend")
        if leg is not None:
            cands.append(_clean_text(leg.get_text(" ", strip=True)))
        for t in nca.find_all(True, class_=_QTEXT_CLASS_RE):
            cands.append(_clean_text(t.get_text(" ", strip=True)))
            break
        # heading tags
        for t in nca.find_all(["h1", "h2", "h3", "h4"]):
            cands.append(_clean_text(t.get_text(" ", strip=True)))
            break

    # walk backwards from the first field, collecting strings
    chunks: list[str] = []
    for node in first_tag.previous_elements:
        if node is nca:
            break
        if isinstance(node, NavigableString):
            txt = _node_text(node)
            if txt:
                chunks.append(txt)
                joined = " ".join(reversed(chunks))
                if len(joined) >= 25:
                    break
        elif isinstance(node, Tag):
            if node.name in ("input", "select", "textarea"):
                break
            if node.name in ("form", "body"):
                break
    if chunks:
        cands.append(" ".join(reversed(chunks)))

    for c in cands:
        if not c:
            continue
        # strip instruction/red fragments we already captured separately
        stripped = c
        for ex in exclude:
            if ex and ex in stripped:
                stripped = stripped.replace(ex, " ")
        stripped = _clean_text(stripped)
        if len(stripped) >= 2:
            return stripped
    return _clean_text(cands[0]) if cands else ""


def _row_label(tag: Tag) -> str:
    """Row header for a grid cell: preceding cell/sibling text."""
    for sib in list(tag.previous_siblings)[:3]:
        txt = _node_text(sib)
        if 0 < len(txt) <= 120:
            return txt
    parent = tag.parent
    for _ in range(2):
        if parent is None or not isinstance(parent, Tag):
            break
        for sib in list(parent.previous_siblings)[:3]:
            txt = _node_text(sib)
            if 0 < len(txt) <= 120:
                return txt
        parent = parent.parent
    return ""


# --------------------------------------------------------------------------- #
# instruction / validation extraction
# --------------------------------------------------------------------------- #

@dataclass
class ExtractedTexts:
    instructions: list[str]
    notes: list[str]
    validation_msgs: list[str]


def _extract_texts(nca: Tag | None, page_scope: Tag) -> ExtractedTexts:
    scope = nca if nca is not None else page_scope
    instructions: list[str] = []
    notes: list[str] = []
    validation: list[str] = []
    seen: set[str] = set()

    candidates = scope.find_all(
        ["b", "strong", "span", "div", "p", "em", "i", "font", "legend", "li", "td", "h1", "h2", "h3", "h4"]
    )
    for elem in candidates:
        txt = _clean_text(elem.get_text(" ", strip=True), 500)
        if not txt or len(txt) > 400:
            continue
        key = norm_label(txt)
        if not key or key in seen:
            continue
        # never treat answer-option labels as instructions
        if elem.find_parent("label") is not None:
            continue
        red, bold = element_color_bold(elem, scope)
        if red:
            seen.add(key)
            if VALIDATIONISH_RE.search(txt):
                validation.append(txt)
            else:
                instructions.append(txt)
            continue
        # plain instruction containers (non-red) -> notes
        classes = " ".join(elem.get("class") or []).lower()
        if _INSTRUCTION_CLASS_RE.search(classes) and not elem.find_parent("label"):
            seen.add(key)
            if VALIDATIONISH_RE.search(txt):
                validation.append(txt)
            else:
                notes.append(txt)

    # de-duplicate nested notes: drop note fully contained in an instruction
    notes = [n for n in notes if not any(n != i and norm_label(n) in norm_label(i) for i in instructions)]
    return ExtractedTexts(instructions=instructions, notes=notes, validation_msgs=validation)


def _page_error_elements(soup: BeautifulSoup) -> list[str]:
    out = []
    seen: set[str] = set()
    for elem in soup.find_all(True):
        ident = " ".join(elem.get("class") or []) + " " + (elem.get("id") or "") + " " + (elem.get("role") or "")
        if ERROR_ELEM_RE.search(ident):
            txt = _clean_text(elem.get_text(" ", strip=True), 300)
            if txt and norm_label(txt) not in seen:
                seen.add(norm_label(txt))
                out.append(txt)
    return out


# --------------------------------------------------------------------------- #
# question classification
# --------------------------------------------------------------------------- #

def _classify_choice(fields: list[Field]) -> str:
    radios = [f for f in fields if f.type == "radio"]
    checks = [f for f in fields if f.type == "checkbox"]
    pool = radios or checks

    by_name: dict[str, list[Field]] = defaultdict(list)
    for f in pool:
        by_name[f.name].append(f)

    def structure() -> tuple[bool, list[str], int]:
        groups = [by_name[n] for n in by_name]
        sizes = {len(g) for g in groups}
        labelsets = {tuple(sorted({norm_label(x.label or x.value) for x in g})) for g in groups}
        uniform = len(groups) >= 2 and len(sizes) == 1 and next(iter(sizes)) >= 2 and len(labelsets) == 1
        return uniform, [g[0].row_label for g in groups], next(iter(sizes)) if sizes else 0

    if len(by_name) == 1:
        return "single" if radios else "multi"

    uniform, _rows, ncols = structure()
    all_singletons = all(len(g) == 1 for g in by_name.values())

    if radios:
        if uniform:
            # ranking? rows == cols and labels look like ranks 1..N
            nrows = len(by_name)
            col_labels = [norm_label(x.label or x.value) for x in by_name[next(iter(by_name))]]
            rankish = all(_RANK_LABEL_RE.match(c) for c in col_labels) and nrows == ncols
            return "ranking" if rankish else "grid_single"
        if all_singletons:
            return "single"       # per-option names (q1_1, q1_2, ... one radio each)
        return "grid_single"
    else:
        if uniform:
            return "grid_multi"
        if all_singletons:
            return "multi"
        return "grid_multi"


def _parse_selects(fields: list[Field]) -> None:
    for f in fields:
        if f.is_placeholder_option:
            label = f.label
            if not label or PLACEHOLDER_OPTION_RE.match(label) or f.value == "":
                f.is_placeholder_option = True


# --------------------------------------------------------------------------- #
# main entry
# --------------------------------------------------------------------------- #

def analyze(html: str, url: str = "") -> Page:
    full_soup = BeautifulSoup(html or "", html_parser())
    script_count = len(full_soup.find_all("script"))
    soup = full_soup
    for bad in soup(["script", "style", "noscript"]):
        bad.extract()
    page_root: Tag = soup.body or soup

    fields_raw = _collect_fields(soup)
    for_map = _label_map(soup)

    # --- page-level pieces ------------------------------------------------- #
    hiddens: list[tuple[str, str]] = []
    submits: list[SubmitControl] = []
    answer_fields: list[Field] = []

    for dom_index, tag, typ in fields_raw:
        if typ == "hidden":
            if tag.get("name"):
                hiddens.append((tag["name"], tag.get("value", "")))
            continue
        if typ in ("submit", "image") or (tag.name == "button" and typ != "button"):
            label = _clean_text(tag.get("value") or tag.get_text(" ", strip=True), 80)
            submits.append(SubmitControl(
                name=tag.get("name", ""), value=tag.get("value", ""),
                label=label, dom_index=dom_index, tag=tag.name,
            ))
            continue
        if tag.name == "button" or typ == "button":
            label = _clean_text(tag.get_text(" ", strip=True), 80)
            submits.append(SubmitControl(
                name=tag.get("name", ""), value=tag.get("value", ""),
                label=label, dom_index=dom_index, tag="button",
            ))
            continue

        f = Field(dom_index=dom_index, name=tag.get("name", ""), type=typ)
        f.label = _field_label(tag, for_map)
        f.value = tag.get("value", "") or ""
        f.checked = tag.has_attr("checked")
        f.min = tag.get("min")
        f.max = tag.get("max")
        f.step = tag.get("step")
        f.placeholder = tag.get("placeholder")
        f.pattern = tag.get("pattern")
        f.required = tag.has_attr("required") or (tag.get("aria-required") or "").lower() == "true"
        ml, mn = tag.get("maxlength"), tag.get("minlength")
        f.maxlength = int(ml) if ml and str(ml).isdigit() else None
        f.minlength = int(mn) if mn and str(mn).isdigit() else None

        if typ in ("radio", "checkbox"):
            f.exclusive = bool(EXCLUSIVE_RE.search(f.label)) and bool(f.label)
            f.consent = bool(CONSENT_RE.search(f.label)) and bool(f.label)
            if f.type == "radio":
                f.row_label = _row_label(tag)
        if typ in ("select-one", "select-multiple"):
            pass
        answer_fields.append(f)

    # select options live inside <select>: expand them
    for f in answer_fields:
        if f.type not in ("select-one", "select-multiple"):
            continue
        tag = next(t for i, t, _ in fields_raw if i == f.dom_index)
        opts = []
        for o in tag.find_all("option"):
            of = Field(dom_index=f.dom_index, name=f.name, type=f.type)
            of.label = _clean_text(o.get_text(" ", strip=True), 200)
            of.value = o.get("value", "")
            of.is_placeholder_option = (not of.value) or bool(PLACEHOLDER_OPTION_RE.match(of.label))
            of.checked = o.has_attr("selected")
            opts.append(of)
        f.options = opts
        if f.type == "select-one" and not f.new_value:
            # current selection: explicitly selected option, else first non-placeholder
            cur = next((o for o in opts if o.checked), None)
            if cur is None:
                cur = next((o for o in opts if not o.is_placeholder_option), None) or (
                    opts[0] if opts else None
                )
            if cur is not None:
                f.value = cur.value

    # --- clustering -------------------------------------------------------- #
    clusters: dict[str, list[Field]] = defaultdict(list)
    anon_parent: dict[int, Tag] = {}
    for f in answer_fields:
        if f.name:
            clusters[_name_base(f.name)].append(f)
        else:
            tag = next(t for i, t, _ in fields_raw if i == f.dom_index)
            parent = tag.parent if isinstance(tag.parent, Tag) else page_root
            anon_parent.setdefault(id(parent), parent)
            clusters[f"__anon_{id(parent)}"].append(f)

    # DOM-order clusters
    clusters_ordered = sorted(
        clusters.values(),
        key=lambda fs: min(f.dom_index for f in fs),
    )

    dom_tag = {i: t for i, t, _ in fields_raw}
    all_field_ids = {id(t) for t in dom_tag.values()}

    # resolve each cluster's container, then merge clusters that together
    # form one constant-sum question (separate names p1/p2/p3 in one box)
    resolved: list[tuple[list[Field], list[Tag], Tag | None]] = []
    for fs in clusters_ordered:
        tags = [dom_tag[f.dom_index] for f in fs if f.dom_index in dom_tag]
        if not tags:
            continue
        nca = _common_ancestor(tags)
        nca = _climb_to_question(nca, {id(t) for t in tags}, all_field_ids)
        resolved.append((fs, tags, nca))

    merged: list[tuple[list[Field], list[Tag], Tag | None]] = []
    used: set[int] = set()
    for i, (fs, tags, nca) in enumerate(resolved):
        if i in used:
            continue
        group_fs, group_tags, group_idx = list(fs), list(tags), [i]
        if nca is not None and all(f.type == "number" for f in fs):
            ntext = nca.get_text(" ", strip=True)
            if _TOTAL_RE.search(ntext) or _POINTS_RE.search(ntext):
                for j in range(i + 1, len(resolved)):
                    if j in used:
                        continue
                    fs2, tags2, nca2 = resolved[j]
                    if nca2 is nca and all(f.type == "number" for f in fs2):
                        group_fs.extend(fs2)
                        group_tags.extend(tags2)
                        group_idx.append(j)
        for j in group_idx:
            used.add(j)
        merged.append((group_fs, group_tags, nca))

    # --- build questions --------------------------------------------------- #
    questions: list[Question] = []
    all_validation: list[str] = []
    page_scope: Tag = page_root

    for fs, tags, nca in merged:
        texts = _extract_texts(nca, page_scope)
        all_validation.extend(texts.validation_msgs)

        qtext = _question_text(nca, tags[0], texts.instructions)

        # kind
        has_radio = any(f.type == "radio" for f in fs)
        has_check = any(f.type == "checkbox" for f in fs)
        has_select = any(f.type in ("select-one", "select-multiple") for f in fs)
        scalars = [f for f in fs if f.type in ("text", "textarea", "number", "range", "date", "time", "datetime-local")]

        if has_radio or has_check:
            kind = _classify_choice(fs)
        elif has_select:
            kind = "dropdown"
        elif scalars:
            stypes = {f.type for f in scalars}
            if stypes == {"number"} or stypes == {"number", "text"}:
                if len(scalars) >= 2 or _TOTAL_RE.search(qtext) or _POINTS_RE.search(qtext):
                    kind = "constant_sum"
                else:
                    kind = "number"
            elif "range" in stypes:
                kind = "slider"
            elif "date" in stypes:
                kind = "date"
            elif "time" in stypes:
                kind = "time"
            else:
                kind = "text"
        else:
            kind = "unknown"

        # sig
        sig = ""
        if nca is not None:
            for attr in ("data-qid", "data-question-id", "id"):
                v = nca.get(attr)
                if v and re.search(r"[Qq]|\d", str(v)):
                    sig = str(v)
                    break
        if not sig:
            named = [f.name for f in fs if f.name]
            if named:
                base = _name_base(named[0])
                # ensure all names share the base, otherwise hash
                if all(_name_base(n) == base or n == base for n in named):
                    sig = base
        if not sig:
            sig = hashlib.sha1(norm_label(qtext).encode()).hexdigest()[:10]

        q = Question(sig=sig, text=qtext, kind=kind, fields=fs,
                     instructions=texts.instructions, notes=texts.notes)
        q.qid = (nca.get("data-qid") or nca.get("id") or "") if nca is not None else ""

        # min/max/total from wording
        m = _MIN_SEL_RE.search(qtext) or _MIN_SEL_RE.search(" ".join(q.notes + q.instructions))
        if m:
            q.min_select = max(1, int(m.group(1)))
        m = _MAX_SEL_RE.search(qtext)
        if m:
            q.max_select = int(m.group(1))
        if kind == "constant_sum":
            m = _TOTAL_RE.search(qtext) or _POINTS_RE.search(qtext)
            if m:
                q.total = int(m.group(1))

        # grid row labels (assign to every field of the row)
        if kind in ("grid_single", "grid_multi", "ranking"):
            by_name: dict[str, list[Field]] = defaultdict(list)
            for f in fs:
                if f.type in ("radio", "checkbox"):
                    by_name[f.name].append(f)
            for _n, group in by_name.items():
                rl = group[0].row_label
                for f in group:
                    f.row_label = rl

        questions.append(q)

    # --- validation messages ---------------------------------------------- #
    all_validation.extend(_page_error_elements(soup))
    visible = page_scope.get_text("\n", strip=True)
    all_validation.extend(find_validation(visible))
    # dedupe, keep order
    seen_v: set[str] = set()
    validations = []
    for v in all_validation:
        k = norm_label(v)
        if k and k not in seen_v:
            seen_v.add(k)
            validations.append(v)

    # --- outcome ----------------------------------------------------------- #
    status, evidence = classify_text(visible, url)
    has_answer = bool(answer_fields)
    if status and has_answer and not _url_signals_end(url):
        # end-text coexisting with real answer controls on a fresh page is
        # usually question wording (e.g. "Have you ever been terminated?")
        status = ""

    if status:
        outcome = Outcome(status)
        messages = validations if outcome in (Outcome.TERMINATE, Outcome.QUOTA_FULL) else evidence
        if outcome in (Outcome.TERMINATE, Outcome.QUOTA_FULL, Outcome.COMPLETE):
            messages = evidence + validations
    elif validations and has_answer:
        outcome = Outcome.VALIDATION
        messages = validations
    elif has_answer:
        outcome = Outcome.QUESTION
        messages = validations
    elif submits:
        outcome = Outcome.INTERSTITIAL
        messages = validations
    else:
        outcome = Outcome.UNKNOWN
        messages = validations

    # form info
    form_action, form_method, form_tag = "", "POST", None
    if answer_fields:
        first_tag = next((t for i, t, _ in fields_raw if i == answer_fields[0].dom_index), None)
        form_tag = first_tag.find_parent("form") if first_tag is not None else None
    if form_tag is None:
        form_tag = soup.find("form")
    if form_tag is not None:
        form_action = form_tag.get("action", "") or url
        form_method = (form_tag.get("method") or "POST").upper()

    js_heavy = (not answer_fields) and (not submits) and script_count > 0

    return Page(
        url=url,
        outcome=outcome,
        questions=questions,
        messages=messages,
        hiddens=hiddens,
        submits=submits,
        form_action=form_action,
        form_method=form_method,
        js_heavy=js_heavy,
    )


def _url_signals_end(url: str) -> bool:
    u = (url or "").lower()
    return bool(re.search(r"terminat|quota|complete|finished|thanks|thankyou|/end\b", u))


def page_report(page: Page) -> str:
    """Human-readable structure report (used by the `analyze` command)."""
    lines = [f"url:       {page.url}",
             f"outcome:   {page.outcome.value}"]
    for m in page.messages[:5]:
        lines.append(f"message:   {m}")
    lines.append(f"questions: {len(page.questions)}")
    for q in page.questions:
        lines.append(f"  - [{q.sig}] ({q.kind}) {q.text[:90]!r}")
        for i in q.instructions:
            lines.append(f"      RED instruction: {i!r}")
        for n in q.notes[:2]:
            lines.append(f"      note: {n!r}")
        show = q.fields if len(q.fields) <= 14 else q.fields[:14]
        for f in show:
            extra = f" row={f.row_label!r}" if f.row_label else ""
            flags = []
            if f.exclusive:
                flags.append("exclusive")
            if f.consent:
                flags.append("consent")
            if f.is_placeholder_option:
                flags.append("placeholder")
            fl = (" [" + ",".join(flags) + "]") if flags else ""
            lines.append(
                f"        #{f.dom_index} {f.type:<14} name={f.name!r} "
                f"label={f.label!r} value={f.value!r}{extra}{fl}"
            )
        if len(q.fields) > 14:
            lines.append(f"        ... {len(q.fields) - 14} more fields")
    if page.submits:
        lines.append("submits:   " + ", ".join(
            f"{s.label!r}({s.tag}:{s.name})" for s in page.submits[:6]))
    return "\n".join(lines)
