"""只适配当前输入版本断言；内置历史规则和其它数值均保持。"""
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[3]
OUT=Path(__file__).resolve().parent
changes=[]
for name in ['test_level_growth.py','test_equipment_recycling.py','test_safe_zone_binding.py','test_pet_runtime.py','test_summon_candidates.py','test_teleport.py']:
    path=ROOT/'tools/config'/name;old=path.read_bytes();new=old
    new=new.replace(b"['rule_version'],21)",b"['rule_version'],22)").replace(b"['rule_version'], 21)",b"['rule_version'], 22)")
    if name=='test_summon_candidates.py':
        new=new.replace(b'for version in (17,18,19,20,21):',b'for version in (17,18,19,20,21,22):')
        new=new.replace(b"self.profile(tables)['rule_version']=22",b"self.profile(tables)['rule_version']=23")
    if name=='test_equipment_recycling.py':
        old_text="next(r for r in tables['game_equipment'] if r['item_id']==218)['required_level']=37"
        new_text="next(r for r in tables['game_equipment'] if r['item_id']==218).update(required_level=37,stage=3)"
        assert old_text.encode() in new;new=new.replace(old_text.encode(),new_text.encode())
    assert new!=old,name;path.write_bytes(new)
    changes.append({'path':path.relative_to(ROOT).as_posix(),'before_sha256':hashlib.sha256(old).hexdigest(),'after_sha256':hashlib.sha256(new).hexdigest(),'scope':'仅current版本；装备level37样本同步其唯一stage3；候选支持22并拒23'})
for directory,names,count in [('devconfig',['map_catalog_test.go','pet_runtime_test.go','summon_test.go'],1),('integration',['level_growth_test.go','summon_pet_test.go'],None)]:
    for name in names:
        path=ROOT/'server/internal'/directory/name;old=path.read_bytes();n=old.count(b'.RuleVersion != 21')
        assert n==(2 if name=='level_growth_test.go' else 1),(name,n)
        new=old.replace(b'.RuleVersion != 21',b'.RuleVersion != 22');path.write_bytes(new)
        changes.append({'path':path.relative_to(ROOT).as_posix(),'before_sha256':hashlib.sha256(old).hexdigest(),'after_sha256':hashlib.sha256(new).hexdigest(),'current_guard_replacements':n,'scope':'仅当前主fixture22门禁，不改历史合同或业务断言'})
(OUT/'current-test-adaptations.json').write_text(json.dumps(changes,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(changes,ensure_ascii=False))
