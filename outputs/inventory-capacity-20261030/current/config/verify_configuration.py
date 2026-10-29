"""核对stage21唯一单格变更与全部同源派生；不修改正式输入。"""
from copy import deepcopy
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[3]
WORK = Path(__file__).resolve().parent


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


baseline, candidate, applied = [load(WORK / name) for name in ['baseline.json', 'candidate-review.json', 'application.json']]
assert baseline['head'] == '5b6be439c013d16c4ca0bf0372c294e47b29a141'
assert candidate['changed_cells'] == 1 and candidate['only_profile_rule_20_to_21']
assert candidate['formulas_preserved'] == 460 and candidate['untouched_zip_members_verified'] == 102
assert applied['applied'] and applied['after_sha256'] == candidate['candidate_sha256']
book = next(name for name in baseline['files'] if name.endswith('/玩法配置.xlsx'))
architecture = next(name for name in baseline['files'] if name.endswith('/架构配置.xlsx'))
assert sha(ROOT / book) == candidate['candidate_sha256']
for name in [architecture, 'config/schema/catalog.json', 'tools/resources/prepare_first_loop.py', 'tools/resources/prepare_pet_runtime.py']:
    assert sha(ROOT / name) == baseline['files'][name], name

fixture = load(ROOT / 'config/development/first-loop.json')
public = load(ROOT / 'client/assets/resources/development/gameplay.json')
assert fixture['rule_version'] == public['rule_version'] == 21
for name, evidence in [('config/development/first-loop.json', '_source_evidence'),
                       ('config/development/pet-runtime-candidates.json', 'source_evidence'),
                       ('config/development/summon-candidates.json', 'source_evidence')]:
    before, after = [load(path) for path in [WORK / 'before' / name, ROOT / name]]
    before.pop(evidence)
    after.pop(evidence)
    if name.endswith('first-loop.json'):
        before['rule_version'] = 21
    assert before == after, '派生业务数值发生额外变化：' + name
old_public = load(WORK / 'before/client/assets/resources/development/gameplay.json')
old_public['rule_version'] = 21
assert old_public == public
assert len(fixture['skills']) == 22 and all(row['id'] != 17 for row in fixture['skills'])
assert fixture['casting_supply']['enabled'] is False
assert fixture['pet_runtime']['bootstrap_mode'] == 'legacy_untrained_pet'
assert fixture['progression']['exp_per_level'] == load(WORK / 'before/config/development/first-loop.json')['progression']['exp_per_level']
assert fixture['_source_evidence']['workbook']['sha256'] == sha(ROOT / book)
summon = load(ROOT / 'config/development/summon-candidates.json')
assert summon['runtime_enabled'] is False and summon['required_rule_version'] == 17
assert summon['source_evidence']['generator_sha256'] == sha(ROOT / 'tools/resources/prepare_summon_candidates.py')

pointer = load(ROOT / 'config/releases/preview_current.json')
version = pointer['version']
assert version == 'dev-inventory-capacity-v0.21-ed0b55518991'
release = ROOT / 'config/releases' / pointer['path']
manifest = load(release / 'manifest.json')
assert manifest['version'] == version
for name, value in manifest['files'].items():
    assert sha(release / name) == value
sync = load(ROOT / 'conf/toolchain/config-binding-sync.json')
assert sync['config_version'] == version and sync['generated_only'] is True
for name in ['server/internal/configtypes/config_types.go', 'client/assets/scripts/generated/config_types.ts']:
    assert subprocess.check_output(['git', 'show', 'HEAD:' + name], cwd=ROOT) == (ROOT / name).read_bytes(), name

go_tests = ['server/internal/devconfig/' + name for name in ['map_catalog_test.go', 'pet_runtime_test.go', 'summon_test.go']]
for name in go_tests:
    before = (WORK / 'before' / name).read_bytes()
    assert before.count(b'.RuleVersion != 20') == 1
    assert before.replace(b'.RuleVersion != 20', b'.RuleVersion != 21') == (ROOT / name).read_bytes(), name

commands = []
for name in ['candidate', 'apply', 'export-check', 'pet-generate', 'summon-generate', 'fixture-generate', 'export',
             'pet-check', 'summon-check', 'fixture-check', 'python-full', 'sync-bindings', 'verify-bindings']:
    report = load(WORK / (name + '.json'))
    assert report['exit_code'] == 0
    for field in ['stdout', 'stderr']:
        assert sha(WORK / report[field]) == report[field + '_sha256']
    commands.append({'name': name, 'report': name + '.json', 'exit_code': report['exit_code'], 'seconds': report['seconds']})
text = (WORK / 'python-full.stderr.txt').read_text(encoding='utf-8')
match = re.search(r'^Ran (\d+) tests? in ', text, re.M)
assert match and int(match[1]) == 120 and re.search(r'^OK\s*$', text, re.M)
paths = set(baseline['files']) | {'conf/toolchain/config-binding-sync.json', 'server/internal/configtypes/config_types.go',
                                 'client/assets/scripts/generated/config_types.ts'}
paths |= {path.relative_to(ROOT).as_posix() for path in release.iterdir() if path.is_file()}
review = {'status': 'configuration_complete_and_frozen', 'recorded_at_utc': datetime.now(timezone.utc).isoformat(),
          'rule_version': 21, 'preview': version, 'python_tests': 120,
          'changed_workbook_cells': 1, 'only_profile_rule_20_to_21': True, 'formulas_preserved': 460,
          'untouched_zip_members_verified': 102, 'architecture_workbook_preserved': True, 'schema_unchanged': True,
          'all_runtime_gameplay_values_other_than_rule_version_unchanged': True,
          'independent_candidate_payloads_unchanged': True, 'go_ts_generated_type_bytes_unchanged': True,
          'sync_record_matches_new_release': True, 'main_skill_count': 22, 'summon17_enabled': False,
          'casting_supply_enabled': False, 'bootstrap_mode': 'legacy_untrained_pet',
          'summon_candidate_required_rule_version': 17, 'summon_candidate_runtime_enabled': False,
          'legacy_rule20_contract_tests_unchanged': True, 'current_devconfig_assertions_changed': go_tests,
          'commands': commands, 'current_hashes': {name: sha(ROOT / name) for name in sorted(paths)},
          'scope': '配置/投影/类型一致性及Python校验，不表示stage21满包业务、Creator、GUI、Mongo或容量验收'}
(WORK / 'final-review.json').write_text(json.dumps(review, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({k: review[k] for k in ['status', 'rule_version', 'preview', 'python_tests', 'changed_workbook_cells', 'formulas_preserved', 'untouched_zip_members_verified']}, ensure_ascii=False))
print(json.dumps({key: review['current_hashes'][key] for key in ['config/development/first-loop.json', 'client/assets/resources/development/gameplay.json', book]}, ensure_ascii=False))
