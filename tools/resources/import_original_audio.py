#!/usr/bin/env python3
"""按锁定清单导入用户提供的原字节 WAV 和 Creator 元数据。"""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import tempfile
import uuid
import wave


ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / '资料索引/原版音频来源.json'
TARGET = ROOT / 'client/assets/resources/audio'


def creator_meta(name):
    value = {
        'ver': '1.0.0', 'importer': 'audio-clip', 'imported': True,
        'uuid': str(uuid.uuid5(uuid.NAMESPACE_URL, f'urn:minimir2-gpt:client/assets/resources/audio/{name}')),
        'files': ['.json', '.wav'], 'subMetas': {}, 'userData': {'downloadMode': 0},
    }
    return (json.dumps(value, indent=2) + '\n').encode('utf-8')


def read_original(source, entry, audio_format):
    name = entry['file']
    path = source / name
    if not path.is_file() or path.stat().st_size != entry['bytes']:
        raise ValueError(f'来源音频缺失或字节数不符：{path}')
    data = path.read_bytes()
    if hashlib.sha256(data).hexdigest() != entry['sha256']:
        raise ValueError(f'来源音频 SHA256 不符：{path}')
    with wave.open(io.BytesIO(data), 'rb') as audio:
        actual = (audio.getcomptype(), audio.getsampwidth() * 8, audio.getframerate(),
                  audio.getnchannels(), audio.getnframes())
    expected = ('NONE', audio_format['bits_per_sample'], audio_format['sample_rate_hz'],
                entry['channels'], entry['frames'])
    if actual != expected:
        raise ValueError(f'来源音频格式与清单不符：{path}')
    return data


def installed_matches(path, expected):
    if path.is_symlink():
        raise ValueError(f'目标文件是符号链接：{path}')
    if not path.exists():
        return False
    if not path.is_file() or path.read_bytes() != expected:
        raise ValueError(f'目标已有不同内容：{path}')
    return True


def install_missing(pending):
    work = ROOT / '.work'
    if not work.resolve().is_relative_to(ROOT):
        raise ValueError('临时目录不在项目内')
    work.mkdir(exist_ok=True)
    TARGET.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='original-audio-', dir=work) as temporary:
        stage = Path(temporary)
        for target, data in pending:
            (stage / target.name).write_bytes(data)
        for target, data in pending:
            # 同一磁盘链接已写完的文件，确保现有资源不会被覆盖。
            try:
                os.link(stage / target.name, target)
            except FileExistsError:
                if not installed_matches(target, data):
                    raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-dir', type=Path, help='用户提供的原客户端 Wav 目录')
    parser.add_argument('--check', action='store_true', help='只核验来源与本地安装')
    args = parser.parse_args()
    try:
        manifest = json.loads(MANIFEST.read_text(encoding='utf-8'))
        if manifest['schema_version'] != 1 or not manifest['assets']:
            raise ValueError('不支持的原版音频清单')
        source = (args.source_dir or Path(manifest['source_dir'])).resolve(strict=True)
        target_dir = TARGET.resolve()
        if (not source.is_dir() or source.is_relative_to(target_dir)
                or target_dir.is_relative_to(source) or not target_dir.is_relative_to(ROOT)):
            raise ValueError('来源与项目音频目录必须相互独立')

        pending = []
        seen = set()
        # 所有源文件和目标冲突均在写入前检查。音频元数据先写，供 Creator 按固定 UUID 导入。
        for entry in manifest['assets']:
            name = entry['file']
            if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*\.wav', name) or name.casefold() in seen:
                raise ValueError(f'清单文件名无效或重复：{name}')
            seen.add(name.casefold())
            data = read_original(source, entry, manifest['audio_format'])
            for filename, content in [(name + '.meta', creator_meta(name)), (name, data)]:
                target = TARGET / filename
                if not installed_matches(target, content):
                    pending.append((target, content))

        if args.check and pending:
            raise ValueError(f'本地缺少 {len(pending)} 个音频或元数据文件，请先运行导入')
        if pending:
            install_missing(pending)
        print(json.dumps({'verified_wav_files': len(seen), 'installed_files': len(pending),
                          'source_dir': str(source), 'target_dir': str(TARGET)}, ensure_ascii=False))
        return 0
    except (OSError, ValueError, KeyError, wave.Error, EOFError) as error:
        parser.exit(2, f'原版音频导入失败：{error}\n')


if __name__ == '__main__':
    raise SystemExit(main())
