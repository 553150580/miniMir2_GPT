"""只读解析补充Lib动作表、来源声音及既有下载元数据。"""
from __future__ import annotations

import gzip
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import struct
import wave

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def record(path):
    path = Path(path)
    return {"path": str(path), "bytes": path.stat().st_size, "sha256": sha(path)}


lib = Path("D:/123DownLoad/GoMir2_DS/.local/crystal-actors-src/Data/Monster/78.Lib")
lib_before = record(lib)
data = lib.read_bytes()
version, image_count, frame_seek = struct.unpack_from("<iii", data)
assert version == 3
indices = struct.unpack_from(f"<{image_count}i", data, 12)
assert all(12 + image_count * 4 <= offset < frame_seek for offset in indices)
frame_count = struct.unpack_from("<i", data, frame_seek)[0]
assert frame_seek + 4 + frame_count * 35 == len(data)
enums = (ROOT / "资料参考/crystal/Shared/Enums.cs").read_text(encoding="utf-8-sig")
enum_body = enums.split("public enum MirAction : byte", 1)[1].split("{", 1)[1].split("}", 1)[0]
actions = [name.strip() for name in enum_body.split(",") if name.strip()]
fields = ["start", "count", "skip", "interval_ms", "effect_start", "effect_count", "effect_skip", "effect_interval_ms"]
frames = []
for index in range(frame_count):
    offset = frame_seek + 4 + index * 35
    frame = {"action_id": data[offset], "action": actions[data[offset]], **dict(zip(fields, struct.unpack_from("<8i", data, offset + 1))), "reverse": bool(data[offset + 33]), "blend": bool(data[offset + 34])}
    frame["direction_stride"] = frame["count"] + frame["skip"]
    frames.append(frame)
appear = next(frame for frame in frames if frame["action"] == "Appear")
assert appear == {"action_id": 27, "action": "Appear", "start": 224, "count": 10, "skip": -10, "interval_ms": 100, "effect_start": 0, "effect_count": 0, "effect_skip": 0, "effect_interval_ms": 0, "reverse": False, "blend": True, "direction_stride": 0}

spec = importlib.util.spec_from_file_location("wil", ROOT / "tools/resources/inspect_wil.py")
wil_tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(wil_tool)
wil = Path("D:/Mir2/mir2_gxx/assets/legacy/primary_180/Data/Mon4.wil")
wix = wil.with_suffix(".WIX")
original_hashes = [sha(wil), sha(wix)]
probe = wil_tool.probe(wil, wix)
pairs = []
for index in range(10):
    lib_index, wil_index = 224 + index, 2860 + index
    offset = indices[lib_index]
    width, height, x, y, shadow_x, shadow_y, shadow, length = struct.unpack_from("<6hBi", data, offset)
    assert not shadow & 128
    bgra = gzip.decompress(data[offset + 17:offset + 17 + length])
    assert len(bgra) == width * height * 4
    rgba = bytearray(bgra)
    rgba[0::4], rgba[2::4] = bgra[2::4], bgra[0::4]
    source = probe["frames"][wil_index]
    original = wil_tool.decode_frame(wil, source, probe["palette"])
    cropped = bytes(rgba[:len(original)])
    difference = [abs(cropped[pixel + channel] - original[pixel + channel]) for pixel in range(0, len(original), 4) if original[pixel + 3] for channel in range(3)]
    pair = {"lib_frame": lib_index, "wil_frame": wil_index, "lib_dimensions": [width, height], "wil_dimensions": [source["width"], source["height"]], "offset": [x, y], "lib_bgra_sha256": hashlib.sha256(bgra).hexdigest(), "width_equal": width == source["width"], "offset_equal": (x, y) == (source["offset_x"], source["offset_y"]), "alpha_exact_after_top_crop": cropped[3::4] == original[3::4], "additional_bottom_rows_zero": not any(rgba[len(original):]), "rgba_exact": cropped == original, "visible_rgb_max_difference": max(difference), "visible_rgb_mean_difference": sum(difference) / len(difference)}
    assert pair["width_equal"] and pair["offset_equal"] and pair["alpha_exact_after_top_crop"] and pair["additional_bottom_rows_zero"]
    pairs.append(pair)

