"""可独立加载的 Creator JSON 导入地址随内容变化。"""

import hashlib
import json
import uuid


def content_meta(logical_path: str, payload: bytes) -> dict:
    """为固定逻辑路径和原始 JSON 字节生成稳定且独立的导入 UUID。"""
    digest = hashlib.sha256(payload).hexdigest()
    asset_id = uuid.uuid5(uuid.NAMESPACE_URL, f'minimir2://creator-json/{logical_path}/{digest}')
    return {'ver': '2.0.1', 'importer': 'json', 'imported': True, 'uuid': str(asset_id),
            'files': ['.json'], 'subMetas': {}, 'userData': {}}


def content_meta_bytes(logical_path: str, payload: bytes) -> bytes:
    """以项目的 UTF-8 格式生成可用于快照校验的元数据字节。"""
    return (json.dumps(content_meta(logical_path, payload), ensure_ascii=False, indent=2) + '\n').encode('utf-8')
