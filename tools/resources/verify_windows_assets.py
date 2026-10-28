"""从摘要相同的只读原包再生成到临时目录，比较既有PNG和领域JSON；不改原资产元数据。"""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import argparse

ROOT = Path(__file__).resolve().parents[2]
ASSETS = ROOT / 'client/assets/resources/development'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report',default='.work/resource-regeneration-check.json')
    args=parser.parse_args()
    original = json.loads((ASSETS / 'manifest.json').read_text(encoding='utf-8'))
    source = Path(os.environ.get('MINIMIR_CLIENT_180_ROOT', 'D:/Mir2/mir2_gxx/assets/legacy/primary_180'))
    if sha(source / 'Map/0.map') != original['map']['source_sha256']:
        raise RuntimeError('Windows原地图与已登记来源摘要不一致')
    for package in original['sources']:
        for kind in ('wil', 'wix'):
            actual = source / 'Data' / Path(package[kind + '_path']).name
            if not actual.is_file() or sha(actual) != package[kind + '_sha256']:
                raise RuntimeError('Windows原包摘要不符：' + str(actual))
    temporary = ROOT / '.work/windows-migration/regenerated-assets'
    cache = temporary / 'manifest.json'
    cached = json.loads(cache.read_text(encoding='utf-8')) if cache.is_file() else {}
    cache_valid = cached.get('converter_sha256') == sha(ROOT / 'tools/resources/build_development_assets.py') and cached.get('wil_decoder_sha256') == sha(ROOT / 'tools/resources/inspect_wil.py')
    if not cache_valid:
        result = subprocess.run([sys.executable, str(ROOT / 'tools/dev'), 'python3', 'tools/resources/build_development_assets.py',
                                 '--source-root', str(source), '--output', str(temporary)], cwd=ROOT, capture_output=True)
        if result.returncode:
            raise RuntimeError(result.stderr.decode('utf-8', errors='replace')[-2500:])
    regenerated = json.loads((temporary / 'manifest.json').read_text(encoding='utf-8'))
    # 保留原转换器摘要和Mac原路径作为历史来源；只对这些已明确登记的元数据差异分别报告。
    def domain(manifest):
        value = json.loads(json.dumps(manifest))
        value.pop('converter_sha256', None)
        for package in value['sources']:
            package.pop('wil_path', None)
            package.pop('wix_path', None)
        return value
    errors = []
    if domain(original) != domain(regenerated):
        errors.append('再生成领域manifest存在非来源路径/转换器摘要差异')
    pngs = 0
    for key, frame in original['frames'].items():
        if frame['png']:
            relative = frame['png']
            if sha(ASSETS / relative) != frame['png_sha256'] or sha(temporary / relative) != frame['png_sha256']:
                errors.append('PNG字节发生变化：' + relative)
            pngs += 1
    for relative in ('maps/first-map.json', 'maps/first-map.png'):
        if relative.endswith('.json'):
            prior = json.loads((ASSETS / relative).read_text(encoding='utf-8'))
            current = json.loads((temporary / relative).read_text(encoding='utf-8'))
            # 原始地图摘要已在入口核验；仅原机绝对路径不同，旧JSON继续原字节保留。
            prior.pop('source_path')
            current.pop('source_path')
            equal = prior == current
        else:
            equal = sha(ASSETS / relative) == sha(temporary / relative)
        if not equal:
            errors.append('地图领域文件发生变化：' + relative)
    report = dict(passed=not errors, verified_pngs=pngs, errors=errors, raw_package_count=len(original['sources']),
                  source_root=str(source), original_converter_sha256=original['converter_sha256'],
                  regeneration_cache_reused=cache_valid, map_source_path_remapped_only=True,
                  regeneration_converter_sha256=regenerated['converter_sha256'], original_manifest_preserved=True,
                  source_packages_modified=False, official_180_restoration_verified=False, runtime_ready=False)
    path = (ROOT / args.report).resolve()
    if not path.is_relative_to(ROOT):raise ValueError('报告必须位于当前项目内')
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps(report, ensure_ascii=False))
    return 1 if errors else 0


if __name__ == '__main__':
    raise SystemExit(main())
