"""保存本阶段前态，复制获准的独立来源资产，并生成唯一Excel单格编辑计划。"""
import copy
import hashlib
import json
from pathlib import Path
import shutil
import sys

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / "tools/config"))
import export_configs as config


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")


schema = config.load_json(ROOT / "config/schema/catalog.json")
book = ROOT / next(w["path"] for w in schema["workbooks"] if w["group"] == "gameplay")
tables, errors, _, _ = config.collect(schema)
assert not errors, errors
profile = tables["dev_first_loop_profile"][0]
parameters = copy.deepcopy(profile["parameters"])
assert parameters["rule_version"] == 17
parameters["rule_version"] = 18
baseline_path = OUT / "before.json"
assert not baseline_path.exists(), "前态已保存，禁止覆盖"
paths = [p for p in (ROOT / "client/assets/resources").rglob("*") if p.is_file()]
paths += [p for p in (ROOT / "client/assets").rglob("*") if p.is_file() and p.suffix in {".scene", ".prefab"}]
paths += [book, ROOT / "config/schema/catalog.json", ROOT / "tools/resources/build_development_assets.py", ROOT / "config/development/first-loop.json", ROOT / "config/development/pet-runtime-candidates.json", ROOT / "config/development/summon-candidates.json"]
write(baseline_path, {"scope": "本阶段写入前真实字节", "files": {p.relative_to(ROOT).as_posix(): sha(p) for p in paths}, "profile": profile})
shutil.copyfile(book, OUT / "gameplay-before.xlsx")
shutil.copyfile(ROOT / "client/assets/resources/development/manifest.json", OUT / "manifest-before.json")
shutil.copyfile(ROOT / "tools/resources/build_development_assets.py", OUT / "builder-before.py")
write(OUT / "gameplay-plan.json", {"workbook": book.relative_to(ROOT).as_posix(), "group": "gameplay", "input_workbook_sha256": sha(book), "operations": [{"kind": "update", "table_id": "dev_first_loop_profile", "id": profile["id"], "values": {"parameters": parameters}}]})

sound = Path("D:/123DownLoad/CrystalLocal/downloads/client-resources/Sound")
inputs = [(Path("D:/123DownLoad/GoMir2_DS/.local/crystal-actors-src/Data/Monster/78.Lib"), "Monster/78.Lib", "local_technical_asset_candidate_download_chain_unresolved")]
inputs += [(sound / name, "Sound/" + name, "existing_local_plist_name_length_and_verification_record_not_current_network_authentication") for name in ["M65-0.wav", "078-5.wav", "SoundList.lst"]]
copies = []
for source, relative, trust in inputs:
    target = ROOT / "资料参考/crystal-supplement/assets" / relative
    assert not target.exists(), target
    before = sha(source)
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)
    assert before == sha(source) == sha(target)
    assert target.stat().st_nlink == 1
    copies.append({"source_path": str(source), "local_reference_path": target.relative_to(ROOT).as_posix(), "sha256": before, "bytes": target.stat().st_size, "copy_mode": "independent_byte_copy", "target_st_nlink": 1, "source_hash_unchanged": True, "source_status": trust})

code_paths = ["资料参考/crystal/Client/MirObjects/PlayerObject.cs", "资料参考/crystal/Client/MirObjects/Frames.cs", "资料参考/crystal/Client/MirObjects/Effect.cs", "资料参考/crystal/Client/MirObjects/MonsterObject.cs", "资料参考/crystal/Client/MirGraphics/MLibrary.cs", "资料参考/crystal/Client/MirGraphics/DXManager.cs", "资料参考/crystal/Shared/Enums.cs"]
contract = {"schema_version": 1, "status": "candidate", "is_gameplay_configuration": False, "official_180_restoration_verified": False, "source_programs_executed": False, "source_inputs_read_only": True, "copies": copies, "source_code": [{"path": p, "sha256": sha(ROOT / p)} for p in code_paths], "visual_mapping": {"cast": {"skill_id": 17, "source_spell": "SummonSkeleton", "package": "Magic", "parameters_from": "locked PlayerObject Spell effect and Frames.Player Spell; no separately maintained duration"}, "birth": {"profile": "pet_skeleton", "package": "Mon4", "raw_start": 2860, "source_library": "资料参考/crystal-supplement/assets/Monster/78.Lib", "source_action": "Appear", "parameters_from": "locked Lib FrameSet count/skip/interval/blend; no separately maintained total duration", "mapping_status": "cross_source_candidate_alpha_and_offsets_equal_rgb_differences_at_most_7"}}, "audio_mapping": {"summon_skeleton_cast": {"source": "资料参考/crystal-supplement/assets/Sound/M65-0.wav", "output": "audio/summon_skeleton_cast.wav", "trigger": "accepted_skill_cast"}, "pet_skeleton_born": {"source": "资料参考/crystal-supplement/assets/Sound/078-5.wav", "output": "audio/pet_skeleton_born.wav", "trigger": "authoritative_fresh_birth_only"}}, "download_evidence": {"sound_files": "既有PList Name/Length与verification记录相符；未进行本轮联网认证，PList不含内容签名", "monster_library": "仅找到本地DS技术资产，未找回对应历史下载清单，不将声音PList证据套用到Lib", "record": json.loads((OUT / "source-assets/supplement-audit.json").read_text(encoding="utf-8"))["sound_download_evidence"]}, "scope": "只补来源出生/起手和原声资源；主技能仍22/legacy/materialfalse；健康重观及Recall不重播出生，真实UI/Creator/业务分别验收"}
write(ROOT / "资料索引/宠物出生表现来源契约.json", contract)
write(OUT / "input-copy-report.json", {"copies": copies, "contract_sha256": sha(ROOT / "资料索引/宠物出生表现来源契约.json"), "profile_edit": "仅parameters.rule_version17→18，保持其它业务字段"})
print(json.dumps({"baseline_files": len(paths), "copied_assets": len(copies), "plan": str(OUT / "gameplay-plan.json")}, ensure_ascii=False))
