#!/usr/bin/env python3
"""Build the shareable ONE-FILE tool: dist/SurveyFlow.pyw

The output file contains everything (the tool + its libraries) and runs on any
machine that has Python 3, with nothing to install:

    python SurveyFlow.pyw                # open the window (double-click on Windows)
    python SurveyFlow.pyw selftest       # check the tool is working
    python SurveyFlow.pyw run URL --completes 5

How the file is put together:
  * third-party libraries (requests, beautifulsoup4, ...) are bundled as
    pure-Python source (no .so/.pyd - the build fails if a compiled file
    appears), so the file is not tied to one OS or one Python version;
  * the survey_flow code itself is bundled ENCRYPTED (zlib + xor keystream)
    and loaded from memory, so the file cannot simply be unzipped to read the
    source;
  * on first run the file unpacks its libraries to ~/.survey_flow_app/ and
    starts; later runs reuse that cache.

Usage:
    python scripts/build_standalone.py [--out dist/SurveyFlow.pyw] [--verify]
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import io
import os
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import zipfile
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PKG = "survey_flow"

# pure-Python runtime dependencies (see requirements.txt; lxml is optional and
# deliberately NOT bundled - the tool falls back to stdlib html.parser).
VENDOR_MODULES = [
    "requests",
    "beautifulsoup4",
    "soupsieve",
    "urllib3",
    "idna",
    "certifi",
    "charset_normalizer",
    "typing_extensions",
]

COMPILED_SUFFIXES = {".so", ".pyd", ".dll", ".dylib"}
STRIP_SUFFIXES = {".pyc", ".pyo"}
STRIP_DIRS = {"__pycache__", ".pytest_cache"}


# --------------------------------------------------------------------------- #
# module encryption (must match the launcher template below)
# --------------------------------------------------------------------------- #

def _keystream(key: bytes, name: str, n: int) -> bytes:
    out = bytearray()
    counter = 0
    while len(out) < n:
        out += hashlib.sha256(
            key + name.encode("utf-8") + counter.to_bytes(4, "little")
        ).digest()
        counter += 1
    return bytes(out[:n])


def _xor(data: bytes, key: bytes) -> bytes:
    return bytes(b ^ key[i % len(key)] for i, b in enumerate(data))


def encrypt_module(source: bytes, key: bytes, fullname: str) -> bytes:
    plain = zlib.compress(source, 9)
    return b"SFP1" + _xor(plain, _keystream(key, fullname, len(plain)))


# --------------------------------------------------------------------------- #
# payload assembly
# --------------------------------------------------------------------------- #

def package_version() -> str:
    text = (ROOT / PKG / "__init__.py").read_text(encoding="utf-8")
    m = re.search(r'__version__\s*=\s*"([^"]+)"', text)
    return m.group(1) if m else "0.0.0"


def collect_modules(key: bytes) -> dict:
    """survey_flow source -> encrypted SFP entries {arcname: bytes}."""
    entries = {}
    base = ROOT / PKG
    for py in sorted(base.rglob("*.py")):
        rel = py.relative_to(base)
        parts = list(rel.with_suffix("").parts)
        if parts[-1] == "__main__":
            continue
        if parts[-1] == "__init__":
            parts = parts[:-1]
        fullname = ".".join([PKG] + parts) if parts else PKG
        arc = "SFP/" + PKG + "/" + "/".join(rel.with_suffix(".sfp").parts)
        entries[arc] = encrypt_module(py.read_bytes(), key, fullname)
    return entries


def install_vendor(target: Path) -> None:
    """pip-install the pure-Python dependencies into `target` and sanitize."""
    subprocess.check_call(
        [sys.executable, "-m", "pip", "install", "--quiet", "--upgrade",
         "--target", str(target), "--no-deps"] + VENDOR_MODULES,
    )
    removed = []
    for path in sorted(target.rglob("*"), reverse=True):
        if path.is_dir():
            if path.name in STRIP_DIRS or path.name.endswith(".dist-info"):
                shutil.rmtree(path, ignore_errors=True)
            continue
        if path.suffix in COMPILED_SUFFIXES or path.suffix in STRIP_SUFFIXES:
            removed.append(path)
            path.unlink()
    # every compiled module we removed must have a pure-Python twin
    for path in removed:
        if path.suffix in STRIP_SUFFIXES:
            continue
        twin = path.parent / (path.name.split(".")[0] + ".py")
        if not twin.is_file():
            raise SystemExit(
                f"build aborted: {path.name} has no pure-Python fallback ({twin})"
            )
    for path in target.rglob("*"):
        if path.suffix in COMPILED_SUFFIXES:
            raise SystemExit(f"build aborted: compiled file survived: {path}")


def build_payload(key: bytes) -> bytes:
    entries = collect_modules(key)
    with tempfile.TemporaryDirectory() as td:
        vendor = Path(td) / "vendor"
        vendor.mkdir()
        install_vendor(vendor)
        for path in sorted(vendor.rglob("*")):
            if path.is_file():
                rel = path.relative_to(vendor).as_posix()
                entries["vendor/" + rel] = path.read_bytes()
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as zf:
        for name in sorted(entries):
            zf.writestr(name, entries[name])
    return buf.getvalue()


# --------------------------------------------------------------------------- #
# the launcher that is written as the final .pyw (stdlib only)
# --------------------------------------------------------------------------- #

LAUNCHER_TEMPLATE = r'''#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Survey Flow %%VERSION%% - single-file survey test tool.

Double-click this file (Windows) or run:  python SurveyFlow.pyw
Paste your survey TEST link, set the number of completes, press Start.

Useful commands (from a terminal):
    python SurveyFlow.pyw selftest             <- check the tool is working
    python SurveyFlow.pyw run URL --completes 5
    python SurveyFlow.pyw analyze URL
    python SurveyFlow.pyw memory show
    python SurveyFlow.pyw ui                   <- open the window

Nothing to install: this one file carries the whole tool. On first start it
unpacks its own libraries into ~/.survey_flow_app/ (hidden cache).

Built %%BUILD_TIME%%.
"""

