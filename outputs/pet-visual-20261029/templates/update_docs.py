"""根据正式limited_passed验收准备/更新rule18文档；默认只校验，--apply才写入。"""
from __future__ import annotations
import argparse
import datetime
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STAGE = 'docs/宠物出生表现与隔离客户端阶段验收.json'
BOUNDARY = 'docs/宠物出生表现与隔离客户端实施边界.md'
ROLLING = ('README.md', 'client/README.md', 'config/README.md', 'shared/proto/README.md',
           'docs/开发阶段与验收清单.md', 'docs/正式开发推进记录.md', 'docs/架构适配与规则优先级.md')
TITLE = '宠物出生表现与隔离客户端已通过rule18限定验收'
REQUIRED = {
    'root': ['schema_version', 'date', 'status', 'runtime', 'verification', 'main_creator', 'isolated_creator', 'gui'],
    'runtime': ['rule_version', 'preview', 'skills', 'summon17_enabled', 'casting_supply_enabled', 'bootstrap_mode',
                'legacy_pet_trains', 'summon_candidate_runtime_enabled', 'summon_candidate_required_rule_version',
                'table_count', 'record_count', 'release_blockers', 'runtime_ready', 'production_ready'],
    'verification': ['go_full_mongo_ws_race_passed_including_subtests', 'go_skipped', 'go_failed', 'go_inputs',
                     'go_source_stable', 'configuration_python_passed', 'javascript_ui_app_passed',
                     'javascript_scene_passed', 'javascript_total_passed', 'client_inputs', 'client_external_engine_inputs', 'pb_messages',
                     'pb_long_fields', 'pb_boundary_cases', 'pb_protocol_ids', 'creator_seconds', 'creator_exit_code',
                     'saved_layouts_preserved', 'source_town_artifact_files_unchanged', 'source_added_png_frames',
                     'source_added_wav_files', 'source_creator_imported_meta_files', 'previous_development_assets_preserved',
                     'gameplay_excel_changed_cells', 'only_profile_rule_17_to_18', 'architecture_workbook_preserved'],
    'main_creator': ['passed', 'seconds', 'exit_code', 'source_stable'],
    'isolated_creator': ['passed', 'seconds', 'exit_code', 'source_stable'],
    'gui': ['passed', 'checks', 'errors', 'warnings', 'privateCastInvoked', 'routeOverride', 'bodySeeded'],
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def integer(value, label, positive=False):
    require(type(value) is int and value >= (1 if positive else 0), '缺少真实非负整数：' + label)
    return value


def positive_number(value, label):
    require(type(value) in (int, float) and 0 < value < float('inf'), '缺少真实正数：' + label)
    return value


def load_stage():
    report = json.loads((ROOT / STAGE).read_text(encoding='utf-8'))
    require(isinstance(report, dict), '阶段验收必须为对象')
    for field in REQUIRED['root']:
        require(field in report, '阶段验收缺少字段：' + field)
    require(report['schema_version'] == 1, '不支持的阶段验收schema')
    require(report['status'] == 'limited_passed', '仅status=limited_passed允许准备通过文案')
    datetime.date.fromisoformat(report['date'])
    for group, names in REQUIRED.items():
        if group == 'root':
            continue
        require(isinstance(report[group], dict), group + '必须为对象')
        for name in names:
            require(name in report[group], '阶段验收缺少字段：' + group + '.' + name)
    runtime, verification = report['runtime'], report['verification']
    expected = {'rule_version': 18, 'skills': 22, 'summon17_enabled': False, 'casting_supply_enabled': False,
                'bootstrap_mode': 'legacy_untrained_pet', 'legacy_pet_trains': False,
                'summon_candidate_runtime_enabled': False, 'summon_candidate_required_rule_version': 17,
                'table_count': 98, 'record_count': 965, 'release_blockers': 715,
                'runtime_ready': False, 'production_ready': False}
    for key, value in expected.items():
        require(type(runtime[key]) is type(value) and runtime[key] == value, '当前开放边界不符：runtime.' + key)
    require(isinstance(runtime['preview'], str) and bool(runtime['preview'].strip()), '预览版本必须显式存在')
    booleans = ['go_source_stable', 'previous_development_assets_preserved', 'only_profile_rule_17_to_18', 'architecture_workbook_preserved']
    for key in booleans:
        require(verification[key] is True, '未证明冻结/保留：verification.' + key)
    for key in REQUIRED['verification']:
        if key not in booleans and key != 'creator_seconds':
            integer(verification[key], 'verification.' + key)
    require(verification['go_failed'] == verification['go_skipped'] == 0, '完整Go不能有失败或跳过')
    for key in ['go_full_mongo_ws_race_passed_including_subtests', 'go_inputs', 'configuration_python_passed',
                'javascript_ui_app_passed', 'javascript_scene_passed', 'javascript_total_passed', 'client_inputs', 'client_external_engine_inputs',
                'pb_messages', 'pb_long_fields', 'pb_boundary_cases', 'pb_protocol_ids', 'source_town_artifact_files_unchanged']:
        integer(verification[key], key, positive=True)
    require(verification['javascript_total_passed'] == verification['javascript_ui_app_passed'] + verification['javascript_scene_passed'], 'JS总数必须来自UI/app与场景实际之和')
    for key, value in {'saved_layouts_preserved': 10, 'source_added_png_frames': 20, 'source_added_wav_files': 2,
                       'source_creator_imported_meta_files': 23, 'gameplay_excel_changed_cells': 1, 'creator_exit_code': 36}.items():
        require(verification[key] == value, '本阶段素材/布局/单格合同不符：verification.' + key)
    positive_number(verification['creator_seconds'], 'verification.creator_seconds')
    for name in ['main_creator', 'isolated_creator']:
        creator = report[name]
        require(creator['passed'] is True and creator['source_stable'] is True and type(creator['exit_code']) is int and creator['exit_code'] == 36, name + '未真实成功或输入未冻结')
        positive_number(creator['seconds'], name + '.seconds')
    require(report['main_creator']['seconds'] == verification['creator_seconds'], '主Creator耗时摘要与verification不一致')
    gui = report['gui']
    require(gui['passed'] is True, '隔离正常触屏GUI尚未通过')
    require(isinstance(gui['checks'], list) and len(gui['checks']) > 0 and all(isinstance(item, str) and item for item in gui['checks']), 'GUI实际checks必须显式存在')
    require(isinstance(gui['errors'], list) and len(gui['errors']) == 0, 'GUI仍有错误或缺少errors')
    require(isinstance(gui['warnings'], list), 'GUI必须保留真实warnings数组')
    for key in ['privateCastInvoked', 'routeOverride', 'bodySeeded']:
        require(gui[key] is False, 'GUI不得通过私有施法/运行时替换/预造身体：' + key)
    return report


def replace_once(text, old, new, label):
    require(text.count(old) == 1, '文档锚点缺失或重复：' + label)
    return text.replace(old, new, 1)


def prepend(text, addition):
    require('\n\n' in text, '文档缺少首标题')
    require(TITLE not in text[:2500], 'rule18当前摘要已存在，拒绝重复写入')
    heading, rest = text.split('\n\n', 1)
    return heading + '\n\n' + addition + '\n\n以下保留各自历史阶段范围；当前运行状态以上述rule18与最新限定验收为准。\n\n' + rest


def rewrite_row(text, prefix, value):
    lines = text.splitlines()
    matches = [i for i, line in enumerate(lines) if line.startswith(prefix)]
    require(len(matches) == 1, '文档条目缺失或重复：' + prefix)
    lines[matches[0]] = value
    return '\n'.join(lines) + '\n'


def build_updates(report):
    runtime, v, gui = report['runtime'], report['verification'], report['gui']
    original = {path: (ROOT / path).read_text(encoding='utf-8') for path in (*ROLLING, BOUNDARY)}
    updated = dict(original)
    count, warnings = len(gui['checks']), len(gui['warnings'])
    closed = '主仍22技能、原商店与初始赠书、legacy免费未训宠及护符关闭；独立17候选required_rule_version=17、runtime_enabled=false，护符包Count单位与售价仍待用户答复。'
    visual = 'PetBorn仅在fresh17真正安装、原current World及成功SyncReady的完整World.Ref/Auth stamp仍匹配、含body完整基线全组发送后发出；非当前或未Ready不积压。旧附着quiescing仍可观察出生，输入门槛不开放；Cancel不重播，新Entry清scope。'
    journal = '材料未决日志严格区分17无target的8字段与14/15原目标形状，先校验持久化再发40014；落盘前或失败不发送，未知保留原op重查，不补零坐标。'
    editor_cache = '唯一可再生Editor问卷缓存settings/v2/packages/information.json以excluded_generated_editor_caches单项数组声明，生成文件另记generated_editor_cache_sha256；原tracked字节与SHA已移.work/quarantine/creator-information-20261029，gitignore只排精确项，其它settings/源/产物严核。源码确认Editor问卷与cache查询，runtime splash另读SettingsCategory.SPLASH_SCREEN；实际.ccc缓存非明文，未声称读出正文form/SID。不改splash或授权，不作授权旁路；旧425 compile36但SID漂移的guard失败保留历史。'
    build_policy = '独立静态端口18082；18081已有服务不接管。仅声明单项Editor缓存排除后的source/input全树、完整build与同源public全树SHA严格相等可复用已真实构建的不变包，TS变化必须重构，不运行时替换资源。' + editor_cache
    audio = '声音在保存Safe的TOUCH_END捕获阶段先于Button停冒泡解锁，全局input仅无UI认领触摸fallback；current拒旧scope。Safe公开NODE_DESTROYED提前退休音频，全局input始终主动退订；正常主动释放按原root/context/capture退订根监听，根销毁时由引擎自清事件表。isValid(strict)不含Destroying，另记根退休；排队组件先解绑clip且不重复destroy，实际已毁组件不再触碰，旧释放不清同Safe新声道。两段原WAV按完整剩余时长播放；GPU帧和AudioSource实际播放观察不等于硬件听感或手机真机。'
    summary = (f"{TITLE}：{v['go_full_mongo_ws_race_passed_including_subtests']}项全包Go race（开启真实Mongo/WS，0跳过、{v['go_inputs']}份输入稳定）、"
               f"{v['configuration_python_passed']}项配置Python、{v['javascript_total_passed']}项JS（{v['javascript_ui_app_passed']} UI/app、{v['javascript_scene_passed']}场景；{v['client_inputs']}份项目输入与另{v['client_external_engine_inputs']}份锁定引擎源码分别冻结）、"
               f"{v['pb_messages']}种PB消息/{v['pb_long_fields']}个64位字段/{v['pb_boundary_cases']}个Long边界/{v['pb_protocol_ids']}个消息号通过；"
               f"主Creator{report['main_creator']['seconds']}秒、独立候选包原真实Creator构建{report['isolated_creator']['seconds']}秒，均exit36且输入稳定；仅声明单项Editor缓存排除后的受控全树SHA相等时允许复用真实产物。"
               f"隔离19级/原书25/Count2后的正常触屏{count}项、0错误/{warnings}警告通过，覆盖学习装备、新生、无符Recall与健康重入。"
               + closed + f"两本Excel仅玩法profile版本17→18的{v['gameplay_excel_changed_cells']}处单格变更；新增{v['source_added_png_frames']}张原PNG、{v['source_added_wav_files']}段原WAV及{v['source_creator_imported_meta_files']}份Creator导入meta，"
               f"{v['saved_layouts_preserved']}份保存布局、架构书及冻结旧资源保持。" + journal + audio + editor_cache)
    for path, prefix in [('README.md', 'docs/'), ('docs/开发阶段与验收清单.md', ''), ('docs/架构适配与规则优先级.md', '')]:
        updated[path] = prepend(updated[path], summary + f'见[阶段验收]({prefix}宠物出生表现与隔离客户端阶段验收.json)。')
    path = 'docs/架构适配与规则优先级.md'
    require('| A41 |' not in updated[path], 'A41已存在，拒绝重复或覆盖')
    lines = updated[path].splitlines()
    anchors = [i for i, line in enumerate(lines) if line.startswith('| A40 |')]
    require(len(anchors) == 1, 'A40锚点不唯一')
    lines.insert(anchors[0] + 1, '| A41 | adapt | rule18只对真正fresh17新生发送临时30015；成功SyncReady锁原连接/完整Auth/完整World.Ref，完整含body基线先发，非当前/未Ready不积压。客户端按附着/handle/Life/definition/map/运动水位及server elapsed裁剪一次出场；旧附着quiescing继续观察但不放开输入，Cancel不重播、新Entry清scope。17无target八字段与14/15原目标形状严格分支，持久化前/失败禁发。保存Safe捕获触摸解锁，global input仅无UI认领fallback；旧scope拒绝。Safe公开NODE_DESTROYED早退、global总退，根事件表由引擎自清；isValid(strict)不含Destroying，排队声道先清clip不重复destroy，实际已毁不碰，旧释放不清新声道。原10×100ms加色与两原声分时钟，声音播完整剩余时长；独立包只精确排除Editor问卷缓存information.json并另记生成SHA，其它settings/源/产物全树严格核验，TS变必须重构；不改splash/授权，旧SID guard失败保留。正常触屏不等于主17/购符开放、硬件听感或自然养成；主22/legacy/护符关闭及715正式阻断保持。 | 《宠物出生表现与隔离客户端实施边界.md》与本阶段限定验收；源Extra与Monster78.Lib动作/原WAV |')
    updated[path] = '\n'.join(lines) + '\n'
    path = 'README.md'
    updated[path] = rewrite_row(updated[path], '- 宠物底座：', '- 宠物底座：rule18补真实fresh出生事件、一次原帧加色出场及原声；完整World.Ref/Auth、原附着/旧Life和quiescing边界保持。主仍legacy不训练，17/购符关闭。')
    updated[path] = rewrite_row(updated[path], '- 验证：', f"- 验证：{v['go_full_mongo_ws_race_passed_including_subtests']}项全包Go race（含真实Mongo/WS）、{v['configuration_python_passed']}项配置Python、{v['javascript_total_passed']}项JS（项目{v['client_inputs']}份输入，另{v['client_external_engine_inputs']}份引擎源独立冻结）、{v['pb_boundary_cases']}个PB边界；主/独立原真实Creator构建分别{report['main_creator']['seconds']}/{report['isolated_creator']['seconds']}秒通过，只有声明单项Editor缓存排除后的受控全树SHA相等才复用。{v['saved_layouts_preserved']}布局与冻结旧资源保持；玩法只改{v['gameplay_excel_changed_cells']}格profile版本，架构书未变。")
    updated[path] = rewrite_row(updated[path], '- 本机观察：', f'- 本机观察：独立rule18候选包正常道士触屏{count}项通过，0错误/{warnings}警告；19级/初始原书25/Count2明确隔离。GPU原帧、真实AudioSource播放与健康不重播已观察，硬件听感、自然购符、尸体后重召GUI及真机/容量另验。')
    path = 'client/README.md'
    updated[path] = replace_once(updated[path], '当前为rule17、22项', '当前为rule18、22项', path + '版本')
    begin = updated[path].find('当前Owned自有活宠与跨图观察')
    end = updated[path].find('\n\n开发前先读', begin)
    require(begin >= 0 and end > begin, 'clientREADME当前摘要锚点不符')
    current = (f"当前rule18新生表现通过{v['javascript_total_passed']}项JS（{v['javascript_ui_app_passed']} UI/app、{v['javascript_scene_passed']}场景）及{v['pb_boundary_cases']}个PB边界；{v['client_inputs']}份项目输入与另{v['client_external_engine_inputs']}份锁定引擎源码分别冻结，不合并冒称同一输入集。"
               f"主Creator{report['main_creator']['seconds']}秒、独立候选包原真实构建{report['isolated_creator']['seconds']}秒，{v['saved_layouts_preserved']}份保存布局与旧资源保持。"
               + visual + ' Session最多保留1条pending，资源/visual更换保留消费水位；baseline/Owned首次出现不能推断出生，也不创建身体。'
               + f'原出场10帧×100ms加色，17施法Magic原帧600ms，原WAV独立完整播放。隔离正常触屏{count}项0错误/{warnings}警告，覆盖学习、装备、新生、卸符Recall与健康重入。'
               + journal + build_policy + closed + audio + '见[阶段验收](../docs/宠物出生表现与隔离客户端阶段验收.json)。')
    updated[path] = updated[path][:begin] + current + updated[path][end:]
    path = 'docs/开发阶段与验收清单.md'
    updated[path] = rewrite_row(updated[path], '| T030 |', '| T030 | 部分实现，整体待核验 | 主22项与原获取升级保持；rule17召唤规则/Owned/原子材料与训练底座后，rule18补真实新生事件/原帧出场/原声、独立候选包正常触屏学习装备→新生→无符Recall→健康不重播 | 主17/购符仍闭门，Count包规格待用户答复；19级/初始原书25/Count2为隔离前态，不代表自然供给、尸体重召GUI、硬件听感、完整三职业或1.80 | 策划4.8/4.20；宠物出生表现与隔离客户端阶段验收.json |')
    updated['docs/正式开发推进记录.md'] = prepend(updated['docs/正式开发推进记录.md'], report['date'] + '：' + summary + visual + '见[阶段验收](宠物出生表现与隔离客户端阶段验收.json)。')
    path = 'config/README.md'
    updated[path] = replace_once(updated[path], '## 当前rule17召唤与地图禁宠候选', '## rule17召唤与地图禁宠候选：历史验收', path + '历史标题')
    config = (f"## 当前rule18出生显示协议边界\n\n预览`{runtime['preview']}`，{runtime['table_count']}表{runtime['record_count']}记录、{runtime['release_blockers']}项正式阻断保持。"
              f"仅玩法Excel profile版本17→18的{v['gameplay_excel_changed_cells']}格受控变更，架构书、其它数据及公式保持；主22技能、原商店/赠书、legacy未训宠、护符false、187未知容量不变。"
              '独立17候选仍required_rule_version=17/runtime_enabled=false，本阶段不猜护符包Count或售价，不因协议/素材能力开放主17。\n\n'
              f"新增{v['source_added_png_frames']}张原PNG、{v['source_added_wav_files']}段原WAV和{v['source_creator_imported_meta_files']}份Creator真实导入meta来自锁定来源，不新增出生业务延迟配置。"
              '500ms业务到期、2000ms走攻就绪、1000ms图像序列、原声音时长各自保留；只有独立局部fixture/public明确纳入17与Count2，不能反写唯一Excel或主public。' + build_policy
              + '见[阶段验收](../docs/宠物出生表现与隔离客户端阶段验收.json)。')
    updated[path] = prepend(updated[path], config)
    path = 'shared/proto/README.md'
    proto = (f"当前rule18共{v['pb_messages']}个PB数据消息、{v['pb_protocol_ids']}个消息号、{v['pb_long_fields']}个64位字段及{v['pb_boundary_cases']}个Long边界案例，仍属draft开发合同。"
             '新增纯S2C30015 PetBorn；14字段承载精确scene/baseline/state_sequence/handle/Life/definition/map/cast/skill/出生Unix/运动水位/方向/RoleKey/server elapsed。movement_revision=0合法；64位保持Long/BigInt，不上传坐标，不新增经济游标。\n\n'
             + visual + ' 客户端消费最多1条pending并保留Life水位；健康基线/Owned/永久回执不补播出生。显示发送失败不回滚宠物或材料，不重投新票。'
             + closed + '见[阶段验收](../../docs/宠物出生表现与隔离客户端阶段验收.json)。')
    updated[path] = prepend(updated[path], proto)
    boundary = updated[BOUNDARY].split('\n\n', 2)
    require(len(boundary) == 3 and '状态：实施中' in boundary[1], '实施边界文首已变或不再实施中')
    boundary[1] = ('日期：' + report['date'] + '。状态：rule18限定验收通过；具体检查、真实构建与正常触屏范围见[阶段验收](宠物出生表现与隔离客户端阶段验收.json)。'
                   '来源映射、组件检查、真实业务与硬件听感分别登记，不据此宣布完整1.80、三职业或P0完成。')
    updated[BOUNDARY] = '\n\n'.join(boundary)
    require(set(updated) == set((*ROLLING, BOUNDARY)), '写入目标超出七份rolling与本阶段实施边界')
    require(all(updated[path] != original[path] for path in updated), '存在未更新或空更新目标')
    return original, updated


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true', help='正式验收通过后显式更新；默认仅检查并预览')
    parser.add_argument('--print-requirements', action='store_true', help='只打印必填字段，不读取或写入正式文档')
    args = parser.parse_args()
    if args.print_requirements:
        print(json.dumps({'status': 'limited_passed', 'required': REQUIRED}, ensure_ascii=False, indent=2))
        return
    report = load_stage()
    original, updated = build_updates(report)
    summary = {'passed': True, 'applied': False, 'source': STAGE, 'status': report['status'], 'rule_version': report['runtime']['rule_version'],
               'rolling_documents': len(ROLLING), 'implementation_front_matter': 1, 'targets': list(updated),
               'javascript_total_passed': report['verification']['javascript_total_passed'], 'gui_checks': len(report['gui']['checks'])}
    if args.apply:
        # 全部输入与所有文本锚点先检查完成，写前再核当前字节，拒绝中途文档被另一任务更改。
        for path in original:
            require((ROOT / path).read_text(encoding='utf-8') == original[path], '准备期间文档发生变化：' + path)
        backup = ROOT / '.work/pet-visual-20261029/docs-before-rule18'
        require(not backup.exists(), '保留原文与上次现场，拒绝覆盖文档备份')
        backup.mkdir(parents=True)
        hashes = {}
        for path in original:
            target = backup / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes((ROOT / path).read_bytes())
            hashes[path] = hashlib.sha256(target.read_bytes()).hexdigest()
        (backup / 'before.json').write_text(json.dumps(hashes, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
        for path, text in updated.items():
            (ROOT / path).write_text(text, encoding='utf-8', newline='\n')
        summary['applied'] = True
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
