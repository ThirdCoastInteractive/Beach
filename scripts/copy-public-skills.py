"""Copy canonical skills/ into .grok/skills for the local Grok loader.

Author in skills/. This script is private-dev (stripped from public).
"""
from pathlib import Path
import shutil

root = Path(__file__).resolve().parents[1]
src = root / "skills"
dst = root / ".grok" / "skills"
if dst.exists():
    shutil.rmtree(dst)
for name in ("beach", "beach-ui", "beach-a11y", "beach-test"):
    s = src / name
    d = dst / name
    d.mkdir(parents=True)
    for p in s.rglob("*"):
        if p.is_dir():
            continue
        out = d / p.relative_to(s)
        out.parent.mkdir(parents=True, exist_ok=True)
        t = p.read_text(encoding="utf-8")
        t = t.replace("../../docs", "../../../docs")
        out.write_text(t, encoding="utf-8", newline="\n")
        print(out)
