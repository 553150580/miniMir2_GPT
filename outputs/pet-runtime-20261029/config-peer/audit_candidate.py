"""只读交叉核对真实JSON和Go字段；不运行Go、不导出配置。"""
from pathlib import Path
import hashlib
import json
import re

ROOT=Path(__file__).resolve().parents[3]
BASE=Path(__file__).resolve().parent
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
FILES=['server/internal/devconfig/pet_runtime.go','server/internal/devconfig/pet_runtime_test.go',
    'server/internal/devconfig/config.go','server/internal/devconfig/displacement.go',
    'server/internal/game/pet_training.go','server/internal/game/pet_training_test.go',
    'config/development/pet-runtime-candidates.json','config/development/first-loop.json',
    'tools/resources/prepare_pet_runtime.py','tools/resources/prepare_first_loop.py','config/schema/catalog.json',
    'outputs/development-prep-20261026/玩法配置.xlsx','outputs/development-prep-20261026/架构配置.xlsx']
before={name:sha(ROOT/name) for name in FILES}
catalog=json.loads((ROOT/FILES[6]).read_text(encoding='utf-8'))
fixture=json.loads((ROOT/FILES[7]).read_text(encoding='utf-8'))
assert catalog['runtime_enabled'] is False and fixture['pet_runtime']['enabled'] is True
assert catalog['definitions']==fixture['pet_runtime']['definitions']
assert catalog['budgets']==fixture['pet_runtime']['budgets']
assert fixture['rule_version']==16 and fixture['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet'
assert 17 not in [row['id'] for row in fixture['skills']] and fixture['casting_supply']['enabled'] is False
fighter=(ROOT/'server/internal/devconfig/config.go').read_text(encoding='utf-8').split('type Fighter struct {',1)[1].split('\n}',1)[0]
fields=set(re.findall(r'json:"([a-z_]+)"',fighter))
definition=catalog['definitions'][0]
assert set(definition['base'])==fields
assert definition['base']=={key:value for key,value in fixture['pet'].items() if key!='undead'}
assert definition['undead']==fixture['pet']['undead']
maximum=max(definition['training']['maximum_level_by_rank'])
assert [row['level'] for row in definition['growth']]==list(range(maximum+1))
assert all((row['required_experience'] is None)==(row['level']==maximum) for row in definition['growth'])
for book in catalog['source_evidence']['workbooks']:assert sha(ROOT/book['path'])==book['sha256']
assert catalog['source_evidence']['schema_sha256']==sha(ROOT/'config/schema/catalog.json')
assert catalog['source_evidence']['generator_sha256']==sha(ROOT/'tools/resources/prepare_pet_runtime.py')
assert fixture['_source_evidence']['pet_runtime_inputs']['compiler_sha256']==sha(ROOT/'tools/resources/prepare_pet_runtime.py')
after={name:sha(ROOT/name) for name in FILES}
assert before==after
result={'review_mode':'只读静态Go审查及真实候选交叉核对；没有运行Go或修改冻结输入',
    'input_sha256':after,'inputs_stable':True,'base_exactly_matches_exported_go_fighter_fields':True,
    'base_matches_legacy_pet':True,'candidate_and_fixture_same_definitions_and_budgets':True,
    'rule16_explicit_legacy_untrained':True,'skill17_and_material_closed':True,
    'source_evidence_matches_current_compiler_books_schema':True}
(BASE/'candidate-cross-check.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(result,ensure_ascii=False))
