#!/usr/bin/env python3
"""从两本唯一Excel编译关闭的召唤候选；不把候选技能加入当前主profile。"""
from __future__ import annotations

import argparse
from copy import deepcopy
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'tools/config'))
sys.path.insert(0,str(ROOT/'tools/resources'))
import export_configs as config
import prepare_first_loop as loop
import prepare_pet_runtime as pets

DEST=ROOT/'config/development/summon-candidates.json'

def compile_catalog(tables,evidence):
    profile=deepcopy(pets.one(tables,'dev_first_loop_profile',1)['parameters'])
    if profile['rule_version'] not in (17,18,19,20,21):
        raise ValueError('关闭召唤候选须使用完整rule17至21主合同；候选最低合同仍为17')
    references=[(row['id'],row['summon_skill_id']) for row in tables['game_pet_templates'] if row['record_status']!='historical' and row['summon_skill_id'] is not None]
    ids=[skill for _,skill in references]
    if not ids or len(set(ids))!=len(ids) or any(type(id) is not int or id<=0 for id in ids):
        raise ValueError('独立召唤候选必须由PetDefs唯一显式引用技能，不猜默认17')
    if len(profile['skill_ids'])!=22 or any(id in profile['skill_ids'] for id in ids) or profile['casting_supply']['enabled'] or profile['pet_runtime']['bootstrap_mode']!='legacy_untrained_pet':
        raise ValueError('本阶段主profile须保持22技能范围、材料关闭与legacy不训练，候选不得暗启')
    profile['skill_ids']+=ids
    all_skills=loop.skill_definitions(tables,profile)
    loop.validate_skills(profile,all_skills)
    skills=[row for row in all_skills if row['id'] in ids]
    definitions=pets.compile_definitions(tables,[id for id,_ in references])
    for map_id in [profile['map_id']]+profile['map_catalog_ids']:
        loop.map_policy(tables,pets.one(tables,'game_maps',map_id),profile)
    by_id={row['id']:row for row in definitions}
    for definition_id,skill_id in references:
        skill=next(row for row in skills if row['id']==skill_id)
        if skill['behavior']!='summon_pet' or skill['summon']!={'definition_id':definition_id} or by_id[definition_id]['source_skill_id']!=skill_id or not by_id[definition_id]['recall_name']:
            raise ValueError('召唤技能、PetDefs、来源身份及召回名必须成对一致')
    books=[loop.skill_book_definition(pets.one(tables,'game_items',skill['book_item_id']),skill,profile,complete=True) for skill in skills]
    return {'schema_version':1,'development_only':True,'runtime_enabled':False,'required_rule_version':17,
        'skills':deepcopy(skills),'books':books,'upgrades':loop.skill_upgrade_definitions(tables,profile,skills),
        'pet_definition_ids':[id for id,_ in references],'source_evidence':deepcopy(evidence)}

def source_evidence(paths):
    return {'workbooks':[{'group':group,'path':path.relative_to(ROOT).as_posix(),'sha256':config.sha256(path)} for group,path in paths.items()],
        'schema_sha256':config.sha256(ROOT/'config/schema/catalog.json'),
        'compiler_sha256':config.sha256(Path(loop.__file__)),'pet_compiler_sha256':config.sha256(Path(pets.__file__)),
        'generator_sha256':config.sha256(Path(__file__)),
        'source_contract_sha256':config.sha256(ROOT/'资料索引/职业宠物补充来源契约.json'),'target_180_verified':False}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check',action='store_true')
    args=parser.parse_args()
    schema=config.load_json(ROOT/'config/schema/catalog.json')
    tables,errors,_,paths=config.collect(schema)
    errors+=config.check_json_contracts(schema,tables)+config.semantic_checks(tables)
    if errors:raise ValueError(errors)
    catalog=compile_catalog(tables,source_evidence(paths))
    if args.check:
        if json.loads(DEST.read_text(encoding='utf-8'))!=catalog:
            raise ValueError('召唤独立候选过期，须从唯一Excel重新编译')
    else:
        DEST.parent.mkdir(parents=True,exist_ok=True)
        DEST.write_text(json.dumps(catalog,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps({'check':args.check,'runtime_enabled':False,'skills':[row['id'] for row in catalog['skills']],'books':[row['id'] for row in catalog['books']]}))

if __name__=='__main__':main()
