"""构造rule17受控候选和逐格/公式/ZIP证据；本脚本不替换唯一源表。"""
from copy import deepcopy
from pathlib import Path
import json
import shutil
import sys
import zipfile
import xml.etree.ElementTree as ET

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'tools/config'));sys.path.insert(0,str(ROOT/'tools/resources'))
import export_configs as config
import edit_workbook as editor
import prepare_first_loop as loop
import prepare_pet_runtime as pets
import prepare_summon_candidates as summons

SOURCE='本任务rule17关闭的开发候选；MYF ApexM2_Magic.csv:18技能17、ApexM2_stditems.csv:27书25；Crystal MirEnvir/Envir.cs:997–1012 MP12+4/range0/learn19；HumanObject.cs:3424–3451,4399–4438；MagicInfo.cs:18默认CD1800；不是MYF16困魔咒，不认证原服部署'
NAME_SOURCE='资料索引/职业宠物补充来源契约.json及crystal-supplement/source-slices/Settings.pet-defaults.json:技术行162 SkeletonName=BoneFamiliar；与MYF变异骷髅是明确跨来源候选，不是客户端display_name'

def save(name,data):
    (BASE/name).write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

def formulas(path):
    result={}
    with zipfile.ZipFile(path) as archive:
        for name in archive.namelist():
            if name.startswith('xl/worksheets/') and name.endswith('.xml'):
                for cell in ET.fromstring(archive.read(name)).iter('{'+editor.MAIN+'}c'):
                    formula=cell.find('{'+editor.MAIN+'}f')
                    if formula is not None:result[(name,cell.attrib['r'])]=ET.tostring(formula,encoding='unicode')
    return result

