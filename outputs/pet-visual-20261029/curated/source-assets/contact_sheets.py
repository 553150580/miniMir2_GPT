"""将来源原格无缩放排成只读审查接触表，不生成正式美术。"""
import hashlib
import importlib.util
import json
from pathlib import Path
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("wil", ROOT / "tools/resources/inspect_wil.py")
wil_tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(wil_tool)
audit = json.loads((OUT / "audit.json").read_text(encoding="utf-8"))
outputs = []
for package, start in [("Magic", 1500), ("Mon4", 2860)]:
    source = next(r for r in audit["frame_ranges"] if r["package"] == package)
    wil, wix = (Path(source["source_paths"][key]) for key in ("wil", "wix"))
    before = [wil_tool.sha256_file(p) for p in (wil, wix)]
    probe = wil_tool.probe(wil, wix)
    image = Image.new("RGBA", (1000, 420), (42, 44, 48, 255))
    draw = ImageDraw.Draw(image)
    draw.text((10, 8), f"{package} raw {start}..{start+9} / native RGBA / audit only", fill="white")
    for i in range(10):
        frame = probe["frames"][start + i]
        rgba = wil_tool.decode_frame(wil, frame, probe["palette"])
        original = Image.frombytes("RGBA", (frame["width"], frame["height"]), rgba)
        x, y = (i % 5) * 200, 30 + (i // 5) * 190
        origin = (x + 80, y + 110)
        draw.line((origin[0] - 5, origin[1], origin[0] + 5, origin[1]), fill=(170, 170, 170, 255))
        draw.line((origin[0], origin[1] - 5, origin[0], origin[1] + 5), fill=(170, 170, 170, 255))
        image.alpha_composite(original, (origin[0] + frame["offset_x"], origin[1] + frame["offset_y"]))
        draw.text((x + 8, y + 155), f"{start+i}: {frame['width']}x{frame['height']} offset {frame['offset_x']},{frame['offset_y']}", fill="white")
    path = OUT / f"{package.lower()}-{start}-native-contact.png"
    image.save(path)
    assert before == [wil_tool.sha256_file(p) for p in (wil, wix)]
    outputs.append({"path": str(path.relative_to(ROOT)), "sha256": hashlib.sha256(path.read_bytes()).hexdigest(), "native_scale": True, "source_hashes_unchanged": True})
(OUT / "contact-sheets.json").write_text(json.dumps(outputs, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
print(json.dumps(outputs, ensure_ascii=False))