from __future__ import annotations

import base64
import hashlib
import importlib.abc
import importlib.util
import io
import os
import shutil
import sys
import traceback
import zipfile
import zlib

BUILD_VERSION = "%%VERSION%%"
_BUILD_KEY = bytes.fromhex("%%KEY%%")
_PAYLOAD = """%%PAYLOAD%%"""


# --------------------------------------------------------------------- #
# crash reporting (the window may run without a console)
# --------------------------------------------------------------------- #

def _crash_dir():
    d = os.path.join(os.path.expanduser("~"), ".survey_flow_app")
    os.makedirs(d, exist_ok=True)
    return d


def _crash():
    text = traceback.format_exc()
    try:
        path = os.path.join(_crash_dir(), "crash-" + BUILD_VERSION + ".log")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text)
    except Exception:
        path = "(crash log could not be written)"
    if sys.platform == "win32":
        try:
            import ctypes
            ctypes.windll.user32.MessageBoxW(
                0,
                "Survey Flow failed to start.\n\nDetails were saved to:\n"
                + path + "\n\n" + text[-800:],
                "Survey Flow",
                16,
            )
        except Exception:
            pass
    try:
        if sys.stderr:
            sys.stderr.write(text)
    except Exception:
        pass


# --------------------------------------------------------------------- #
# payload extraction + obfuscated module loader
# --------------------------------------------------------------------- #

def _keystream(key, name, n):
    out = bytearray()
    counter = 0
    while len(out) < n:
        out += hashlib.sha256(
            key + name.encode("utf-8") + counter.to_bytes(4, "little")
        ).digest()
        counter += 1
    return bytes(out[:n])


def _xor(data, ks):
    return bytes(b ^ ks[i % len(ks)] for i, b in enumerate(data))


def _ensure_payload():
    raw = zlib.decompress(base64.b64decode(_PAYLOAD))
    digest = hashlib.sha256(raw).hexdigest()[:16]
    root = os.path.join(os.path.expanduser("~"), ".survey_flow_app",
                        "payload-" + digest)
    if os.path.isfile(os.path.join(root, ".done")):
        return root
    os.makedirs(os.path.dirname(root), exist_ok=True)
    tmp = root + ".tmp-" + str(os.getpid())
    shutil.rmtree(tmp, ignore_errors=True)
    try:
        with zipfile.ZipFile(io.BytesIO(raw)) as zf:
            zf.extractall(tmp)
        with open(os.path.join(tmp, ".done"), "w", encoding="utf-8"):
            pass
        shutil.rmtree(root, ignore_errors=True)
        os.replace(tmp, root)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
    return root


