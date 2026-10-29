"""只读记录本任务实际HTTP健康身份，不记录登录凭据。"""
import argparse, datetime, hashlib, json
from pathlib import Path
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument('label', choices=['before', 'after'])
args = parser.parse_args()
target = WORK / ('runtime-' + args.label + '.json')
assert not target.exists()
report = {'recorded_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'label': args.label,
          'ports': {}, 'scope': '开发实例身份及实际HTTP可达性，非业务或容量验收'}
for port in [18080, 18100, 18101, 18102]:
    with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=5) as response:
        report['ports'][str(port)] = json.load(response)
report['current_disk_fixture_sha256'] = hashlib.sha256((ROOT / 'config/development/first-loop.json').read_bytes()).hexdigest()
report['current_disk_public_sha256'] = hashlib.sha256((ROOT / 'client/assets/resources/development/gameplay.json').read_bytes()).hexdigest()
report['loaded_runtime_matches_current_disk_fixture'] = all(report['ports'][str(port)]['fixture_hash'] == report['current_disk_fixture_sha256'] for port in [18100, 18101, 18102])
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'label': args.label, 'backend_rules': {port: health.get('rule_version') for port, health in report['ports'].items() if port != '18080'}, 'loaded_runtime_matches_current_disk_fixture': report['loaded_runtime_matches_current_disk_fixture'], 'report': str(target)}, ensure_ascii=False))
