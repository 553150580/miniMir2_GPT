"""只读核乌木来源、唯一表和原图格式；只写本阶段.work研究报告，不编译新候选。"""
from datetime import datetime, timezone
import csv
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / 'tools/config'))
sys.path.insert(0, str(ROOT / 'tools/resources'))
import export_configs as config
import inspect_wil as wil


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def relative(path):
    return path.relative_to(ROOT).as_posix() if path.is_relative_to(ROOT) else str(path)


workbook = ROOT / 'outputs/development-prep-20261026/玩法配置.xlsx'
architecture = ROOT / 'outputs/development-prep-20261026/架构配置.xlsx'
csv_path = ROOT / '资料参考/myf/Mud2/DB/ApexM2_stditems.csv'
drop_path = ROOT / '资料参考/myf/Mir200/Envir/MonItems/稻草人.txt'
technical_sources = [
    'tools/resources/inspect_wil.py', 'tools/resources/build_development_assets.py', 'tools/resources/build_hud_assets.py',
    'tools/resources/prepare_first_loop.py', 'server/internal/devconfig/equipment_recycling.go',
    'server/internal/persistence/equipment_recycling.go', 'server/internal/persistence/assets.go',
    'client/assets/scripts/app/GameWorldView.ts', 'client/assets/scripts/app/WorldAssets.ts',
    '资料参考/crystal/Shared/Data/ItemData.cs', '资料参考/crystal/Server/MirObjects/HumanObject.cs',
    '资料参考/crystal/Client/MirObjects/PlayerObject.cs', '资料参考/crystal/Client/MirGraphics/MLibrary.cs',
    '资料参考/crystal/Client/MirObjects/ItemObject.cs', '资料参考/crystal/LibraryEditor/Graphics/WeMadeLibrary.cs']
frozen = [workbook, architecture, csv_path, drop_path, ROOT / 'config/schema/catalog.json',
          ROOT / 'config/development/first-loop.json', ROOT / 'client/assets/resources/development/gameplay.json',
          ROOT / 'client/assets/resources/development/manifest.json'] + [ROOT / name for name in technical_sources]
before = {relative(path): sha(path) for path in frozen}
schema = load(ROOT / 'config/schema/catalog.json')
tables, errors, _, _ = config.collect(schema)
assert not errors, errors
profile = tables['dev_first_loop_profile'][0]['parameters']
source_raw = csv_path.read_bytes()
csv_rows = list(csv.DictReader(source_raw.decode('utf-8-sig').splitlines()))
weapon = next(row for row in csv_rows if row['Idx'] == '220')
assert weapon['Name'] == '乌木剑' and weapon['Shape'] == '34' and weapon['Looks'] == '43'
assert [weapon[k] for k in ['Dc', 'Dc2', 'Mc', 'Mc2', 'Sc', 'Sc2', 'Need', 'NeedLevel', 'Price']] == ['4', '8', '0', '1', '0', '0', '0', '1', '3000']
drop_lines = drop_path.read_bytes().split(b'\n')
drop_line = drop_lines[12].rstrip(b'\r').decode('gb18030')
assert drop_line.split() == ['1/60', '乌木剑']
reference = load(ROOT / '资料索引/参考文件清单.json')
registry = {row['reference_id'].casefold(): row for row in reference['files']}
assert sha(csv_path) == registry['myf:mud2/db/apexm2_stditems.csv']['sha256']
primary = Path('D:/Mir2/mir2_gxx/assets/legacy/primary_180/Data')
primary_probes = []
for package in ['Items', 'stateitem', 'DnItems', 'Weapon']:
    a = next(p for p in primary.iterdir() if p.stem.casefold() == package.casefold() and p.suffix.casefold() == '.wil')
    b = next(p for p in primary.iterdir() if p.stem.casefold() == package.casefold() and p.suffix.casefold() == '.wix')
    initial = [sha(a), sha(b)]
    data = wil.probe(a, b)
    ids = [42, 43] if package != 'Weapon' else [1200, 1800, 40800, 40801, 41400]
    frames = [{**data['frames'][i], 'rgba_sha256': hashlib.sha256(wil.decode_frame(a, data['frames'][i], data['palette'])).hexdigest()}
              for i in ids]
    assert initial == [sha(a), sha(b)]
    primary_probes.append({'package': package, 'wil_path': str(a), 'wix_path': str(b), 'wil_sha256': initial[0], 'wix_sha256': initial[1],
        'registered_hash_match': initial[0] == registry[('client_180_named:Data/' + a.name).casefold()]['sha256'] and initial[1] == registry[('client_180_named:Data/' + b.name).casefold()]['sha256'],
        'format': data['format_id'], 'frame_count': data['frame_count'], 'native_frames': frames, 'source_before_after_equal': True,
        'item_identity_mapping_certified': False})
