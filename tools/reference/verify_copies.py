"""迁移后只核验已登记的独立副本；原位置不可访问不冒称重新扫描原始资料。"""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report', default='.work/reference-copies-check.json')
    args = parser.parse_args()
    manifest = json.loads((ROOT / '资料索引/本地参考副本.json').read_text(encoding='utf-8'))
    errors, checked, originals, seen = [], 0, 0, set()
    source_manifest = ROOT / manifest['source_manifest_path']
    if not source_manifest.is_file() or sha(source_manifest) != manifest['source_manifest_sha256']:
        errors.append('历史来源清单摘要不符')
    for record in manifest['files']:
        relative = record['target_relative_path']
        target = ROOT / relative
        if relative in seen or not target.resolve().is_relative_to(ROOT) or target.is_symlink() or not target.is_file():
            errors.append('副本目标重复、越界或不存在：' + relative)
            continue
        seen.add(relative)
        if target.stat().st_nlink != 1 or target.stat().st_size != record['bytes'] or sha(target) != record['target_sha256'] or record['target_sha256'] != record['source_sha256']:
            errors.append('副本字节或独立性不符：' + relative)
            continue
        checked += 1
        original = Path(record['source_root']) / record['source_relative_path']
        if original.is_file():
            if sha(original) != record['source_sha256']:
                errors.append('可访问原始文件摘要不符：' + str(original))
            else:
                originals += 1
    if checked != manifest['copied_file_count'] or sum(record['bytes'] for record in manifest['files']) != manifest['copied_bytes']:
        errors.append('副本清单统计不符')
    report = dict(passed=not errors, checked_copies=checked, accessible_originals_reverified=originals,
                  errors=errors, original_source_tree_rescanned=False, source_rules_certified=False, runtime_ready=False)
    target = (ROOT / args.report).resolve()
    if not target.is_relative_to(ROOT):
        raise RuntimeError('报告路径必须在本项目内')
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps(report, ensure_ascii=False))
    return 0 if not errors else 1


if __name__ == '__main__':
    raise SystemExit(main())
