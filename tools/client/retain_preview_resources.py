"""把经校验且新包缺失的历史资源保留在隔离发版暂存目录。"""

import argparse
import hashlib
import json
from pathlib import Path
import re


ROOT = Path(__file__).resolve().parents[2]
RESOURCE_PATH = re.compile(r'assets/[a-zA-Z0-9_-]+/(?:import|native)/[0-9a-f]{2}/[A-Za-z0-9_.@-]+')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def index(path):
    """读取锁定的 Creator 文件索引。"""
    result = read(path)['build_sha256']
    if not isinstance(result, dict) or 'index.html' not in result:
        raise ValueError('缺少完整 Creator 文件索引')
    return result


def retain(prior, previous, candidate, current):
    """只追加新构建没有引用的旧导入和原生资源，不覆盖当前文件。"""
    if prior == candidate or not candidate.is_relative_to(ROOT / '.work') or candidate == ROOT / '.work':
        raise ValueError('历史资源只允许加入项目内独立候选')
    selected = {}
    for name in sorted(previous.keys() - current.keys()):
        if not RESOURCE_PATH.fullmatch(name):
            continue
        expected = previous[name]
        source, target = prior / name, candidate / name
        if not re.fullmatch(r'[0-9a-f]{64}', expected) or source.resolve(strict=True) != source:
            raise ValueError(f'历史资源摘要或路径无效：{name}')
        data = source.read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise ValueError(f'历史资源摘要或路径无效：{name}')
        if target.resolve() != target:
            raise ValueError(f'暂存历史资源路径有重定向：{name}')
        if target.exists():
            if sha(target) != expected:
                raise ValueError(f'暂存与历史资源的字节不一致：{name}')
        else:
            target.parent.mkdir(parents=True, exist_ok=True)
            temporary = target.with_name(target.name + '.retained-tmp')
            with temporary.open('xb') as output:
                output.write(data)
            temporary.rename(target)
        selected[name] = expected
    return selected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for argument in ('prior-dir', 'prior-index', 'candidate-dir', 'candidate-index', 'report'):
        parser.add_argument('--' + argument, type=Path, required=True)
    parser.add_argument('--prior-extra', type=Path, action='append', default=[])
    args = parser.parse_args()
    previous = index(args.prior_index)
    for path in args.prior_extra:
        extra = read(path)
        rows = extra['files'] if 'files' in extra else {extra['path']: extra['sha256']}
        if any(key in previous and previous[key] != value for key, value in rows.items()):
            raise ValueError('历史扩展文件与原 Creator 索引冲突')
        previous.update(rows)
    files = retain(args.prior_dir.resolve(strict=True), previous, args.candidate_dir.resolve(strict=True),
                   index(args.candidate_index))
    proof = {'files': files, 'retained_files': len(files)}
    args.report.write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'retained_files': len(files)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
