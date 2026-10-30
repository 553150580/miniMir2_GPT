"""仅迁移当前配置版本断言，并清除历史全夹具中的未来仓储合同。"""
from pathlib import Path
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
FILES = [
    'server/internal/devconfig/equipment_compat_test.go',
    'server/internal/devconfig/map_catalog_test.go',
    'server/internal/devconfig/summon_test.go',
    'server/internal/devconfig/pet_runtime_test.go',
    'server/internal/integration/equipment_test.go',
    'server/internal/integration/level_growth_test.go',
    'server/internal/integration/summon_pet_test.go',
    'server/internal/game/equipment_test.go']


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    report_path = WORK / 'current-go-test-adaptations.json'
    assert not report_path.exists()
    records = []
    for name in FILES:
        path = ROOT / name
        raw = path.read_bytes()
        assert b'RuleVersion != 22' in raw, name
        updated = raw.replace(b'RuleVersion != 22', b'RuleVersion != 23')
        updated = updated.replace('必须保持主rule22'.encode(), '必须保持主rule23'.encode())
        path.write_bytes(updated)
        records.append({'path': name, 'before_sha256': digest(raw), 'after_sha256': digest(updated),
                        'changes': '当前配置版本22→23；原装备/成长/关闭候选断言保持'})
    name = 'server/internal/integration/binding_test.go'
    path = ROOT / name
    raw = path.read_bytes()
    marker = b'\tf.RuleVersion = 12\n'
    newline = b'\r\n' if b'\r\n' in raw else b'\n'
    marker = marker.replace(b'\n', newline)
    assert raw.count(marker) == 1
    updated = raw.replace(marker, marker + '\tf.Warehouses = nil // 历史rule12不携带rule23仓储合同。'.encode() + newline)
    path.write_bytes(updated)
    records.append({'path': name, 'before_sha256': digest(raw), 'after_sha256': digest(updated),
                    'changes': '历史完整Validate夹具明确移除未来仓储字段；保留所有原校验'})
    report_path.write_text(json.dumps({'applied': True, 'files': records}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'applied': True, 'files': len(records)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
