"""Root审阅后使用：先完成全部只读门禁，再按白名单归档；--check不写正式证据。"""
from __future__ import annotations

import argparse
from collections import Counter
from copy import deepcopy
from datetime import datetime, timezone
import gzip
import hashlib
import importlib.util
import json
import math
from pathlib import Path
import re
import runpy
import shutil
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
OUT=ROOT/'outputs/auto-recycle-20261029'
FORMAL=ROOT/'docs/自动回收与挂机战报阶段验收.json'
ENGINE=Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos').resolve()
ENGINE_FILES=['input/input.ts','2d/event/pointer-event-dispatcher.ts','scene-graph/node-event-processor.ts',
              'scene-graph/node.ts','ui/button.ts','core/data/object.ts','scene-graph/component.ts','audio/audio-source.ts','ui/toggle.ts']
FONT_FILES=[Path('C:/Windows/Fonts/arial.ttf'),Path('C:/Windows/Fonts/msyh.ttc'),Path('C:/Windows/Fonts/simsun.ttc')]
FORBIDDEN={'client-project','library','temp','build','node_modules','__pycache__','.git','inputs'}
CHECK_COMMANDS={'models','full-ui-app','scene','strict','protocol-check','protocol-lint'}


def need(ok,message):
    if not ok:raise ValueError(message)


def sha(path):
    value=hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda:stream.read(1024*1024),b''):value.update(block)
    return value.hexdigest()


