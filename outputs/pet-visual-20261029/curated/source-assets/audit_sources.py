"""只读核对召唤17原资源；仅将元数据写入本次.work证据目录。"""

from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import wave


ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent


def sha(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def main() -> None:
    decoder_path = ROOT / "tools/resources/inspect_wil.py"
    spec = importlib.util.spec_from_file_location("inspect_wil_audit", decoder_path)
    decoder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(decoder)
    manifest_path = ROOT / "client/assets/resources/development/manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    gameplay_path = ROOT / "client/assets/resources/development/gameplay.json"
    gameplay = json.loads(gameplay_path.read_text(encoding="utf-8"))
    files = [
        "资料参考/crystal/Server/MirObjects/Monsters/BoneFamiliar.cs",
        "资料参考/crystal/Server/MirObjects/MapObject.cs",
        "资料参考/crystal/Client/MirObjects/MonsterObject.cs",
        "资料参考/crystal/Client/MirObjects/PlayerObject.cs",
        "资料参考/crystal/Client/MirObjects/Frames.cs",
        "资料参考/crystal/Client/MirObjects/Effect.cs",
        "资料参考/crystal/Client/MirGraphics/MLibrary.cs",
        "资料参考/crystal/Client/MirSounds/SoundManager.cs",
        "资料参考/crystal/Client/MirSounds/SoundList.cs",
        "资料参考/crystal/Client/MirSounds/Libraries/CachedSound.cs",
        "资料参考/crystal/Shared/Enums.cs",
        "tools/resources/build_development_assets.py",
        "tools/resources/inspect_wil.py",
        "client/assets/scripts/app/WorldAssets.ts",
        "client/assets/scripts/app/GameWorldView.ts",
        "client/assets/resources/development/manifest.json",
        "client/assets/resources/development/gameplay.json",
    ]
    result = {
        "scope": "只读来源和现有资源结构审查；未运行来源程序/DB/Creator，未生成美术或写入正式资源",
        "source_files": [{"path": p, "sha256": sha(ROOT / p)} for p in files],
        "current": {
            "manifest_frame_count": len(manifest["frames"]),
            "effect_ids": list(manifest["effects"]),
            "effect17_present": "17" in manifest["effects"],
            "pet_skeleton": manifest["profiles"]["pet_skeleton"],
            "gameplay_skill_ids": [s["id"] for s in gameplay["skills"]],
            "audio_files": sorted(str(p.relative_to(ROOT)) for p in (ROOT / "client/assets").rglob("*") if p.is_file() and p.suffix.lower() in {".wav", ".ogg", ".mp3"}),
        },
        "frame_ranges": [],
    }
    sources = {item["package"]: item for item in manifest["sources"]}
    for package, ranges in [("Magic", [(1500, 1510), (1510, 1520)]), ("Mon4", [(2860, 2870), (2870, 2880)])]:
        source = sources[package]
        wil = Path(source["wil_path"])
        wix = Path(source["wix_path"])
        before = {"wil_sha256": sha(wil), "wix_sha256": sha(wix)}
        probe = decoder.probe(wil, wix)
        for first, stop in ranges:
            frames = []
            for index in range(first, stop):
                meta = probe["frames"][index]
                rgba = decoder.decode_frame(wil, meta, probe["palette"])
                frames.append({
                    **{k: v for k, v in meta.items() if k != "png_path"},
                    "rgba_sha256": hashlib.sha256(rgba).hexdigest(),
                    "visible_alpha_pixels": sum(value != 0 for value in rgba[3::4]),
                    "manifest_present": f"{package}:{index}" in manifest["frames"],
                })
            after = {"wil_sha256": sha(wil), "wix_sha256": sha(wix)}
            assert before == after
            assert before["wil_sha256"] == source["wil_sha256"]
            assert before["wix_sha256"] == source["wix_sha256"]
            result["frame_ranges"].append({
                "package": package, "first_raw_frame": first, "stop_exclusive": stop,
                "source_paths": {"wil": str(wil), "wix": str(wix)},
                "before": before, "after": after,
                "matches_existing_manifest_source": True,
                "format": probe["format_id"], "header": probe["header"],
                "frames": frames,
            })
    wav_root = Path("D:/Mir2/mir2_gxx/assets/legacy/primary_180/Wav")
    sound_candidates = []
    for name in ("M17-1.wav", "M17-3.wav"):
        path = wav_root / name
        before = sha(path)
        with wave.open(str(path), "rb") as stream:
            metadata = {"channels": stream.getnchannels(), "sample_width_bytes": stream.getsampwidth(), "sample_rate": stream.getframerate(), "frame_count": stream.getnframes(), "compression": stream.getcomptype()}
            metadata["duration_seconds"] = metadata["frame_count"] / metadata["sample_rate"]
        assert before == sha(path)
        sound_candidates.append({"path": str(path), "sha256": before, **metadata, "identity": "仅现存文件；未证明是Crystal65起手或BoneFamiliar出场声音"})
    crystal_client = Path("D:/123DownLoad/CrystalLatest/Client")
    result["unresolved"] = {
        "crystal_client_lib_count": len(list(crystal_client.rglob("*.Lib"))),
        "crystal_lib_scope": str(crystal_client),
        "default_cast_sound": {"spell_enum": 65, "index": 20650, "filename_stem": "M65-0", "files": [p.name for p in wav_root.glob("M65-0.*")]},
        "default_born_sound": {"monster_enum": 78, "index": 785, "filename_stem": "078-5", "files": [p.name for p in wav_root.glob("078-5.*")]},
        "sound_list_path": str(wav_root / "SoundList.lst"),
        "sound_list_present": (wav_root / "SoundList.lst").exists(),
        "other_unverified_wav_files": sound_candidates,
        "appear": "Mon4:2860..2869可解码但未获得BoneFamiliar Lib的Appear动作表；start/count/direction-stride/interval未认证，不能写成原动作已验证",
    }
    for entry in result["source_files"]:
        assert sha(ROOT / entry["path"]) == entry["sha256"]
    result["source_files_unchanged_after_audit"] = True
    output = OUT / "audit.json"
    output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(json.dumps({"audit": str(output), "source_files": len(files), "ranges": [{"package": r["package"], "start": r["first_raw_frame"], "count": len(r["frames"]), "visible": sum(f["visible_alpha_pixels"] > 0 for f in r["frames"])} for r in result["frame_ranges"]], "source_unchanged": True}, ensure_ascii=False))


if __name__ == "__main__":
    main()
