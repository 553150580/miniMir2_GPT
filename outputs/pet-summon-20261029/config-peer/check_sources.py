"""只读验证预览、两份候选及fixture仍引用同一批唯一输入。"""
from pathlib import Path
import hashlib,json

ROOT=Path(__file__).resolve().parents[3]
BASE=Path(__file__).resolve().parent
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
read=lambda path:json.loads(path.read_text(encoding='utf-8'))
preview=read(ROOT/'config/releases/preview_current.json')
release=ROOT/'config/releases'/preview['path']
manifest=read(release/'manifest.json')
assert preview['version']==manifest['version']=='dev-pet-summon-v0.17-9cfc98f544bf'
assert preview['content_sha256']==manifest['content_sha256'] and preview['runtime_ready'] is False
for name,digest in manifest['files'].items():assert sha(release/name)==digest,name
catalog=read(ROOT/'config/development/summon-candidates.json')
pet=read(ROOT/'config/development/pet-runtime-candidates.json')
fixture=read(ROOT/'config/development/first-loop.json')
e=catalog['source_evidence']
for book in e['workbooks']:assert sha(ROOT/book['path'])==book['sha256']
for key,name in [('schema_sha256','config/schema/catalog.json'),('compiler_sha256','tools/resources/prepare_first_loop.py'),
                 ('pet_compiler_sha256','tools/resources/prepare_pet_runtime.py'),('generator_sha256','tools/resources/prepare_summon_candidates.py'),
                 ('source_contract_sha256','资料索引/职业宠物补充来源契约.json')]:assert sha(ROOT/name)==e[key],key
assert manifest['schema_sha256']==e['schema_sha256']
assert pet['source_evidence']['workbooks']==e['workbooks']
assert pet['definitions']==fixture['pet_runtime']['definitions']
assert fixture['_source_evidence']['pet_runtime_inputs']['compiler_sha256']==e['pet_compiler_sha256']
assert catalog['runtime_enabled'] is False and pet['runtime_enabled'] is False
assert len(fixture['skills'])==22 and not any(s['id']==17 for s in fixture['skills'])
assert fixture['casting_supply']['enabled'] is False and fixture['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet'
result={'preview':preview,'preview_file_hashes_match':True,'source_evidence_matches_current_inputs':True,
    'candidate_definition_equals_fixture':True,'main_22_legacy_and_material_closed':True,'pet_schema_version':pet['schema_version'],
    'summon_required_rule_version':catalog['required_rule_version'],
    'artifact_sha256':{name:sha(ROOT/name) for name in ['config/development/first-loop.json','config/development/pet-runtime-candidates.json','config/development/summon-candidates.json','client/assets/resources/development/gameplay.json']},
    'reviewed_go_sha256':{name:sha(ROOT/name) for name in ['server/internal/devconfig/summon.go','server/internal/devconfig/summon_candidates.go',
        'server/internal/devconfig/pet_runtime.go','server/internal/devconfig/summon_test.go','server/internal/devconfig/skill_economy.go','server/internal/integration/summon_pet_test.go']}}
(BASE/'source-evidence-check.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(result,ensure_ascii=False))
