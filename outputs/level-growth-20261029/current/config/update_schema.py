"""rule20只新增profile技术合同与经验正整数边界，保持原表字段顺序/类型。"""
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
path=ROOT/'config/schema/catalog.json';schema=json.loads(path.read_text(encoding='utf-8'))
contract=next(c for c in schema['json_contracts'] if c['table_id']=='dev_first_loop_profile')
assert 'progression' not in contract['shape']['properties']
contract['shape']['properties']['progression']={'type':'object','properties':{
    'curve_version':{'type':'string'},'operating_level_cap':{'type':'int','minimum':1,'maximum':2**32-1},
    'catalog_max_level':{'type':'int','minimum':1,'maximum':2**32-1},'pending_pool_policy':{'type':'string'},
    'new_overflow_policy':{'type':'string'},'legacy_overflow_policy':{'type':'string'},'level_up_resources':{'type':'string'},'rebirth_enabled':{'type':'boolean'}}}
contract['source_ref']+='；本任务rule20仅来源经验1–50候选及显式上限/待用池/旧溢出/同生命升级资源策略；上限/每转增量/不重置读取唯一game_progression_rules已定行；转生关闭，不认证自然成长周期'
table=next(t for t in schema['tables'] if t['table_id']=='game_exp_levels')
for field in table['fields']:
    if field['name'] in ['id','level','next_level_exp']:
        field['minimum']=1;field['maximum']=2**53-1 if field['name']=='next_level_exp' else 2**32-1
text=json.dumps(schema,ensure_ascii=False,indent=2)+'\n'
(WORK/'schema.candidate.json').write_text(text,encoding='utf-8',newline='\n')
path.write_text(text,encoding='utf-8',newline='\n')