def main():
    schema=config.load_json(ROOT/'config/schema/catalog.json')
    tables,errors,_,paths=config.collect(schema);assert not errors,errors
    before={group:config.sha256(path) for group,path in paths.items()}
    assert tables['dev_first_loop_profile'][0]['parameters']['rule_version']==16
    assert not any(row['id']==17 for row in tables['game_skills'])
    assert not any(row['id']==25 for row in tables['game_items'])
    old_fixture=loop.fixture_from_rows(tables)
    after=deepcopy(tables)
    skill=deepcopy(next(row for row in after['game_skills'] if row['id']==31))
    skill.update(id=17,skill_key='source_summon_skeleton',display_name='召唤骷髅',class_id=2,learn_level=19,
        behavior_id='summon_pet',book_item_id=25,damage_or_heal_path='summon_pet',effect_ids=[],
        action_resource_key='player_male.casting',reference_evidence_id='development_source_summon_skeleton_v1',
        source_ref=SOURCE,note='仅独立候选；同名存活有居民节点先Recall且不耗符，新生才材料1；主22技能不选17。MYF19/23/26与Crystal19/22/26不同，三阶另由171–173候选维护，主升级未选入。')
    skill['target_rule'].update(auto_enabled=False,auto_priority=0,auto_min_targets=0,summon={'definition_id':1})
    skill['resource_costs']={'mp_base_cost':12,'mp_level_cost':4,'items':[],'material_item_id':187,'material_quantity':1}
    skill['damage_parameters']={key:(False if key=='add_caster_level' else [] if key in ['accuracy_by_level','max_attack_bonus_by_level','ready_proc_numerators'] else 0) for key in loop.SKILL_DAMAGE_FIELDS}
    after['game_skills'].append(skill)
    book=deepcopy(next(row for row in after['game_items'] if row['id']==13))
    book.update(id=25,item_key='skill_book_17',display_name='《召唤骷髅》',description='关闭的召唤候选书；不增加初始赠书或商店，正常学习与升级入口待后续验收',
        source_ref='MYF Mud2/DB/ApexM2_stditems.csv:27（Idx25/Stdmode4/原名召唤骷髅）；本任务候选，未新增价格行',
        note='书25只由召唤独立catalog投影，不进入主22技能对应书表；未开17/购符/赠书/升级；护符187数量与售价继续未知。')
    after['game_items'].append(book)
    pet=after['game_pet_templates'][0]
    assert pet['id']==1 and pet['summon_skill_id'] is None
    pet['summon_skill_id']=17
    pet['follow_and_return']['recall_name']='BoneFamiliar'
    pet['source_ref']+='；'+NAME_SOURCE
    pet['note']='rule17主仍原免费legacy_untrained_pet且不训练；summon_skill_id17仅正式候选引用，不加入运行skill_ids。RecallName大小写敏感同名，不由DefinitionID/显示名推断；现有Training.owner_range_cells唯一16兼作源自动Recall范围，未复制第二个16；出生2000/尸体180000不变。'
    operations=[{'kind':'append','table_id':'game_skills','id':17,'values':skill},
        {'kind':'append','table_id':'game_items','id':25,'values':book},
        {'kind':'update','table_id':'game_pet_templates','id':1,'values':{key:pet[key] for key in ['summon_skill_id','follow_and_return','source_ref','note']}}]
    for rank,required_level in enumerate([19,22,26],1):
        identifier=170+rank
        assert not any(row['id']==identifier or row['skill_id']==17 for row in tables['game_skill_upgrades'])
        upgrade=deepcopy(next(row for row in tables['game_skill_upgrades'] if row['id']==210+rank))
        upgrade.update(id=identifier,skill_id=17,required_role_level=required_level,book_cost=[{'item_id':25,'quantity':rank+1}],
            source_ref=SOURCE+'；本任务已选Crystal19/22/26作开发候选，明确区别MYF19/23/26；书目标阶+1为既定公式，页数沿已审5/10/20候选',
            note='仅关闭召唤catalog逐阶合同，保证完整Cfg.Validate；不进主22技能升级，不开商店/赠书/随机练功。')
        after['game_skill_upgrades'].append(upgrade)
        operations.append({'kind':'append','table_id':'game_skill_upgrades','id':identifier,'values':upgrade})
    for row in after['game_map_regions']:
        if row['id'] not in [201,202]:continue
        row['entry_rules']['no_pets']=False
        row['source_ref']+='；本任务rule17显式NoPets=false候选，保持当前可携宠地图开发范围；Crystal HumanObject.cs:4401、MonsterObject.cs:1334–1367；不从MYF NOMANNOMON或NoTeleport推定原服NoPets'
        row['note']+=' rule17独立no_pets布尔；不加入安全区或ZoneSetHash，受理与Recall到期时点按各自源规则。'
        operations.append({'kind':'update','table_id':'game_map_regions','id':row['id'],'values':{key:row[key] for key in ['entry_rules','source_ref','note']}})
    profile=after['dev_first_loop_profile'][0]
    profile['parameters']['rule_version']=17
    profile['parameters']['technical_parameters_notice']+='；rule17显式NoPets/RecallName与关闭的召唤独立候选，主仍22技能、legacy不训练、材料关闭；不认证17/购符业务。'
    profile['source_ref']+='；rule17地图宠物政策与独立召唤候选，同源参数不激活主技能'
    profile['note']+=' rule17主profile不追加技能17/书25/商店/升级/初始赠书；护符187未知值保持。独立catalog含完整三阶候选，不绕过校验。'
    operations.append({'kind':'update','table_id':'dev_first_loop_profile','id':1,'values':{key:profile[key] for key in ['parameters','source_ref','note']}})
    staged_schema=deepcopy(schema)
    for contract in staged_schema['json_contracts']:
        if contract['table_id']=='game_pet_templates' and contract['id']==1 and contract['field']=='follow_and_return':
            contract['shape']['properties']['recall_name']={'type':'string'}
            contract['source_ref']+='；'+NAME_SOURCE
        if contract['table_id']=='game_map_regions' and contract['id'] in [201,202] and contract['field']=='entry_rules':
            contract['shape']['properties']['no_pets']={'type':'boolean'}
            contract['source_ref']+='；rule17独立显式NoPets候选'
    cloned=[deepcopy(row) for row in staged_schema['json_contracts'] if row['table_id']=='game_skills' and row['id']==31]
    for contract in cloned:
        contract['id']=17;contract['source_ref']=SOURCE
        if contract['field']=='target_rule':contract['shape']['properties']['summon']={'type':'object','properties':{'definition_id':{'type':'int','minimum':1,'maximum':2**32-1}}}
        staged_schema['json_contracts'].append(contract)
    for rank in [1,2,3]:
        for original_contract in schema['json_contracts']:
            if original_contract['table_id']=='game_skill_upgrades' and original_contract['id']==210+rank:
                contract=deepcopy(original_contract);contract['id']=170+rank;contract['source_ref']=SOURCE
                staged_schema['json_contracts'].append(contract)
    save('schema-candidate.json',staged_schema)
    save('gameplay-plan.json',{'workbook':next(row['path'] for row in schema['workbooks'] if row['group']=='gameplay'),
        'group':'gameplay','input_workbook_sha256':before['gameplay'],'operations':operations})
    candidate=BASE/'gameplay-candidate.xlsx'
    review=editor.apply(BASE/'gameplay-plan.json',candidate)
    shutil.copyfile(paths['architecture'],BASE/'architecture-candidate.xlsx')
    original=config.read_xlsx(paths['gameplay']);actual=config.read_xlsx(candidate)
    by_table={row['table_id']:row for row in schema['tables']}
    allowed=set()
    for op in operations:
        table=by_table[op['table_id']];sheet=table['sheet_name']
        number=next(n for n,cells in actual[sheet].items() if n>=4 and cells.get(1)==op['id'])
        allowed.update((sheet,number,next(i+1 for i,field in enumerate(table['fields']) if field['name']==key)) for key in op['values'])
    differences=[]
    for sheet in set(original)|set(actual):
        for number in set(original.get(sheet,{}))|set(actual.get(sheet,{})):
            old=original.get(sheet,{}).get(number,{});new=actual.get(sheet,{}).get(number,{})
            for col in set(old)|set(new):
                if old.get(col)!=new.get(col):
                    assert (sheet,number,col) in allowed,(sheet,number,col)
                    differences.append({'sheet':sheet,'row':number,'column':col,'before':old.get(col),'after':new.get(col)})
    assert formulas(paths['gameplay'])==formulas(candidate)
    with zipfile.ZipFile(paths['gameplay']) as old,zipfile.ZipFile(candidate) as new:
        assert set(old.namelist())==set(new.namelist())
        changed_members=[name for name in old.namelist() if old.read(name)!=new.read(name)]
        assert all(name.startswith('xl/worksheets/') for name in changed_members)
        review.update(unchanged_zip_members=len(old.namelist())-len(changed_members),changed_zip_members=changed_members)
    save('gameplay-cell-diff.json',differences)
    review.update(actual_changed_cells=len(differences),all_unrequested_cell_values_preserved=True,all_formula_nodes_preserved=len(formulas(candidate)))
    save('gameplay-editor-review.json',review)
    collect_schema=deepcopy(staged_schema)
    for book_row in collect_schema['workbooks']:
        book_row['path']=(BASE/(book_row['group']+'-candidate.xlsx')).relative_to(ROOT).as_posix()
    staged,errors,_,_=config.collect(collect_schema)
    errors+=config.check_json_contracts(staged_schema,staged)+config.semantic_checks(staged)
    assert not errors,errors
    fixture=loop.fixture_from_rows(staged)
    catalog=summons.compile_catalog(staged,{'stage':'review_candidate_only'})
    save('summon-candidates.json',catalog);save('first-loop-candidate.json',fixture)
    for key in ['skills','items','skill_shop','skill_upgrades','initial_skill_books','casting_supply','pet']:
        assert fixture[key]==old_fixture[key],key+'主范围意外改变'
    for key in ['safe_zones','safe_zone_set_hash']:
        if key in old_fixture['maps'][0]:assert fixture['maps'][0][key]==old_fixture['maps'][0][key]
    assert all(config.sha256(path)==before[group] for group,path in paths.items())
    assert config.load_json(ROOT/'config/schema/catalog.json')==schema
    summary={'candidate_only':True,'source_books_unchanged':True,'source_schema_unchanged':True,'book_sha256_before':before,
        'changed_cells':{'gameplay':len(differences),'architecture':0},'gameplay_editor_review':review,
        'candidate_rule_version':17,'main_skills':len(fixture['skills']),'main_material_enabled':False,
        'legacy_unchanged':True,'old_5_zones_unchanged':True,'candidate_skill_ids':[row['id'] for row in catalog['skills']],
        'candidate_book_ids':[row['id'] for row in catalog['books']],'catalog_runtime_enabled':False}
    save('candidate-review.json',summary)
    print(json.dumps(summary,ensure_ascii=False))

if __name__=='__main__':main()