legacy_manifest_path = Path('D:/Mir2/mir2_gxx/assets/legacy/manifest.json')
legacy_manifest = load(legacy_manifest_path)
original_assets = []
for row in legacy_manifest['files']:
    if Path(row['relative_path']).stem.casefold() not in {'weapon', 'items', 'stateitem', 'dnitems'}:
        continue
    p = Path(row['path'])
    actual = sha(p) if p.is_file() else None
    original_assets.append({'path': str(p), 'priority': row['priority'], 'registered_sha256': row['sha256'],
        'current_sha256': actual, 'original_accessible': p.is_file(), 'matches_manifest': actual == row['sha256'],
        'original_manifest_visual_semantics': row.get('visual_semantics')})
alternate = Path('D:/Mir2/mir2_gxx/assets/legacy/fallback_mir19/Data')
alternate_weapon, alternate_index = alternate / 'Weapon.wil', alternate / 'Weapon.WIX'
try:
    wil.probe(alternate_weapon, alternate_index)
    raise AssertionError('本次已审旧格式拒绝边界发生变化')
except wil.UnsupportedFormat as error:
    alternate_error = str(error)
fixture = load(ROOT / 'config/development/first-loop.json')
public = load(ROOT / 'client/assets/resources/development/gameplay.json')
prior = [WORK.parent / 'level-growth-20261029' / ('next-' + name + '.json') for name in ['content-audit', 'weapon-semantics', 'equipment-pipeline']]
report = {
    'schema_version': 1, 'recorded_at_utc': datetime.now(timezone.utc).isoformat(),
    'head': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
    'status': 'read_only_source_audit_no_runtime_or_workbook_change',
    'scope': '下一阶段乌木220及装备成长前置；只确认原列/源像素/当前门禁，不是新装备候选配置或目标1.80认证',
    'production_or_excel_or_schema_changed': False, 'new_runtime_catalog_created': False,
    'source_program_or_deployment_or_account_database_or_external_network_used': False,
    'previous_audits': [{'path': relative(p), 'sha256': sha(p), 'status': 'historical_rule20_context_not_current_workbook'} for p in prior],
    'source_220': {'path': relative(csv_path), 'encoding': 'utf-8-sig', 'sha256': sha(csv_path), 'physical_line': 222,
        'line_raw_sha256': hashlib.sha256(source_raw.splitlines(keepends=True)[221]).hexdigest(), 'raw': weapon,
        'confirmed_columns': {'DC': [4, 8], 'MC': [0, 1], 'SC': [0, 0], 'Need': 0, 'NeedLevel': 1, 'Price': 3000, 'Shape': 34, 'Looks': 43,
            'DuraMax': 7000, 'Weight': 8, 'Stdmode': 5, 'Anicount': 0, 'Ac_Ac2_Mac_Mac2': [0, 0, 0, 0]},
        'clarifications': ['需求等级1对应Need=0/NeedLevel=1，不是原Need=1', 'Shape34/Looks43，不能互换',
            'DuraMax7000是原列事实；未据此认证本作完整持久损坏、修理或药修经济', 'Color255不转成本作品质，原行没有本作职业白名单或T1枚举']},
    'price_usage': {'raw_fact': 'Price列3000', 'apex_same_version_price_consumer_found': False,
        'current_project_evidence': ['tools/resources/prepare_first_loop.py:135、754、836：既有50/400显式profile借Price做开发金币事务，不是正式分解价',
            'game_recycle_values唯一行gold_distribution=null，正式配方未审；220没有开发回收值或NPC报价行'],
        'prohibited_inferences': ['不能把3000自动写入RecycleGold', '不能把未知配方写0以让物品被吞', '不能据Price/Stock直接开放装备商店、卖店或配额', 'Crystal独立ItemInfo.Price行为不能替代Apex同引擎消费合同']},
    'weapon_field_semantics': {'same_apex_consumer_or_manual_found': False, 'status': 'unknown_preserved',
        'sources': ['.work/level-growth-20261029/next-weapon-semantics.json', '资料参考/crystal/Shared/Data/ItemData.cs:99-118、641-675',
            '资料参考/crystal/Server/MirObjects/HumanObject.cs:1831-1847', '资料参考/crystal/Client/MirObjects/PlayerObject.cs:552-586'],
        'conclusions': ['Stdmode5/6差异不能从Crystal ItemType反推；尤其不能据5限制战士或据6限制法师。',
            '武器Ac/Ac2/Mac/Mac2重载与Anicount含义仍未补同引擎证据；乌木这些列恰为0，八荒27和木剑Ac2=3不能静默归零或当防御/攻速。',
            'Crystal直接Info.Image→Items索引及Shape→CWeapons独立库可作其自身技术参考，不能证明MYF Looks/Shape到当前WIL编号。']},
    'current_unique_inputs': {'rule_version': profile['rule_version'], 'workbook_sha256': sha(workbook), 'architecture_sha256': sha(architecture),
        'schema_sha256': sha(ROOT / 'config/schema/catalog.json'), 'counts': {key: len(tables[key]) for key in ['game_items', 'game_equipment', 'game_appearance', 'game_recycle_values']},
        'selected_equipment': tables['game_equipment'], 'item220_present': any(row['id'] == 220 for row in tables['game_items']),
        'equipment220_present': any(row['item_id'] == 220 for row in tables['game_equipment']), 'development_recycle_gold': profile['recycle_test_gold_by_item'],
        'formal_recycle_values': tables['game_recycle_values'], 'profile_equipment_drops': fixture['drops']['equipment_item_ids'],
        'main_skills': len(fixture['skills']), 'summon17_enabled': any(row['id'] == 17 for row in fixture['skills']),
        'casting_supply_enabled': fixture['casting_supply']['enabled'], 'bootstrap_mode': fixture['pet_runtime']['bootstrap_mode']},
    'current_admission_blockers': [
        {'path': 'tools/resources/prepare_first_loop.py', 'lines': [737, 748, 754, 655],
         'detail': '主物品循环固定35/36/193/218；既有属性只投影DC/AC/MAC与attack_speed，没有MC/SC四上下限；每装备强索引profile金币并要求明确整数。'},
        {'path': 'server/internal/devconfig/equipment_recycling.go', 'lines': [25, 33, 48],
         'detail': 'missing/null与明确0已分开，但ValidateEquipmentRecycling现在强制每个装备EquipmentRecycleKnown；所以正常准入无配方装备尚缺合同，不可直接绕过Validate做假运行。'},
        {'path': 'server/internal/persistence/equipment_recycling.go', 'lines': [69, 73, 84, 95],
         'detail': '运行回收保护已有recipe_unknown返回；要让新可穿装备合法无配方，应显式拆开装备准入与回收资格，保留原件并跳过，不填0或3000。'},
        {'path': 'client/assets/scripts/app/GameWorldView.ts', 'lines': [73, 333, 376],
         'detail': '当前Weapon层仅weaponEquipped布尔显示；尚无按itemID/职业固定外观挑选的完整资源身份。'}],
    'drop_source': {'path': relative(drop_path), 'encoding': 'gb18030', 'sha256': sha(drop_path), 'physical_lf_line': 13,
        'raw_line': drop_line, 'source_legacy_probability': '1/60', 'adopted_runtime_probability': False,
        'current_policy': '原类别65/20/10/5、装备保底20及已选子集由唯一Excel维护；新增220入等权子集会改变每件供给及可回收金币结构，须明确选择并验收。',
        'communication_erratum': '此前向server协作消息误记1/200，已立即更正为本次原文1/60；不采用聊天记忆作数据源。'},
    'native_assets': {'primary_probes': primary_probes, 'legacy_manifest': {'path': str(legacy_manifest_path), 'sha256': sha(legacy_manifest_path)},
        'accessible_original_files': original_assets, 'samples_index': relative(WORK / 'source-samples/index.json'),
        'sample_pixel_transforms': [], 'source_asset_bytes_unchanged': True,
        'visual_observation': {'method': '本agent通过view_image实际查看6张原生解码PNG，未缩放/裁切/改色',
            'Items42_State42': '弯头杖样；不能为了Looks43改取42', 'Items43_State43': '木色直剑样，可支持明确跨来源图标候选，尚无同版本MYF映射消费者证明',
            'Weapon40800_41400': '饰柄金属武器样，未据外观认证为乌木；两个索引只为排查Shape34×1200/+600假说，绝非正式映射',
            'item_name_to_pixels_certified': False},
        'fallback_weapon': {'path': str(alternate_weapon), 'sha256': sha(alternate_weapon), 'index_sha256': sha(alternate_index),
            'signature': '0x19 WEMADE Entertainment inc.', 'decoder_support': False, 'reason': alternate_error,
            'scope': '该备用mir19不是primary_180；现inspect_wil只支持#ILIB v1内嵌256色未压缩，不能硬改header套读。'},
        'wzx_scope': '所白名单核的primary_180及可访问原目录四个命名包为WIL/WIX；fallback镜像也只有Weapon WIL/WIX，gxx data只有ui_android.psk。未在这些命名包看到WZX/WZL；不声称整个外部来源树不存在。',
        'official_180_item_mapping_verified': False},
    'existing_resource_contract': {'source': 'tools/resources/build_development_assets.py:22-29、313-321；现development/manifest.json',
        'profiles': {'player_male': {'Hum': 1200, 'Weapon': 1200, 'Hair': 1200}, 'player_female': {'Hum': 1800, 'Weapon': 1800, 'Hair': 1800}},
        'adaptation_status': '既有固定开发profile，不是木剑/乌木/衣服逐件源映射',
        'direction_and_offsets': '八向动作按profile元数据；原PNG保留WIL offsets，direction层顺序；新外观须覆盖各动作/八向、遮挡与锚点，不能只认证一帧。',
        'icon_status': 'game_items.icon_resource_key现装备为空；build_hud_assets.py:176-192已明确Looks不可直接等同该1.80 Items，药水3/4仅通用入口候选。',
        'fixed_job_identity': '最终策划案.md:115：男战士/女法师/男道士，同itemID不拆性别物品或性别穿戴限制。'},
    'complete_usable_prerequisites': [
        '唯一game_items与game_equipment明确新增选择/职业/需求/品质/阶段；DC4–8/MC0–1/SC0–0逐列来源可用，未使用属性显式适配，不由缺省补0。',
        '四MC/SC bonus从唯一装备定义经fixture/严格Go类型/fighter累加/战斗实际作用，并公开同源基础属性供单件详情；有溢出与原身体/Life/unknown保护。',
        '正常来源入口必须明确（掉落子集或有审定报价的通用商店）；现book-only报价/UI不能冒充装备商店。不能增初始赠220绕过自然获得。',
        '装备替换及真正卸装均走原资产事务/完整背包归属/满包失败原件保留/原op重放；当前真正卸装入口仍缺。',
        '先解决无回收配方装备合法准入并recipe_unknown保留；不把正式空表或Price3000当0/3000回收定义。',
        '图标可在明确candidate选择后引用原Items/StateItem帧；角色body须有可靠库/动作映射或明确共同开发外观适配，不冒称乌木原服外观。',
        '正常可获得→详情→穿上实际DC/MC变化/真实伤害→卸下恢复→健康票保持；SC成长需要另一个真实SC装备，不能因乌木MC就声称三职业MC/SC全完成。'],
    'decisions_or_unknowns': [
        'Root需确定本轮采用哪些明确基础属性/职业白名单和供给入口，保留非已解释Apex字段。',
        '尚无Apex同版本Stdmode/Shape/Looks消费源码或可信字段手册；官方1.80全映射认证仍unknown。',
        '是否接受Items/StateItem43作为明确跨来源候选，以及body继续共同开发profile还是先补真实外观；不能靠Shape乘法默认。',
        '没有乌木回收配方；正式洗炼石与祝福油、损坏、修理、词条、10部位强化和经济平衡不属于这条来源事实的自动授权。'],
    'non_claims': ['本报告不是配置候选编译、业务测试、Creator或正常GUI验收', '未运行来源程序、部署指令、账号库、Go/DB/Creator或外网',
        '没有新增生产配置/Excel/schema/PNG资源，仅.work原生解码样例和研究报告'],
    'inputs_before': before}
after = {relative(path): sha(path) for path in frozen}
assert before == after
report['inputs_after'] = after
report['production_and_reference_inputs_unchanged'] = True
report['git_status_after'] = subprocess.check_output(['git', 'status', '--short'], cwd=ROOT, text=True)
target = WORK / 'source-audit.json'
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'report': relative(target), 'sha256': sha(target), 'raw_source': 'Shape34/Looks43/Need0/NeedLevel1/Price3000',
    'runtime220_enabled': False, 'mapping': 'unknown_candidate_pixels_only', 'inputs_unchanged': True}, ensure_ascii=False))