def read(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def local(value,base=ROOT):
    p=Path(value)
    if not p.is_absolute():p=base/p
    p=p.resolve()
    need(p.is_relative_to(base.resolve()),'路径越过受控目录：'+str(p))
    need(not p.is_symlink() and not getattr(p,'is_junction',lambda:False)(),'证据不得是链接：'+str(p))
    return p


def current(values,label):
    need(isinstance(values,dict) and values,label+'缺摘要集合')
    for name,digest in values.items():
        p=local(name)
        need(p.is_file() and sha(p)==digest,label+'当前字节不同：'+name)


def client_snapshot():
    paths=[p for folder in ['client/assets/scripts','client/tests'] for p in (ROOT/folder).rglob('*') if p.is_file()]
    paths += [p for pattern in ['*.scene','*.prefab'] for p in (ROOT/'client/assets').rglob(pattern)]
    paths += [ROOT/p for p in ['client/assets/resources/development/gameplay.json','client/assets/resources/development/manifest.json','config/development/first-loop.json']]
    return {local(p).relative_to(ROOT).as_posix():sha(p) for p in sorted(set(paths))}


def node_counts(path):
    text=Path(path).read_text(encoding='utf-8')
    result={key:int(value) for key,value in re.findall(r'^(?:ℹ|#)\s+(tests|pass|fail|skipped|cancelled|todo)\s+(\d+)\s*$',text,re.M)}
    need(set(result)=={'tests','pass','fail','skipped','cancelled','todo'},'缺少Node完整终态：'+str(path))
    need(result['tests']==result['pass']>0 and all(result[k]==0 for k in ['fail','skipped','cancelled','todo']),'Node并非全通过：'+str(path))
    return result


def json_output(path):
    text=Path(path).read_text(encoding='utf-8')
    try:return json.loads(text)
    except json.JSONDecodeError:
        for line in reversed(text.splitlines()):
            try:return json.loads(line)
            except json.JSONDecodeError:pass
    raise ValueError('缺实际JSON命令结果：'+str(path))


def engine_current(values):
    for name,digest in values.items():
        p=Path(name).resolve()
        need(p.is_relative_to(ENGINE) and p.is_file() and not p.is_symlink() and sha(p)==digest,'独立引擎当前变化：'+name)


def font_current(values):
    need(set(map(lambda s:str(Path(s).resolve()),values))=={str(p.resolve()) for p in FONT_FILES},'字体不是精确3文件白名单')
    for name,digest in values.items():
        p=Path(name).resolve()
        need(p.is_file() and not p.is_symlink() and sha(p)==digest,'独立字体当前变化：'+name)


def font_evidence(client,path):
    before,after,recorded=[client[k] for k in ['external_font_before','external_font_after','external_font_current']]
    need(before==after==recorded,'字体3源前后不等');font_current(recorded)
    review_path=WORK/'client/confirmation-review.json';review=read(review_path)
    need(review['validation_3_sha256']==sha(path) and review['font_files_hashed_only']==recorded and review['font_binaries_copied'] is False,'字体测量与本次client冻结不配对')
    metric_path=local(review['metrics_file']);need(metric_path.is_relative_to(WORK/'client') and sha(metric_path)==review['metrics_sha256'],'字体测量JSON摘要不等')
    r=read(metric_path);label=r['label'];samples=r['samples'];metrics=r['metrics']
    need(len(samples)==21 and all(isinstance(s,str) and s for s in samples),'缺21项真实确认文案样本')
    lines={line for sample in samples for line in sample.split('\n')}
    pairs={(row['font'],row['line']) for row in metrics}
    need(len(pairs)==len(metrics) and pairs=={(font,line) for font in ['Arial','Microsoft YaHei','SimSun'] for line in lines},'字体度量未覆盖每个实际样本行/3字体')
    need(all(type(row['width']) in (int,float) and math.isfinite(row['width']) and row['width']>=0 for row in metrics),'字体宽度非有限非负数')
    maximum=max(row['width'] for row in metrics);max_lines=max(len(sample.split('\n')) for sample in samples)
    need(r['maxWidth']==review['max_width_px']==maximum and r['maxLines']==review['max_lines']==max_lines,'字体宽/行数统计不等于原明细')
    scene=read(ROOT/'client/assets/scenes/Game.scene');nodes={i:r for i,r in enumerate(scene) if r.get('__type__')=='cc.Node'}
    def node_path(i):
        p=nodes[i].get('_parent');return (node_path(p['__id__'])+'/' if p and p['__id__'] in nodes else '')+nodes[i]['_name']
    message=next(row for i,row in nodes.items() if node_path(i)=='Canvas/Safe/PopupView/Panels/Confirm/Message')
    components=[scene[c['__id__']] for c in message['_components']]
    transform=next(c for c in components if c['__type__']=='cc.UITransform');text=next(c for c in components if c['__type__']=='cc.Label')
    need(label=={'width':transform['_contentSize']['width'],'height':transform['_contentSize']['height'],'font':text['_fontFamily'],'fontSize':text['_fontSize'],'lineHeight':text['_lineHeight']},'测量不是保存Confirm真实Label尺寸/字体')
    need(maximum<label['width'] and max_lines*label['lineHeight']<=label['height'] and label['lineHeight']==review['line_height_px'],'确认文本超出保存Label边界')
    return {'font_inputs':len(recorded),'samples':len(samples),'max_width_px':maximum,'max_lines':max_lines,'height_px':max_lines*label['lineHeight'],
            'label':label,'scope':r['scope'],'review':review_path,'metrics':metric_path,'sample_texts':samples}


def go_evidence(report_path,events_path):
    report=read(report_path)
    need(report['passed'] is True and report['mongo_e2e_explicit'] is True and report['race_detector'] is True and report['exit_code']==0,'最终Go未真实Mongo/race通过')
    need('./...' in report['command'] and '-race' in report['command'] and '-json' in report['command'],'Go不是完整全包race命令')
    need(report['source_changed_during_test'] is False and report['source_sha256']==report['source_sha256_after'],'Go输入前后不等')
    current(report['source_sha256'],'Go')
    definition=runpy.run_path(str(ROOT/'tools/server/test.py'),run_name='read_only_input_enumeration')
    need({p.relative_to(ROOT).as_posix() for p in definition['input_files']()}==set(report['source_sha256']),'Go不是完整当前输入集合')
    need(sha(events_path)==report['events_sha256'],'Go raw events摘要不等')
    events=[json.loads(line) for line in events_path.read_text(encoding='utf-8').splitlines() if line.strip()]
    passed=[{'package':e['Package'],'test':e['Test']} for e in events if e.get('Action')=='pass' and e.get('Test')]
    skipped=[{'package':e['Package'],'test':e['Test']} for e in events if e.get('Action')=='skip' and e.get('Test')]
    failed=[{'package':e['Package'],'test':e.get('Test')} for e in events if e.get('Action') in {'fail','build-fail'}]
    need(passed==report['passed_tests'] and skipped==report['skipped_tests'] and failed==report['failed_tests'],'Go报告与raw事件不一致')
    need(passed and not skipped and not failed,'Go存在失败或测试跳过')
    need(report['fixture']['sha256']==sha(ROOT/'config/development/first-loop.json'),'Go不是当前fixture')
    need(report['finished_at_utc'] and report['started_at_utc'],'Go缺实际起止时间')
    return report,{'passed':len(passed),'failed':len(failed),'skipped':len(skipped),'inputs':len(report['source_sha256']),
                   'raw_package_actions':dict(Counter(e['Action'] for e in events if not e.get('Test')))}


def client_evidence(path):
    need(path.is_relative_to(WORK/'client') and path.name=='report.json','client必须显式选本阶段报告')
    report=read(path)
    need(report['passed'] is True and report['stable'] is True and report['freeze_started_before_commands'] is True,'client未冻结通过')
    need(type(report['input_count']) is int and report['input_count']==len(report['before'])==len(report['after'])==len(report['current'])>0,'client声明数量不等于集合')
    need(report['before']==report['after']==report['current']==client_snapshot(),'client前后/current/完整重扫不同')
    current(report['current'],'client')
    before,after,recorded=[report[key] for key in ['external_engine_before','external_engine_after','external_engine_current']]
    need(before==after==recorded and set(map(lambda s:str(Path(s).resolve()),recorded))=={str((ENGINE/p).resolve()) for p in ENGINE_FILES},'引擎9源不是精确独立白名单')
    engine_current(recorded)
    need(len(report['commands'])==len(CHECK_COMMANDS) and {c['name'] for c in report['commands']}==CHECK_COMMANDS,'client检查组缺失或重复')
    logs={}
    for c in report['commands']:
        p=local(c['log']);need(c['exit_code']==0 and sha(p)==c['sha256'],'client命令或日志不通过：'+c['name']);logs[c['name']]=p
    app,scene=node_counts(logs['full-ui-app']),node_counts(logs['scene'])
    # task协议检查外层记录执行命令；实际codec JSON在该记录的output文件，不能把包装器当codec计数。
    tasks=[json.loads(line) for line in logs['protocol-check'].read_text(encoding='utf-8').splitlines() if line.strip()]
    codec=[task for task in tasks if 'tools/protocol/verify_client_pb.cjs' in task['command']]
    need(len(codec)==1 and all(task['exit_code']==0 for task in tasks),'协议检查task未完整通过')
    codec_path=local(codec[0]['output']);pb=json_output(codec_path);protocol=json_output(logs['protocol-lint'])
    need(pb['status']=='passed' and protocol['valid'] is True,'PB边界/协议清单未通过')
    fonts=font_evidence(report,path)
    return report,app,scene,pb,protocol,len(recorded),[local(task['output']) for task in tasks],fonts


def creator_evidence(path):
    r=read(path)
    need(r['creator']=='3.8.8' and r['platform']=='web-mobile' and r['exit_code']==36 and r['success'] is True,'Creator未真实退出36成功')
    need(r['source_stable_during_build'] is True and r['scene_prefab_layout_preserved'] is True,'Creator构建时输入/布局不稳定')
    need(type(r['seconds']) in (int,float) and r['seconds']>0,'Creator缺真实耗时')
    current(r['asset_hashes'],'Creator保存布局');current(r['script_hashes'],'Creator脚本')
    need(local(r['log']).is_file(),'Creator缺原日志')
    preserved=path.parent/'creator.log'
    need(preserved.is_file() and sha(preserved)==sha(local(r['log'])),'Creator专属保存日志与实际构建日志不配对')
    return r


def layout_evidence():
    before=read(WORK/'client/before.json')
    layouts={p:v for p,v in before.items() if Path(p).suffix in {'.scene','.prefab'}}
    need(len(layouts)==10,'原保存布局集合不完整')
    game='client/assets/scenes/Game.scene';old_path=WORK/'client/Game.before.scene'
    need(sha(old_path)==layouts[game],'旧Game保存源摘要不符')
    old,new=read(old_path),read(ROOT/game)
    changed=[i for i,row in enumerate(old) if row!=new[i]]
    need(len(old)==1053 and len(new)-len(old)==137 and changed==[257,425,435,436],'Game不是已审1053原对象/4处变化/137新增')
    diff=read(WORK/'client/scene-diff.json')
    need(diff['old_object_count']==len(old) and diff['new_objects']==len(new)-len(old) and diff['changed_existing_indices']==changed and diff['sha256']==sha(ROOT/game),'保存场景diff与当前不等')
    for i in changed:
        need(diff['changes'][str(i)]=={'before':old[i],'after':new[i]},'场景逐对象diff不同')
        keys=[k for k in set(old[i])|set(new[i]) if old[i].get(k)!=new[i].get(k)]
        need(keys=={257:['_children'],425:['_children'],435:['_lpos'],436:['_contentSize']}[i],'旧对象修改超出已审字段')
        if i in [257,425]:need(new[i]['_children'][:len(old[i]['_children'])]==old[i]['_children'] and all(x['__id__']>=1053 for x in new[i]['_children'][len(old[i]['_children']):]),'原子引用不是纯追加')
    actual={p.relative_to(ROOT).as_posix() for pattern in ['*.scene','*.prefab'] for p in (ROOT/'client/assets').rglob(pattern)}
    need(actual==set(layouts),'保存布局文件集合变化')
    for p,digest in layouts.items():
        if p!=game:need(sha(ROOT/p)==digest,'其它9布局变化：'+p)
    return {'old_objects':len(old),'new_objects':len(new)-len(old),'changed_existing_indices':changed,'other_saved_layouts_unchanged':len(layouts)-1}


def formula_map(path):
    ns={'s':'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
    with zipfile.ZipFile(path) as book:
        return {name:{c.attrib['r']:ET.tostring(c.find('s:f',ns),encoding='unicode') for c in ET.fromstring(book.read(name)).findall('.//s:c',ns) if c.find('s:f',ns) is not None}
                for name in book.namelist() if name.startswith('xl/worksheets/') and name.endswith('.xml')}


def configuration_evidence():
    sys.path.insert(0,str(ROOT/'tools/config'));import export_configs as config
    review=read(WORK/'config/final-review.json');current(review['current_hashes'],'配置冻结')
    old=WORK/'config/before/outputs/development-prep-20261026/玩法配置.xlsx';new=ROOT/'outputs/development-prep-20261026/玩法配置.xlsx'
    a,b=config.read_xlsx(old),config.read_xlsx(new);differences=[]
    for sheet in set(a)|set(b):
        for row in set(a.get(sheet,{}))|set(b.get(sheet,{})):
            left,right=a.get(sheet,{}).get(row,{}),b.get(sheet,{}).get(row,{})
            for column in set(left)|set(right):
                if left.get(column)!=right.get(column):differences.append((sheet,row,column,left.get(column),right.get(column)))
    need(len(differences)==1 and differences[0][:3]==('开发首循环夹具',4,3),'源书不是唯一profile一格变化')
    previous,profile=map(json.loads,differences[0][3:]);normalized=deepcopy(profile)
    need(previous['rule_version']==18 and normalized.pop('equipment_recycling')=={'mode':'development_gold_only','outputs':[]} and normalized['rule_version']==19,'源profile版本/合同错误')
    normalized['rule_version']=18;need(normalized==previous,'原profile其它值变化')
    f1,f2=formula_map(old),formula_map(new);need(f1==f2 and sum(map(len,f1.values()))==460,'460公式不等')
    with zipfile.ZipFile(old) as left,zipfile.ZipFile(new) as right:
        need(left.namelist()==right.namelist(),'工作簿ZIP成员变化')
        touched=[n for n in left.namelist() if left.read(n)!=right.read(n)]
        need(len(touched)==1 and len(left.namelist())-len(touched)==102,'非目标ZIP成员变化')
    baseline=read(WORK/'config/baseline.json')
    arch='outputs/development-prep-20261026/架构配置.xlsx';need(sha(ROOT/arch)==baseline['files'][arch],'架构书变化')
    raw=(WORK/'config/python-full.stdout.txt').read_text(encoding='utf-8')+(WORK/'config/python-full.stderr.txt').read_text(encoding='utf-8')
    matched=re.search(r'^Ran (\d+) tests? in ',raw,re.M)
    need(read(WORK/'config/python-full.json')['exit_code']==0 and matched and int(matched[1])==111 and re.search(r'^OK\s*$',raw,re.M) and not re.search(r'^(?:FAILED|ERROR|FAIL:)',raw,re.M),'Python111未实际通过')
    for name in ['export-check','pet-check','summon-check','fixture-check']:
        need(read(WORK/'config'/f'{name}.json')['exit_code']==0,'配置check失败：'+name)
    # 完整内容重算；不执行写文件入口或外部程序。
    schema=config.load_json(ROOT/'config/schema/catalog.json');tables,errors,_,_=config.collect(schema)
    need(not errors and not config.semantic_checks(tables) and not config.check_json_contracts(schema,tables),'唯一Excel当前结构/合同错误')
    spec=importlib.util.spec_from_file_location('readonly_first_loop',ROOT/'tools/resources/prepare_first_loop.py');module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    fixture=read(ROOT/'config/development/first-loop.json');business=module.fixture_from_rows(tables)
    expected=deepcopy(fixture);evidence=expected.pop('_source_evidence');business.pop('_source_evidence')
    need(expected==business,'当前fixture不是唯一Excel业务投影')
    need(module.public_gameplay(fixture)==read(ROOT/'client/assets/resources/development/gameplay.json'),'public不是当前fixture同源投影')
    need(evidence['exporter_sha256']==sha(ROOT/'tools/resources/prepare_first_loop.py'),'fixture编译器摘要变化')
    need(fixture['equipment_recycling']=={'mode':'development_gold_only','outputs':[]},'回收不是显式开发GoldOnly')
    equipment=[r for r in fixture['items'] if r['is_equipment']]
    need({r['id']:r['recycle_gold'] for r in equipment}=={218:previous['recycle_test_gold_by_item']['218'],193:previous['recycle_test_gold_by_item']['193']},'原金币候选变化')
    for r in equipment:
        definition=next(x for x in tables['game_equipment'] if x['item_id']==r['id'])
        need(r['required_level']==definition['required_level']>=1 and r['drop_quality']==definition['quality']==next(x['quality'] for x in tables['game_items'] if x['id']==r['id']),'等级/品质不是唯一装备来源')
    return fixture,profile,review,{'changed_cells':1,'formulas':460,'untouched_zip_members':102,'architecture_unchanged':True,'python_passed':int(matched[1])}


def asset_evidence():
    baseline=read(WORK/'config/baseline.json')['assets'];game='client/assets/scenes/Game.scene'
    for name,digest in baseline.items():
        if name!=game:need(sha(local(name))==digest,'旧素材/地图/meta变化：'+name)
    return {'old_protected_files':len(baseline),'png_preserved':sum(Path(p).suffix.lower()=='.png' for p in baseline),
            'wav_preserved':sum(Path(p).suffix.lower()=='.wav' for p in baseline),
            'source_town_files_preserved':sum('/source-maps/' in p for p in baseline),
            'scope':'逐旧文件原字节核对；Game.scene的4处修改和新增脚本meta单列，不称全部布局不变'}


def policy_match(item,policy,definitions):
    definition=definitions.get(item['itemId'])
    return bool(definition and definition['is_equipment'] and item['quantity']=='1' and item['quality']==definition['drop_quality']
                and not item['locked'] and item['flags']==0 and item['quality'] in policy['qualityIds'] and item['itemId'] not in policy['protectedItemIds']
                and not (policy['protectBound'] if item['bound'] else policy['protectUnbound'])
                and (policy['maxRequiredLevel']==0 or definition['required_level']<=policy['maxRequiredLevel']))


def gui_evidence(path,build,fixture,fonts):
    need(path.is_relative_to(WORK),'GUI必须是本阶段显式报告')
    r=read(path)
    need(r['passed'] is True and r['errors']==[] and r['checks'],'GUI未终态通过')
    need(r['touchInput'] is True and r['privateBusinessInvoked'] is False and r['routeOverride'] is False and r['bodySeeded'] is False,'GUI不是正常触屏无注入合同')
    need(all(w.get('type')!='error' for w in r['warnings']),'GUI含console error，不以warnings名义放行')
    frozen={**build['asset_hashes'],**build['script_hashes']}
    need(r['sourceBefore']==r['sourceAfter']==frozen,'GUI被测源与当前实构不等')
    current(frozen,'GUI源码')
    tool=ROOT/'tools/client/verify_auto_recycle_browser.cjs'
    need(r['toolSha256']==r['toolSha256After']==sha(tool),'GUI执行工具字节变化')
    need(r['fixtureSha256']==sha(ROOT/'config/development/first-loop.json') and r['publicSha256']==sha(ROOT/'client/assets/resources/development/gameplay.json'),'GUI不是当前fixture/public')
    need(len(r['health'])==3 and {h['port'] for h in r['health']}=={18100,18101,18102} and len({h['instance'] for h in r['health']})==3,'GUI没有精确主3实例健康证据')
    need(all(h['status']=='ok' and h['development_only'] is True and h['rule_version']==19 and h['fixture_hash']==r['fixtureSha256'] for h in r['health']),'GUI健康不是当前rule19 fixture')
    o=r['observations'];initial,locked,future,applied,reentry=[o[k] for k in ['initial','locked','futureOnly','applied','reentry']]
    need(initial['rule']=='19' and initial['policy']['enabled'] is False and initial['policy']['revision']=='0','正常新角默认状态错误')
    need(initial['role'] and all(x['role']==initial['role'] for x in [locked,future,applied,reentry]),'GUI角色身份改变')
    need(initial['binding']==locked['binding']==future['binding']==applied['binding'] and reentry['binding']!=applied['binding'],'健康新票未区分')
    selected=locked['selected'];need(re.fullmatch(r'[0-9a-f]{32}',selected or '') is not None,'缺原锁定实例')
    original=next(i for i in initial['items'] if i['id']==selected);protected=next(i for i in locked['items'] if i['id']==selected)
    need(protected['locked'] is True and protected['flags']&1 and int(protected['version'])==int(original['version'])+1,'锁定不是原实例+版本更新')
    need(future['policy']['enabled'] is True and future['policy']['revision']=='1','未来策略未正式提交')
    need(future['items']==locked['items'] and future['wallet']==locked['wallet'],'仅保存策略改变既有资产')
    definitions={r['id']:r for r in fixture['items']};eligible=[i for i in future['items'] if policy_match(i,future['policy'],definitions)]
    need(eligible,'GUI没有真正处理任何原件')
    expected={k:sum(definitions[i['itemId']]['recycle_gold'] for i in eligible if i['bound']==(k=='boundGold')) for k in ['gold','boundGold']}
    need(applied['policy']['revision']=='2' and applied['policy']['enabled'] is True,'明确应用没有原策略修订')
    need(applied['items']==[i for i in future['items'] if i not in eligible],'回收后原件/保护保留不等')
    need(applied['walletRevision']==applied['revision'],'GUI没有等到匹配钱包版本')
    for key,amount in expected.items():need(int(applied['wallet'][key])-int(future['wallet'][key])==amount,'金币分币不等')
    receipts=[q for q in r['receipts'] if q['trigger']==2 and q['role_revision']==applied['revision']]
    need(len(receipts)==1,'缺唯一原提交回执')
    receipt=receipts[0]
    need(receipt['binding']==applied['binding'] and receipt['source_seq']=='0' and receipt['policy_revision']==applied['policy']['revision'] and re.fullmatch(r'[0-9a-f]{32}',receipt['op_id']) is not None,'回执原身份不匹配')
    need(int(receipt['recycled_count'])==len(eligible) and int(receipt['gold'])==expected['gold'] and int(receipt['bound_gold'])==expected['boundGold'],'原回执数量/分币不等')
    need(reentry['policy']==applied['policy'] and reentry['items']==applied['items'] and reentry['wallet']==applied['wallet'],'健康新票丢失或重复资产')
    message=o['applyConfirmation'];need('预计回收'+str(len(eligible))+'件' in message and '金币 '+str(expected['gold']) in message and '绑定金币 '+str(expected['boundGold']) in message,'没有真实确认收益文字')
    need(message in fonts['sample_texts'],'实际GUI文案不是已测字体样本')
    need(len(o['visibleMessage']['entries'])==1,'没有一次已提交可见消息')
    names=['01-locked-equipment.png','02-future-only.png','03-before-apply-confirm.png','03-applied.png','03-visible-result-message.png','04-healthy-reentry.png','05-taller-settings.png']
    screenshots=[path.parent/name for name in names]
    need(all(p.is_file() and p.read_bytes().startswith(b'\x89PNG\r\n\x1a\n') for p in screenshots),'GUI缺真实保存UI截图')
    visual=read(path.parent/'visual-review.json')
    need(visual['visual_passed'] is True and visual['gui_report_sha256']==sha(path) and visual['image_sha256']==sha(path.parent/'03-before-apply-confirm.png') and visual['confirmed_text']==message,'最终GUI缺当前报告/截图配对的完整文字人工复核，run02功能通过不能替代')
    return r,screenshots,{'recycled_count':len(eligible),'gold':expected['gold'],'bound_gold':expected['boundGold'],'screenshots':len(screenshots),'healthy_binding_changed':True}


class Archive:
    """只追加明确文件；大文本无损gzip一份，不递归工程/缓存或整套inputs。"""
    def __init__(self):self.jobs={}

    def add(self,source,target,scope,core=False):
        source=local(source);need(source.is_file(),'归档源缺失：'+str(source))
        need(source.suffix.lower() not in {'.exe','.dll','.lib','.wil','.wix'},'拒绝原始大包或可执行二进制')
        relative=source.relative_to(ROOT).as_posix();destination=Path(target)
        for existing in self.jobs.values():
            if existing['source']==relative:
                need(existing['raw_sha256']==sha(source),'同一归档源在计划期间变化')
                return
        need(not destination.is_absolute() and '..' not in destination.parts and not set(destination.parts)&FORBIDDEN,'归档目标越界/命中缓存')
        compress=not core and source.suffix.lower() in {'.json','.jsonl','.txt','.log','.stdout','.stderr','.scene','.prefab','.py','.cjs','.proto'} and source.stat().st_size>=128*1024
        if source.suffix=='.jsonl':compress=True
        if compress:destination=destination.with_name(destination.name+'.gz')
        name=destination.as_posix()
        need(name not in self.jobs or self.jobs[name]['source']==relative,'归档目标冲突')
        self.jobs[name]={'source':relative,'target':name,'scope':scope,'encoding':'gzip' if compress else 'identity','raw_sha256':sha(source),'raw_bytes':source.stat().st_size}

    def shallow(self,directory,target,scope,names):
        for name in names:
            p=directory/name
            if p.is_file():self.add(p,Path(target)/name,scope,core=name=='report.json')

    def apply(self):
        OUT.mkdir(parents=True,exist_ok=False)
        records=[]
        for record in self.jobs.values():
            p=ROOT/record['source'];need(sha(p)==record['raw_sha256'],'归档前源变化')
            target=OUT/record['target'];target.parent.mkdir(parents=True,exist_ok=True)
            if record['encoding']=='gzip':
                with p.open('rb') as stream,target.open('xb') as output,gzip.GzipFile(filename='',mode='wb',fileobj=output,mtime=0) as compressed:shutil.copyfileobj(stream,compressed)
                digest=hashlib.sha256()
                with gzip.open(target,'rb') as stream:
                    for block in iter(lambda:stream.read(1024*1024),b''):digest.update(block)
                need(digest.hexdigest()==record['raw_sha256'],'gzip解压原字节不等')
            else:shutil.copyfile(p,target);need(sha(target)==record['raw_sha256'],'归档副本不同')
            records.append({**record,'file_sha256':sha(target),'file_bytes':target.stat().st_size})
        return records


def curate(args,client,build,gui,shots,protocol_outputs,fonts):
    archive=Archive()
    for p,target in [(args.go_report,'current/go/report.json'),(args.go_events,'current/go/events.jsonl'),(args.client_report,'current/client/report.json'),(args.creator_report,'current/creator/report.json'),(args.creator_report.parent/'creator.log','current/creator/creator.log'),(args.gui_report,'current/gui/report.json')]:archive.add(p,target,'current',core=target.endswith('report.json'))
    for c in client['commands']:archive.add(local(c['log']),'current/client/'+Path(c['log']).name,'current')
    for p in protocol_outputs:archive.add(p,'current/client/'+p.name,'current_protocol_codec_artifact')
    for p in shots:archive.add(p,'current/gui/'+p.name,'current')
    archive.add(args.gui_report.parent/'visual-review.json','current/gui/visual-review.json','current_visual_review',core=True)
    archive.add(fonts['review'],'current/client/confirmation-review.json','current_font_metrics',core=True)
    archive.add(fonts['metrics'],'current/client/'+fonts['metrics'].name,'current_font_metrics',core=True)
    archive.shallow(args.go_report.parent,'current/go','current',['wrapper.json','tool.stdout.txt','tool.stderr.txt'])
    for folder,names in [('config',['application.json','baseline.json','candidate-review.json','candidate-editor-review.json','edit-plan.json','final-review.json','python-full.json','python-full.stdout.txt','python-full.stderr.txt','export-check.json','export-check.stdout.txt','fixture-check.json','fixture-check.stdout.txt','pet-check.json','pet-check.stdout.txt','summon-check.json','summon-check.stdout.txt']),('client',['scene-diff.json','before.json','Game.before.scene']),('config-peer',['review.json']),('server-peer',['review.json'])]:archive.shallow(WORK/folder,Path('current')/folder,'current',names)
    archive.add(WORK/'config/before/outputs/development-prep-20261026/玩法配置.xlsx','before/玩法配置.xlsx','before_unique_profile_cell')
    history=[]
    for p in sorted((WORK/'server').glob('mongo-*/report.json')):
        r=read(p)
        if r['passed'] is False:
            history.append({'report':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'scope':'historical_mongo_attempt_not_current','passed':False,'failures':r['failed_tests']})
            archive.shallow(p.parent,Path('history/server')/p.parent.name,'historical_failure',['report.json','events.jsonl','stderr.txt','source-before.json','source-after.json','mongodb-status.stdout','mongodb-status.stderr'])
    need(len(history)>=2,'缺本阶段两次Mongo失败原始历史，不能以最终通过覆盖')
    for p in sorted((WORK/'server').glob('full-*/report.json')):
        r=read(p)
        if r['passed'] is False:
            events=p.parent/'events.jsonl';need(sha(events)==r['events_sha256'],'历史全Go原events摘要不等')
            wrapper=read(p.parent/'wrapper.json')
            for name,digest in wrapper['files_sha256'].items():need(sha(p.parent/name)==digest,'历史全Go wrapper原字节变化')
            history.append({'report':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'scope':'historical_full_go_fixture_adaptation_failure_not_current','passed':False,
                            'passed_tests_at_that_time':len(r['passed_tests']),'failures':r['failed_tests'],'inputs_at_that_time':len(r['source_sha256'])})
            archive.shallow(p.parent,Path('history/server')/p.parent.name,'historical_full_go_failure',['report.json','events.jsonl','wrapper.json','tool.stdout.txt','tool.stderr.txt'])
    for p in sorted((WORK/'client').glob('validation-*/report.json')):
        if p.resolve()!=args.client_report:
            r=read(p);history.append({'report':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'scope':'historical_client_validation_not_current','passed_at_that_time':r['passed']})
            archive.add(p,Path('history/client')/p.parent.name/'report.json','historical_client',core=True)
            for c in r['commands']:archive.add(local(c['log']),Path('history/client')/p.parent.name/Path(c['log']).name,'historical_client')
    for p in sorted((WORK/'browser').glob('run-*/report.json')):
        if p.resolve()!=args.gui_report:
            r=read(p);vpath=p.parent/'visual-review.json';visual=read(vpath) if vpath.exists() else None
            history.append({'report':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'scope':'historical_gui_not_current',
                            'functional_passed_at_that_time':r['passed'],'visual_review':visual,
                            'note':'原功能passed和人工视觉判断分别保留；run03只记录等待前置不足，不能追认当时按钮disabled原因'})
            archive.add(p,Path('history/browser')/p.parent.name/'report.json','historical_gui',core=True)
            if vpath.exists():archive.add(vpath,Path('history/browser')/p.parent.name/vpath.name,'historical_visual_review',core=True)
            for image in sorted(p.parent.glob('*.png')):archive.add(image,Path('history/browser')/p.parent.name/image.name,'historical_gui_screenshot')
    # 旧正常Town/UI来自确认文案修复前，只归历史，不计入最终TS当前回归。
    for p in sorted((WORK/'normal-browsers').glob('run-*/report.json')):
        archive.shallow(p.parent,Path('history/normal-browsers')/p.parent.name,'historical_before_confirmation_text_fix',
                        ['report.json','selector.json','inputs-before.json','inputs-after.json','before.json','after.json'])
        history.append({'report':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'scope':'historical_normal_browsers_before_confirmation_text_fix','passed_at_that_time':read(p)['passed']})
        for log in sorted(p.parent.glob('*.txt')):archive.add(log,Path('history/normal-browsers')/p.parent.name/log.name,'historical_normal_log')
        for child in ['browser-town','browser-ui']:
            folder=p.parent/child
            if (folder/'report.json').exists():
                archive.add(folder/'report.json',Path('history/normal-browsers')/p.parent.name/child/'report.json','historical_normal_browser',core=True)
                for image in sorted(folder.glob('*.png')):archive.add(image,Path('history/normal-browsers')/p.parent.name/child/image.name,'historical_normal_screenshot')
    for p in args.history_report:
        need(p.is_relative_to(WORK) and p!=args.gui_report,'历史报告必须显式选本阶段其它路径')
        r=read(p);history.append({'report':p.relative_to(ROOT).as_posix(),'sha256':sha(p),'scope':'explicit_historical_report','passed_at_that_time':r.get('passed')})
        archive.add(p,Path('history/extra')/p.parent.name/p.name,'historical',core=True)
    for p in [ROOT/'shared/proto/assets.proto',ROOT/'shared/proto/protocol_manifest.json',ROOT/'tools/client/verify_auto_recycle_browser.cjs',Path(__file__)]:archive.add(p,Path('sources')/p.relative_to(ROOT),'source_at_archive')
    return archive,history


def write_new(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('x',encoding='utf-8',newline='\n') as stream:stream.write(json.dumps(value,ensure_ascii=False,indent=2)+'\n')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for flag in ['go-report','go-events','client-report','creator-report','gui-report']:parser.add_argument('--'+flag,type=Path,required=True)
    parser.add_argument('--history-report',type=Path,action='append',default=[],help='其它原始失败/历史报告，显式白名单追加，不修改原status')
    parser.add_argument('--check',action='store_true',help='只读完成全部preflight，不建outputs、不写正式docs')
    args=parser.parse_args()
    for name in ['go_report','go_events','client_report','creator_report','gui_report']:setattr(args,name,local(getattr(args,name)))
    args.history_report=[local(p) for p in args.history_report]
    need(not OUT.exists() and not FORMAL.exists(),'正式产物已存在，拒绝覆盖')
    fixture,profile,configuration,cc=configuration_evidence()
    need(fixture['rule_version']==19 and len(fixture['skills'])==22 and all(s['id']!=17 for s in fixture['skills']),'主范围不是rule19/22技能/17关闭')
    need(fixture['casting_supply']['enabled'] is False and fixture['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet','材料/legacy门禁改变')
    summon,pets=[read(ROOT/'config/development'/name) for name in ['summon-candidates.json','pet-runtime-candidates.json']]
    need(summon['runtime_enabled'] is False and summon['required_rule_version']==17 and pets['runtime_enabled'] is False,'独立候选暗启')
    go,gc=go_evidence(args.go_report,args.go_events)
    client,app,scene,pb,protocol,engine_count,protocol_outputs,fonts=client_evidence(args.client_report)
    build=creator_evidence(args.creator_report);layout=layout_evidence();assets=asset_evidence()
    gui,shots,observed=gui_evidence(args.gui_report,build,fixture,fonts)
    archive,history=curate(args,client,build,gui,shots,protocol_outputs,fonts)
    preview=read(ROOT/'config/releases/preview_current.json');config=read(ROOT/'config/reports/配置校验报告.json')
    need(preview['version']==configuration['preview'] and config['structure_valid'] is True and config['runtime_ready'] is False and config['release_blockers'],'预览/正式门禁状态不符')
    report={'schema_version':1,'date':'2026-10-29','recorded_at_utc':datetime.now(timezone.utc).isoformat(),'stage':'开发自动回收、原件保护与逐事件挂机战报','status':'limited_passed',
            'scope':'主rule19开发GoldOnly：永久免费策略/锁定/明确应用背包/普通战斗逐事件回收及托管统计；木剑与布衣沿原显式开发金币候选，不发布正式分解经济',
            'runtime':{'rule_version':fixture['rule_version'],'preview':preview['version'],'fixture_sha256':sha(ROOT/'config/development/first-loop.json'),'skills':len(fixture['skills']),
                       'summon17_enabled':False,'casting_supply_enabled':False,'bootstrap_mode':fixture['pet_runtime']['bootstrap_mode'],'legacy_pet_trains':False,
                       'summon_candidate_runtime_enabled':summon['runtime_enabled'],'summon_candidate_required_rule_version':summon['required_rule_version'],
                       'equipment_recycling':fixture['equipment_recycling'],'equipment_candidate_gold':{str(r['id']):r['recycle_gold'] for r in fixture['items'] if r['is_equipment']},
                       'table_count':config['table_count'],'record_count':config['record_count'],'release_blockers':len(config['release_blockers']),'runtime_ready':False,'production_ready':False},
            'verification':{'go_full_mongo_ws_race_passed_including_subtests':gc['passed'],'go_skipped':gc['skipped'],'go_failed':gc['failed'],'go_inputs':gc['inputs'],'go_source_stable':True,
                            'configuration_python_passed':cc['python_passed'],'javascript_ui_app_passed':app['pass'],'javascript_scene_passed':scene['pass'],'javascript_total_passed':app['pass']+scene['pass'],
                            'client_inputs':client['input_count'],'client_external_engine_inputs':engine_count,'pb_messages':pb['message_count'],'pb_long_fields':pb['int64_field_count'],
                            'client_external_font_inputs':fonts['font_inputs'],'confirmation_text_samples':fonts['samples'],'confirmation_max_width_px':fonts['max_width_px'],'confirmation_max_lines':fonts['max_lines'],'confirmation_height_px':fonts['height_px'],
                            'pb_boundary_cases':pb['int64_boundary_cases'],'pb_protocol_ids':protocol['messages'],'creator_seconds':build['seconds'],'creator_exit_code':build['exit_code'],
                            'gameplay_excel_changed_cells':cc['changed_cells'],'formulas_preserved':cc['formulas'],'untouched_zip_members':cc['untouched_zip_members'],'architecture_workbook_preserved':True},
            'main_creator':{'passed':build['success'],'seconds':build['seconds'],'exit_code':build['exit_code'],'source_stable':build['source_stable_during_build'],'layout_preserved_during_build':build['scene_prefab_layout_preserved']},
            'saved_ui_change':layout,'artifact_preservation':assets,
            'gui':{'passed':gui['passed'],'checks':gui['checks'],'errors':gui['errors'],'warnings':gui['warnings'],'touchInput':gui['touchInput'],'privateBusinessInvoked':gui['privateBusinessInvoked'],
                   'routeOverride':gui['routeOverride'],'bodySeeded':gui['bodySeeded'],'observed':observed,'scope':gui['scope']},
            'raw_go_counts':gc,'historical_attempts':history,'selectors':{k:{'path':getattr(args,k).relative_to(ROOT).as_posix(),'sha256':sha(getattr(args,k))} for k in ['go_report','go_events','client_report','creator_report','gui_report']},
            'limits':['50/400仅原Price开发候选，正式game_recycle_values未发布；石/油副产物显式outputs=[]，不是将未知正式行补零',
                      '本轮未接当前/上限持久，损坏回收及修理未验收','主22技能/legacy不训练/材料false/17closed保持；护符包Count/售价仍待定',
                      '响应丢弃后原ID重试不等于真实UnknownTransaction网络故障注入','真实主预览触屏观察不代表手机真机、容量、完整P0或全1.80'],
            'physical_mobile_verified':False,'capacity_verified':False,'production_ready':False}
    current(go['source_sha256'],'写入前Go');need(client_snapshot()==client['current'],'写入前client变化');engine_current(client['external_engine_current']);font_current(client['external_font_current'])
    for r in archive.jobs.values():need(sha(ROOT/r['source'])==r['raw_sha256'],'写入前归档源变化')
    summary={'preflight':'passed','archive_executed':False,'formal_report_written':False,'status_when_applied':report['status'],'go':gc['passed'],'go_inputs':gc['inputs'],
             'javascript':app['pass']+scene['pass'],'client_inputs':client['input_count'],'engine_inputs':engine_count,'font_inputs':fonts['font_inputs'],'python':cc['python_passed'],'gui_checks':len(gui['checks']),'archive_jobs':len(archive.jobs)}
    if args.check:print(json.dumps(summary,ensure_ascii=False));return
    records=archive.apply()
    with (OUT/'README.md').open('x',encoding='utf-8',newline='\n') as stream:
        stream.write('# 自动回收与挂机战报限定证据\n\ncurrent只收终态通过且当前输入相等的全包Mongo/WS race、分组client/引擎、Creator及正常新角色UI证据；history保留当时原status、失败日志和源码摘要，不能追溯改成通过。\n\n大文本使用标准gzip无损存储。evidence-manifest.json逐文件记录原始raw_sha256/raw_bytes与实际归档file_sha256/file_bytes，解压后须复核raw摘要；原报告内部路径和SHA不重写。未递归复制工程、library、temp、build、node_modules、nbserver.exe或整套inputs。\n\n50/400只沿既有开发金币候选；outputs=[]不是将未知正式洗炼石与祝福油规则补零。持久损坏回收、正式经济、容量、手机、完整P0及全1.80未验收。主22技能、legacy不训练、护符false、17closed保持。\n')
    write_new(OUT/'evidence-manifest.json',{'schema_version':1,'scope':'显式白名单；大文本gzip无损，raw/file SHA分别保存，原报告内部路径与status不改写','files':records})
    write_new(OUT/'evidence-paths.json',{r['source']:{'path':(OUT/r['target']).relative_to(ROOT).as_posix(),'encoding':r['encoding'],'raw_sha256':r['raw_sha256'],'file_sha256':r['file_sha256']} for r in records})
    report['evidence_sha256']={(OUT/r['target']).relative_to(ROOT).as_posix():r['file_sha256'] for r in records}
    for name in ['evidence-manifest.json','evidence-paths.json','README.md']:report['evidence_sha256'][(OUT/name).relative_to(ROOT).as_posix()]=sha(OUT/name)
    report['evidence_file_count']=len(report['evidence_sha256'])
    current(go['source_sha256'],'写入后Go');need(client_snapshot()==client['current'],'写入后client变化');engine_current(client['external_engine_current']);font_current(client['external_font_current'])
    write_new(FORMAL,report)
    print(json.dumps({**summary,'archive_executed':True,'formal_report_written':True,'formal_report':str(FORMAL),'evidence_files':report['evidence_file_count']},ensure_ascii=False))


if __name__=='__main__':main()
