"""只读核对双仓储唯一表与当前投影；不发布数值。"""
from pathlib import Path
import json
import runpy

ROOT = Path(__file__).resolve().parents[2]
module = runpy.run_path(str(ROOT / 'tools/config/export_configs.py'))
book = module['read_xlsx'](ROOT / 'outputs/development-prep-20261026/玩法配置.xlsx')
for name, rows in book.items():
    if '仓' in name or '开发' in name:
        print(json.dumps({'sheet': name, 'rows': rows}, ensure_ascii=False))
release = json.loads((ROOT / 'config/releases/preview_current.json').read_text(encoding='utf-8'))
print(json.dumps({'current_release': release}, ensure_ascii=False))
fixture = json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8'))
print(json.dumps({'fixture_keys': list(fixture)}, ensure_ascii=False))
