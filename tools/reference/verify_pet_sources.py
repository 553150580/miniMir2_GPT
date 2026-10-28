"""核对职业宠物补充技术源码及独立副本，不读取部署配置或账号数据库。"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def local_file(relative: str) -> Path:
    path = ROOT / relative
    if path.is_symlink() or not path.resolve().is_relative_to(ROOT) or not path.is_file():
        raise ValueError('副本不存在或越界：' + relative)
    return path


def verify() -> dict:
    contract = json.loads(local_file('资料索引/职业宠物补充来源契约.json').read_text(encoding='utf-8'))
    if contract['is_runtime_configuration'] or contract['target_180_verified']:
        raise ValueError('补充来源不能冒充运行配置或完整版本认证')
    globals_record = contract['globals']
    copy = local_file(globals_record['local_reference_path'])
    if copy.stat().st_nlink != 1 or digest(copy) != globals_record['source_sha256'] or digest(copy) != globals_record['target_sha256']:
        raise ValueError('Globals独立副本摘要不符')
    match = re.search(r'\bDataRange\s*=\s*(\d+)\s*;', copy.read_text(encoding='utf-8-sig'))
    if match is None or int(match.group(1)) != globals_record['data_range_cells']:
        raise ValueError('DataRange摘录与副本不符')
    settings = contract['settings']
    if settings['whole_file_copied'] or settings['source_sha256'] != settings['historical_registered_sha256']:
        raise ValueError('Settings来源边界或历史摘要不符')
    snippet = local_file(settings['local_technical_slice'])
    if snippet.stat().st_nlink != 1 or digest(snippet) != settings['technical_slice_sha256']:
        raise ValueError('技术摘录摘要不符')
    core = [row for row in contract['core_file_comparison'] if row['root'] == 'windows_latest']
    for row in core:
        reference = local_file('资料参考/crystal/' + row['relative_path'])
        if not row['byte_equal'] or row['source_sha256'] != row['reference_sha256'] or digest(reference) != row['reference_sha256']:
            raise ValueError('既有核心源码身份不符：' + row['relative_path'])
    # 原技术源码可访问时再核摘要；不从旁边的部署配置推断实际模板或覆盖值。
    accessible = []
    for label, record in [('globals', globals_record), ('settings', settings)]:
        source = Path(record['source_path'])
        if source.is_file():
            if digest(source) != record['source_sha256']:
                raise ValueError('可访问技术源码摘要变化：' + label)
            accessible.append(label)
    return {'passed': True, 'existing_core_files_matched': len(core), 'independent_globals_copy': True,
            'technical_settings_slice_only': True, 'accessible_technical_sources_reverified': accessible,
            'source_contract_sha256': digest(ROOT / '资料索引/职业宠物补充来源契约.json'),
            'operating_configuration_or_account_db_read': False, 'runtime_ready': False, 'target_180_verified': False}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report', default='.work/pet-lifecycle-20261029/supplement-check.json')
    args = parser.parse_args()
    report = verify()
    target = (ROOT / args.report).resolve()
    if not target.is_relative_to(ROOT):
        raise ValueError('报告目录越界')
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps(report, ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
