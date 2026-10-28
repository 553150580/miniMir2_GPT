"""原位置不可访问时，使用清单中逐字节登记的独立副本；摘要变化仍拒绝。"""
import hashlib
import json
from pathlib import Path


def resolve_locked_source(root, reference):
    original = root / reference['path']
    if original.exists():
        return original
    manifest = root / '资料索引/本地参考副本.json'
    if not manifest.is_file():
        return original
    for record in json.loads(manifest.read_text(encoding='utf-8')).get('files', []):
        if record.get('source_sha256') != reference['sha256'] or record.get('target_sha256') != reference['sha256']:
            continue
        candidate = root / record['target_relative_path']
        if candidate.is_symlink() or not candidate.resolve().is_relative_to(root.resolve()) or not candidate.is_file():
            continue
        if hashlib.sha256(candidate.read_bytes()).hexdigest() == reference['sha256']:
            return candidate
    return original
