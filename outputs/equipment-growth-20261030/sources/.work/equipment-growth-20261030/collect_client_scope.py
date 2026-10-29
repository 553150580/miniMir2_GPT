"""只读收集本阶段客户端装备UI/元数据，不执行构建或业务。"""
from pathlib import Path
import json

ROOT = Path(__file__).resolve().parents[2]
scene = json.loads((ROOT / 'client/assets/scenes/Game.scene').read_text(encoding='utf-8'))
cache = {}


def node_path(index):
    if index in cache:
        return cache[index]
    node = scene[index]
    parent = node.get('_parent')
    cache[index] = (node_path(parent['__id__']) + '/' if parent else '') + node.get('_name', '')
    return cache[index]


wanted = ['Game/Canvas/Safe/UIMgr/Panels/Inventory', 'Game/Canvas/Safe/UIMgr/Panels/Role',
          'Game/Canvas/Safe/UIMgr/Panels/SkillAssets', 'Game/Canvas/Safe/PopupView/Panels/SkillQuote']
out = []
for index, node in enumerate(scene):
    if node.get('__type__') != 'cc.Node':
        continue
    path = node_path(index)
    if not any(path == root or path.startswith(root + '/') and path.count('/') <= root.count('/') + 2 for root in wanted):
        continue
    out.append({'index': index, 'path': path, 'position': node.get('_lpos'), 'active': node.get('_active'),
                'components': [{'index': ref['__id__'], 'type': scene[ref['__id__']].get('__type__'),
                                'size': scene[ref['__id__']].get('_contentSize'),
                                'text': scene[ref['__id__']].get('_string')}
                               for ref in node.get('_components', [])]})
gameplay = json.loads((ROOT / 'client/assets/resources/development/gameplay.json').read_text(encoding='utf-8'))
print(json.dumps({'nodes': out, 'jobs': gameplay['jobs']}, ensure_ascii=False, indent=2))
