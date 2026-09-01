"""Copy .grok/skills/beach* to skills/ with public-tree doc links."""
from pathlib import Path
import shutil

root = Path(__file__).resolve().parents[1]
src = root / ".grok" / "skills"
dst = root / "skills"
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
        t = t.replace("../../../docs", "../../docs")
        t = t.replace("../../../../docs", "../../docs")
        t = t.replace(" or `cmd/hush`", "")
        t = t.replace("(`cmd/examples/…` or `cmd/hush`)", "(`cmd/examples/…`)")
        t = t.replace("cmd/hush", "cmd/examples")
        out.write_text(t, encoding="utf-8", newline="\n")
        print(out)