class _SfpLoader(importlib.abc.Loader):
    def __init__(self, path, fullname):
        self.path = path
        self.fullname = fullname

    def create_module(self, spec):
        return None

    def exec_module(self, module):
        with open(self.path, "rb") as fh:
            raw = fh.read()
        if raw[:4] != b"SFP1":
            raise ImportError("bad module blob: " + self.path)
        blob = raw[4:]
        plain = zlib.decompress(_xor(blob, _keystream(_BUILD_KEY,
                                                      self.fullname, len(blob))))
        module.__file__ = self.path
        code = compile(plain.decode("utf-8"), self.path, "exec")
        exec(code, module.__dict__)


class _SfpFinder(importlib.abc.MetaPathFinder):
    def __init__(self, root):
        self.root = root

    def find_spec(self, fullname, path=None, target=None):
        rel = fullname.split(".")
        base = os.path.join(self.root, *rel)
        for candidate, is_pkg in ((os.path.join(base, "__init__.sfp"), True),
                                  (base + ".sfp", False)):
            if os.path.isfile(candidate):
                return importlib.util.spec_from_file_location(
                    fullname,
                    candidate,
                    loader=_SfpLoader(candidate, fullname),
                    submodule_search_locations=[base] if is_pkg else None,
                )
        return None


# --------------------------------------------------------------------- #
# entry
# --------------------------------------------------------------------- #

def main():
    # running windowed (pythonw): keep logging/print safe
    if sys.stdout is None:
        sys.stdout = open(os.devnull, "w")
    if sys.stderr is None:
        sys.stderr = open(os.devnull, "w")

    root = _ensure_payload()
    sys.dont_write_bytecode = True
    sys.path.insert(0, os.path.join(root, "vendor"))
    sys.meta_path.insert(0, _SfpFinder(os.path.join(root, "SFP")))

    from survey_flow.cli import main as cli_main

    argv = sys.argv[1:]
    if not argv:
        argv = ["ui"]
    return cli_main(argv)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except SystemExit:
        raise
    except Exception:
        _crash()
        sys.exit(1)
'''


# --------------------------------------------------------------------------- #
# build
# --------------------------------------------------------------------------- #

def render_launcher(payload_b64: str, key_hex: str, version: str,
                    build_time: str) -> str:
    text = LAUNCHER_TEMPLATE
    text = text.replace("%%PAYLOAD%%", payload_b64)
    text = text.replace("%%KEY%%", key_hex)
    text = text.replace("%%VERSION%%", version)
    text = text.replace("%%BUILD_TIME%%", build_time)
    return text


def build(out_path: Path) -> Path:
    version = package_version()
    key = secrets.token_bytes(32)
    print(f"survey_flow {version}: encrypting modules...")
    payload = build_payload(key)
    raw_b64 = base64.b64encode(zlib.compress(payload, 9)).decode("ascii")
    # wrap so the file stays editor/viewer friendly (no giant single line)
    blob = "\n".join(raw_b64[i:i + 120] for i in range(0, len(raw_b64), 120))
    print(f"payload: {len(payload) / 1024 / 1024:.1f} MB "
          f"(base64 {len(raw_b64) / 1024 / 1024:.1f} MB)")

    import time
    stamp = time.strftime("%Y-%m-%d %H:%M:%S")
    text = render_launcher(blob, key.hex(), version, stamp)

    # hard guarantee: no readable survey_flow source in the output
    for needle in (b"class Decider", b"class TerminateMemory", b"def analyze",
                   b"MockSurveyHandler"):
        if needle in text.encode("utf-8"):
            raise SystemExit("build aborted: source leaked into the output")

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(text, encoding="utf-8")
    print(f"wrote {out_path} ({out_path.stat().st_size / 1024 / 1024:.1f} MB)")
    return out_path


def verify(out_path: Path) -> None:
    print(f"verifying: {sys.executable} {out_path} selftest")
    res = subprocess.run([sys.executable, str(out_path), "selftest"],
                         cwd=tempfile.gettempdir())
    if res.returncode != 0:
        raise SystemExit(f"verification FAILED (exit {res.returncode})")
    print("verification passed: the single file runs and the self-test passes.")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--out", default=str(ROOT / "dist" / "SurveyFlow.pyw"),
                    help="output file (default: %(default)s)")
    ap.add_argument("--verify", action="store_true",
                    help="run the built file's self-test after building")
    args = ap.parse_args()
    out = build(Path(args.out))
    if args.verify:
        verify(out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
