"""只在最终limited_passed原报告成立后生成滚动说明计划；默认不改正式文档。"""
from pathlib import Path
import argparse
import hashlib
import json
import re

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
FORMAL=ROOT/'docs/基础装备属性与正常穿脱阶段验收.json'
def sha(raw):return hashlib.sha256(raw).hexdigest()

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--apply',action='store_true');args=parser.parse_args()
    stage=json.loads(FORMAL.read_text(encoding='utf-8'));v,runtime,gui=stage['verification'],stage['runtime'],stage['gui']
    assert stage['status']=='limited_passed' and runtime['rule_version']==22 and stage['full_p0_completed'] is False
    assert gui['passed'] and not gui['errors'] and v['go_failed']==v['go_skipped']==0
    assert v['javascript_total_passed']==v['javascript_ui_app_passed']+v['javascript_scene_passed']
    assert v['saved_layouts_total']==v['saved_layouts_unchanged_against_head']==v['layouts_preserved_during_build']==10 and v['saved_layouts_intentionally_changed']==0
    assert runtime['equipment_item_ids']==[218,193] and runtime['source220_enabled'] is False and runtime['summon17_enabled'] is False and runtime['casting_supply_enabled'] is False
    assert v['gameplay_excel_changed_cells']==6 and v['formulas_preserved']==460 and v['untouched_zip_members']==101
    summary=(f"基础装备属性与正常穿脱已通过rule22限定开发验收：{v['go_full_mongo_ws_race_passed_including_subtests']}项真实Mongo/WS全包Go race、"
        f"{v['configuration_python_passed']}项配置Python、{v['javascript_total_passed']}项JS（{v['javascript_ui_app_passed']} UI/app、{v['javascript_scene_passed']}场景；"
        f"{v['client_inputs']}份项目输入、另{v['client_external_engine_inputs']}份锁定引擎源码和{v['client_external_font_inputs']}份字体分别冻结）。"
        f"Creator3.8.8候选构建及后续SHA核对{v['creator_seconds']}秒、exit36，原主包在构建期间受保护；真实产物逐SHA相等后受控切主。"
        "10份保存布局及旧资源不变。木剑218/布衣193的完整11项基础属性与阶段来自唯一表，四个MC/SC端点原值均为0；非零两职业测试采用明确隔离资格，只验证原施法/属性管线。"
        "40001明确穿入/卸下及原版本，满包等量替换与真正卸下分别校验，迁移原件并保存typed回执；旧合法穿戴物可卸/返包，不因当前新穿职业/等级/品质门槛被困或洗白。"
        "Role与背包选择隔离，单件基础属性和权威总属性分开展示；武器层显隐只观察当前自身附着，不重建Life或恢复HP/MP。"
        f"正常触屏{len(gui['checks'])}项通过、{len(gui['errors'])}错误/{len(gui['warnings'])}警告。"
        f"当前预览`{runtime['preview']}`；唯一玩法书6格受控变更、460公式及架构书保持。"
        "主仍22技能、legacy免费未训宠、护符和17关闭；220未选入，具体body/Shape/Looks映射、完整三职业装备成长、正式洗炼石与祝福油、持久修理、真机、容量与完整P0仍待完成。")
    targets={'README.md':'docs/基础装备属性与正常穿脱阶段验收.json','client/README.md':'../docs/基础装备属性与正常穿脱阶段验收.json',
        'config/README.md':'../docs/基础装备属性与正常穿脱阶段验收.json','docs/开发阶段与验收清单.md':'基础装备属性与正常穿脱阶段验收.json',
        'docs/正式开发推进记录.md':'基础装备属性与正常穿脱阶段验收.json','docs/架构适配与规则优先级.md':'基础装备属性与正常穿脱阶段验收.json'}
    edits={}
    for name,link in targets.items():
        before=(ROOT/name).read_bytes();text=before.decode('utf-8');heading,remainder=text.split('\n',1)
        assert '基础装备属性与正常穿脱已通过rule22' not in remainder[:220]
        after=heading+'\n\n'+summary+f'见[阶段验收]({link})。\n\n以下记录保留各自历史范围，当前以rule22与最新限定验收为准。\n\n'+remainder.lstrip('\r\n')
        if name=='docs/架构适配与规则优先级.md':
            rows=re.findall(r'^\| A45 \|.*$',after,re.M);assert len(rows)==1 and '限定阶段验收待完成' in rows[0]
            after=after.replace(rows[0],rows[0].replace('限定阶段验收待完成','《基础装备属性与正常穿脱阶段验收.json》；仅既有218/193基础链路限定通过，完整装备成长继续待验'))
        if name=='docs/开发阶段与验收清单.md':
            rows=re.findall(r'^\| T031 \|.*$',after,re.M);assert len(rows)==1 and '部分实现' in rows[0]
            replacement='| T031 | 部分实现，仅基础装备子范围 | rule22贯通既有218/193完整11项基础属性与阶段、原MC/SC管线及明确穿脱/回执/权威观察；非零两职业用隔离资格验管线 | 完整装备目录与自然供给、品质/词条/10部位强化、固定职业外观、持久/修理和可感知成长仍待完成；220未启用，不把原MC/SC零值或隔离测试称完整装备成长 | 策划§4.2、§5.10、7.4；基础装备属性与正常穿脱阶段验收.json |'
            after=after.replace(rows[0],replacement)
        edits[name]=(before,after.encode('utf-8'))
    name='docs/装备基础与明确穿脱实施边界.md';before=(ROOT/name).read_bytes();text=before.decode('utf-8')
    assert text.count('本阶段正在实施，尚未验收。')==1
    text=text.replace('本阶段正在实施，尚未验收。','本阶段已通过限定开发验收，见《基础装备属性与正常穿脱阶段验收.json》；完整装备成长仍在推进。')
    edits[name]=(before,text.encode('utf-8'))
    plan={'formal_sha256':sha(FORMAL.read_bytes()),'changes':{name:{'before_sha256':sha(before),'after_sha256':sha(after)} for name,(before,after) in edits.items()},'applied':args.apply}
    if args.apply:
        for name,(before,_) in edits.items():
            assert (ROOT/name).read_bytes()==before
            backup=WORK/'docs-before'/name;backup.parent.mkdir(parents=True,exist_ok=True)
            with backup.open('xb') as stream:stream.write(before)
        for name,(_,after) in edits.items():(ROOT/name).write_bytes(after)
    (WORK/('docs-applied.json' if args.apply else 'docs-plan.json')).write_text(json.dumps(plan,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps({'applied':args.apply,'documents':len(edits),'scope':'仅新限定范围；T031仍部分实现，历史记录保持'},ensure_ascii=False))

if __name__=='__main__':main()
