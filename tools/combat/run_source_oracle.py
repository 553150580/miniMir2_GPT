#!/usr/bin/env python3
"""逐字编入Crystal公式和默认定义，用独立.NET实测并输出Go可复用向量。"""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import shutil
import struct
import sys
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / '.work/combat-modes/oracle'
MAGIC = ROOT / '资料参考/crystal/Server/MirDatabase/MagicInfo.cs'
ENVIR = ROOT / '资料参考/crystal/Server/MirEnvir/Envir.cs'
LOCKS = {
    MAGIC: '08fa4ef733adcfff800d39fd7071936e5ab1e201bd65b55b7b662488164a5e13',
    ENVIR: '9c39ad15452eef9e98323329860b5c86ed57f1a44114d22749c88b07b1756db7',
}
IDS = {1, 2, 3, 4, 5, 7, 9, 10, 11, 12, 23, 24, 25, 26, 29, 33}
BASES = [0, 1, 2, 5, 10, 15, 20, 45, 85, 90, 100, 255, 1024, 32767, 65535, 16777215, 16777216]


def save(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def f32(value):
    return struct.unpack('<f', struct.pack('<f', value))[0]


def bits(value):
    return struct.unpack('<I', struct.pack('<f', value))[0]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go-fixture', type=Path, help='另存13字段确定性gzip回归向量；不作为运行配置')
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)
    for path, digest in LOCKS.items():
        if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise SystemExit('来源摘要变化，先复核：' + str(path))
    mapping = json.loads((ROOT / '资料索引/三职业技能来源映射.json').read_text())
    choices = {r['myf']['id']: r['crystal']['symbol'] for r in mapping['skills'] if r['myf']['id'] in IDS}
    if set(choices) != IDS:
        raise SystemExit('测试候选映射不完整')
    lines = MAGIC.read_text(encoding='utf-8-sig').splitlines(keepends=True)
    source = ENVIR.read_text(encoding='utf-8-sig')
    factories, source_locations, requests = [], [], []
    for skill, symbol in sorted(choices.items()):
        marker = source.index('if (!MagicExists(Spell.' + symbol + '))')
        start = source.index('new MagicInfo', marker)
        opening = source.index('{', start)
        depth, end = 0, opening
        for end in range(opening, len(source)):
            depth += (source[end] == '{') - (source[end] == '}')
            if depth == 0:
                break
        initializer = source[start:end + 1]
        factories.append(f'        {skill} => {initializer},')
        source_locations.append({'skill_id': skill, 'symbol': symbol,
            'start_line': source.count('\n', 0, start) + 1, 'end_line': source.count('\n', 0, end) + 1})
        values = dict(re.findall(r'\b(MPowerBonus|PowerBonus)\s*=\s*(\d+)', initializer))
        for rank in range(4):
            for base in BASES:
                # 两个来源随机项全枚举，不把最终威力区间错当均匀随机。
                for mpower in range(max(1, int(values.get('MPowerBonus', 0)))):
                    for power in range(max(1, int(values.get('PowerBonus', 0)))):
                        requests.append({'SkillId': skill, 'Rank': rank, 'Base': base,
                            'MPowerRoll': mpower, 'PowerRoll': power})
    project = OUT / 'project'
    project.mkdir(exist_ok=True)
    # 只补最小类型和确定性Random桩，公式及默认initializer不做字符串替换。
    compiled = ('using System;\npublic enum Spell {' + ','.join(choices.values()) + '}\npublic sealed class MagicInfo {\n'
        + ''.join(lines[12:22]) + '\n}\npublic sealed class UserMagic { public MagicInfo Info = null!; public byte Level;\n'
        + ''.join(lines[179:222]) + '\n}\npublic static class SourceFactory { public static MagicInfo Create(int id) => id switch {\n'
        + '\n'.join(factories) + '\n_ => throw new ArgumentOutOfRangeException(nameof(id)) }; }\n')
    (project / 'SourceFormula.cs').write_text(compiled, encoding='utf-8')
    shutil.copyfile(ROOT / 'tools/combat/OracleDriver.cs', project / 'OracleDriver.cs')
    (project / 'Oracle.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable><CheckForOverflowUnderflow>false</CheckForOverflowUnderflow></PropertyGroup></Project>\n')
    (project / 'NuGet.Config').write_text('<configuration><packageSources><clear /></packageSources></configuration>\n')
    lock = json.loads((ROOT / 'conf/toolchain/dotnet.lock.json').read_text())
    (project / 'global.json').write_text(json.dumps({'sdk': {'version': lock['version'], 'rollForward': 'disable'}}))
    save(OUT / 'requests.json', requests)
    commands = [
        [sys.executable,str(ROOT / 'tools/dev'), '--cwd', str(project.relative_to(ROOT)), 'dotnet', 'restore', '--configfile', 'NuGet.Config'],
        [sys.executable,str(ROOT / 'tools/dev'), '--cwd', str(project.relative_to(ROOT)), 'dotnet', 'build', '--no-restore', '--configuration', 'Release'],
        [sys.executable,str(ROOT / 'tools/dev'), 'dotnet', str(project / 'bin/Release/net10.0/Oracle.dll'), str(OUT / 'requests.json'), str(OUT / 'csharp-results.jsonl')],
    ]
    with (OUT / 'execution.log').open('w', encoding='utf-8') as log:
        for command in commands:
            result = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT)
            if result.returncode:
                raise SystemExit('C#构建或执行失败，见' + str(OUT / 'execution.log'))
    vectors, errors, ppm_differences = [], [], []
    for line in (OUT / 'csharp-results.jsonl').read_text().splitlines():
        v = json.loads(line)
        mpower = v['mpower_base'] + v['mpower_roll']
        power = v['power_base'] + v['power_roll']
        computed_power = round(f32(f32(f32(mpower / 4) * (v['rank'] + 1)) + power))
        multiplier = f32(f32(v['multiplier_base']) + f32(v['rank'] * f32(v['multiplier_bonus'])))
        computed = int(f32(f32(v['base'] + computed_power) * multiplier))
        if (computed_power, computed, bits(multiplier)) != (v['expected_power'], v['expected_damage'], v['multiplier_bits']):
            errors.append(v)
        naive = (v['base'] + computed_power) * (round(v['multiplier_base'] * 1000000) + v['rank'] * round(v['multiplier_bonus'] * 1000000)) // 1000000
        if naive != v['expected_damage']:
            ppm_differences.append({'skill_id': v['skill_id'], 'rank': v['rank'], 'base': v['base'], 'csharp': v['expected_damage'], 'naive_ppm': naive})
        v['multiplier_base_ppm'] = round(v['multiplier_base'] * 1000000)
        v['multiplier_bonus_ppm'] = round(v['multiplier_bonus'] * 1000000)
        vectors.append(v)
    if len(vectors) != len(requests):
        raise SystemExit('C#输出向量数量不匹配')
    counterexamples = [v for v in vectors if v['skill_id'] == 26 and v['rank'] == 3 and v['base'] in [45, 85, 90]]
    if [v['expected_damage'] for v in counterexamples] != [116, 220, 233]:
        raise SystemExit('实际C#反例不符，请审视运行时浮点行为')
    save(OUT / 'vectors.json', vectors)
    save(OUT / 'ppm-differences.json', ppm_differences)
    fixture_hash = None
    if args.go_fixture:
        fields = 'skill_id rank base mpower_roll power_roll mpower_base mpower_bonus power_base power_bonus multiplier_base_ppm multiplier_bonus_ppm expected_power expected_damage'.split()
        payload = json.dumps([{k: v[k] for k in fields} for v in vectors], ensure_ascii=False, separators=(',', ':')).encode()
        encoded = gzip.compress(payload, mtime=0)
        args.go_fixture.parent.mkdir(parents=True, exist_ok=True)
        args.go_fixture.write_bytes(encoded)
        fixture_hash = hashlib.sha256(encoded).hexdigest()
    report = {'status': 'passed' if not errors else 'failed', 'sdk_version': lock['version'], 'platform': lock['platform'],
        'source_files': [{'path': str(p.relative_to(ROOT)), 'sha256': h} for p, h in LOCKS.items()],
        'source_formula_lines': {'fields': [13, 22], 'methods': [180, 222]}, 'default_initializers': source_locations,
        'compiled_source_sha256': hashlib.sha256(compiled.encode()).hexdigest(), 'vector_count': len(vectors),
        'ranks': [0, 1, 2, 3], 'base_inputs': BASES, 'random_terms_exhaustive': True,
        'independent_binary32_port_mismatches': len(errors), 'naive_ppm_difference_count': len(ppm_differences),
        'counterexamples': counterexamples, 'vectors_sha256': hashlib.sha256((OUT / 'vectors.json').read_bytes()).hexdigest(),
        'go_fixture_sha256': fixture_hash,
        'scope': '真实.NET编译逐字源公式和默认initializer；固定抽样逐项验证，不比较完整PRNG时序，不认证目标1.80或来源实库参数',
        'runtime_configuration': False}
    save(OUT / 'report.json', report)
    print(json.dumps({k: report[k] for k in ['status', 'sdk_version', 'platform', 'vector_count', 'independent_binary32_port_mismatches', 'naive_ppm_difference_count']}, ensure_ascii=False))
    if errors:
        raise SystemExit('独立binary32复算不符')


if __name__ == '__main__':
    main()