download = Path("D:/123DownLoad/CrystalLocal/downloads")
sound = download / "client-resources/Sound"
plist_path = download / "PList.json"
verification_path = download / "client-resources-verification.json"
plist = json.loads(plist_path.read_text(encoding="utf-8-sig"))
verification = json.loads(verification_path.read_text(encoding="utf-8-sig"))
soundlist = sound / "SoundList.lst"
mapped = {}
for line_number, line in enumerate(soundlist.read_text(encoding="utf-8-sig").splitlines(), 1):
    parts = re.split(":|\t", line.replace(" ", ""))
    if len(parts) > 1 and parts[0].isdigit():
        mapped.setdefault(int(parts[0]), {"line": line_number, "filename": parts[-1]})
sound_entries = []
for name, source_index in [("M65-0.wav", 20650), ("078-5.wav", 785)]:
    path = sound / name
    entry = record(path)
    with wave.open(str(path), "rb") as stream:
        entry.update({"channels": stream.getnchannels(), "sample_width_bytes": stream.getsampwidth(), "sample_rate": stream.getframerate(), "frame_count": stream.getnframes(), "compression": stream.getcomptype()})
        entry["duration_seconds"] = entry["frame_count"] / entry["sample_rate"]
    entry["sound_index"] = source_index
    entry["soundlist_override"] = mapped.get(source_index)
    entry["plist_entry"] = next(e for e in plist if e.get("Name", "").replace("\\", "/") == "Sound/" + name)
    assert entry["bytes"] == entry["plist_entry"]["Length"]
    sound_entries.append(entry)
soundlist_entry = next(e for e in plist if e.get("Name", "").replace("\\", "/") == "Sound/SoundList.lst")
assert soundlist_entry["Length"] == soundlist.stat().st_size
other_root = Path("D:/123DownLoad/miniMiro2/Miro2_GPT/资料参考/crystal/downloads/client-resources/Sound")
alternate_copies = [{"path": str(other_root / name), "matches": sha(other_root / name) == sha(sound / name)} for name in ["M65-0.wav", "078-5.wav", "SoundList.lst"]]
world = Path("D:/123DownLoad/GoMir2/client/assets/world")
world_libraries = [{**record(path), "source": (entry := json.loads(path.read_text(encoding="utf-8-sig"))).get("source"), "top_level_keys": list(entry)} for path in world.rglob("library-*.json")]
result = {
    "scope": "本轮只读技术资产解析；无下载/联网认证，无来源程序执行，无正式资源写入",
    "lib": {**lib_before, "version": version, "image_count": image_count, "frame_seek": frame_seek, "frame_count": frame_count, "frame_entry_bytes": 35, "frame_table_exact_eof": True, "frames": frames, "appear_wil_pairs": pairs, "provenance": "仅在GoMir2_DS的.local/crystal-actors-src中找到本地技术资源；当前CrystalLocal PList下载清单没有该Monster78.Lib，尚未独立证实其历史下载链"},
    "blend_evidence": {**record(ROOT / "资料参考/crystal/Client/MirGraphics/DXManager.cs"), "lines": "353–379", "default_normal_blend": "SourceAlpha, One", "library_caller": "MLibrary.cs:692–709 DrawBlend→SetBlend(true,rate)"},
    "soundlist": {**record(soundlist), "plist_entry": soundlist_entry},
    "sounds": sound_entries,
    "sound_download_evidence": {"plist": record(plist_path), "verification": record(verification_path), "recorded": {key: verification[key] for key in ["CheckedAt", "ManifestUrl", "ResourceRoot", "ExpectedCount", "ValidCount", "MissingCount"]}, "claim_limit": "PList仅记录Name/Length等，非文件内容签名；本轮只核既有本地日志与实际SHA，未重新联网验签"},
    "matching_existing_alternate_copies": alternate_copies,
    "negative_world_lookup": {"path": str(world), "lib_count": len(world_libraries), "libraries": world_libraries, "monster_directory_present": (world / "monsters").exists(), "claim_limit": "只限该world目录；不能据此断言整个硬盘不存在怪物资源"},
    "mapping_status": "Lib动作表已解析；Mon4原格与该Lib透明轮廓/锚点实证相同，RGB仍有<=7差异；采用Mon4原像素属于跨来源候选，不宣称逐字节相同或完整原服验收",
}
assert lib_before == record(lib)
assert original_hashes == [sha(wil), sha(wix)]
result["source_assets_unchanged_after_read"] = True
(OUT / "supplement-audit.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
print(json.dumps({"output": str(OUT / "supplement-audit.json"), "lib_frames": frame_count, "appear": appear, "pairs_alpha_equal": len(pairs), "sounds": len(sound_entries), "source_assets_unchanged": True}, ensure_ascii=False))
