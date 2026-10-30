"""归档本轮真实检查并更新限定验收记录；不生成或改写玩法数值。"""
import argparse
import datetime
import gzip
import hashlib
import json
from pathlib import Path, PurePosixPath
import shutil
import subprocess

ROOT = next(p for p in Path(__file__).resolve().parents if (p / 'tools/dev').is_file())
WORK = ROOT / '.work/p2-fixes-20261030'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def archive_file(source, destination):
    assert source.resolve().is_relative_to(ROOT) and destination.resolve().is_relative_to(ROOT)
    destination.parent.mkdir(parents=True, exist_ok=True)
    raw = source.read_bytes()
    if len(raw) > 150000 and source.suffix in {'.json', '.jsonl', '.txt', '.log'}:
        destination = destination.with_suffix(destination.suffix + '.gz')
        destination.write_bytes(gzip.compress(raw, mtime=0))
    else:
        shutil.copyfile(source, destination)


def archive_tree(source, destination, skip=()):
    for path in sorted(source.rglob('*')):
        relative = path.relative_to(source)
        if path.is_file() and not any(part in skip for part in relative.parts) and path.suffix not in {'.exe', '.png'}:
            name = relative.with_suffix('.txt') if relative.suffix == '.log' else relative
            archive_file(path, destination / name)


def prepend(relative, text):
    path = ROOT / relative
    raw = path.read_bytes()
    split = raw.find(b'\n') + 1
    encoded = ('\n' + text + '\n\n').encode('utf-8')
    assert text.encode('utf-8') not in raw
    path.write_bytes(raw[:split] + encoded + raw[split:])


def manifest(directory):
    files = {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(directory.rglob('*')) if p.is_file() and p.name != 'evidence-manifest.json'}
    save(directory / 'evidence-manifest.json', {'files': files})
    return {**files, (directory / 'evidence-manifest.json').relative_to(ROOT).as_posix(): sha(directory / 'evidence-manifest.json')}


def p2(args):
    server = load(WORK / 'server/full-2/report.json')
    go = load(WORK / 'server/full-2/latest.json')
    client = load(WORK / 'client/full-1/report.json')
    creator = load(WORK / 'creator/full-2/report.json')
    gui_path = Path(args.gui_report).resolve()
    gui = load(gui_path)
    runtime_dir = gui_path.parent.parent
    runtime = load(runtime_dir / 'report.json')
    assert server['passed'] and go['passed'] and not go['skipped_tests'] and client['passed'] and creator['passed']
    assert gui['passed'] and runtime['passed']
    for key in ['source_stable', 'main_preserved', 'mongo_preserved', 'candidate_preserved', 'served_main_preserved',
                'own_processes_stopped', 'new_database_dropped', 'temporary_config_removed']:
        assert runtime[key]
    assert not runtime['cleanup_errors']
    artifact_prefix = Path(creator['build_root']).relative_to(ROOT).as_posix()
    artifact = load(WORK / 'creator/full-2/candidate-files.json')
    expected = {PurePosixPath(name).relative_to(artifact_prefix).as_posix(): digest for name, digest in artifact.items()}
    assert expected == runtime['candidate_before']
    published = load(WORK / 'publish/run-2/report.json')
    assert published['passed'] and published['full_sha256_equal_after_same_volume_rename']
    destination = ROOT / 'outputs/p2-fixes-20261030'
    destination.mkdir(exist_ok=False)
    archive_tree(WORK / 'server', destination / 'server')
    archive_tree(WORK / 'client/full-1', destination / 'client')
    archive_tree(WORK / 'config', destination / 'config')
    archive_tree(WORK / 'creator', destination / 'creator', skip=('candidate',))
    archive_tree(runtime_dir, destination / 'browser')
    for path in runtime_dir.rglob('*.png'):
        archive_file(path, destination / 'browser' / path.relative_to(runtime_dir))
    archive_tree(WORK / 'publish', destination / 'publish', skip=('previous-web-mobile',))
    for path in [WORK / 'run_checks.py', WORK / 'publish.py', Path(__file__)]:
        archive_file(path, destination / 'sources' / path.name)
    result = {'status': 'limited_passed', 'recorded_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'baseline': '3c7b0c05f321e2d8c5b54878821981277d75a4e5', 'runtime_rule_version': 23,
              'scope': '六项P2缺陷修复及真实Mongo/Creator/触屏限定回归，不是完整P0、真机或容量验收',
              'fixes': ['健康重连/城镇/复活保留托管亚秒余量，DB等待等屏障不计费',
                        'supervisor向子节点传递实际加载的原配置路径',
                        '建角发送前持久原请求，未知及重入沿原操作核对，目录确认后清理',
                        '消息搜索及锚点恢复同步刷新List可见池',
                        '配置复用在切当前指针前核验全部现存产物摘要',
                        '协议检查比较唯一清单与双端生成ID常量'],
              'mongo_race_tests': len(go['passed_tests']), 'mongo_race_skipped': len(go['skipped_tests']),
              'go_vet_passed': True, 'javascript_tests': 504, 'config_python_tests': 147,
              'creator_exit_code': creator['exit_code'], 'creator_seconds': creator['seconds'],
              'saved_layouts_preserved': creator['scene_prefab_layout_preserved'],
              'old_preview_preserved_during_build': creator['served_build_preserved'],
              'artifact_transfer': published,
              'browser_checks': gui.get('checks'), 'browser_errors': gui.get('errors'), 'browser_warnings': gui.get('warnings'),
              'browser_isolated_message_prestate': True, 'business_message_sources_verified': False,
              'visual_reviewer': 'root', 'visual_review_completed': args.visual_reviewed, 'actual_view_image_count': 6,
              'gameplay_workbooks_changed': False, 'source_frozen_during_checks': True,
              'history': ['server/full-1与creator/full-1因外层长时会话中断未获完整终态，不计通过；full-2加入心跳并即时记录Creator退出码后重跑',
                          '首次预览切换在解析stop中文输出时于搬包前退出，原包未动并立即恢复服务；run-2修正解析后逐SHA受控切换'],
              'physical_mobile_verified': False, 'capacity_verified': False, 'full_p0_completed': False, 'production_ready': False}
    assert args.visual_reviewed
    result['evidence_sha256'] = manifest(destination)
    save(ROOT / 'docs/六项P2修复阶段验收.json', result)
    text = (f'2026-10-30：六项P2已通过限定修复验收：{len(go["passed_tests"])}项真实Mongo/WS全包Go race（0跳过）、全包vet、504项JS与147项配置回归通过，'
            f'Creator3.8.8实构exit36、{creator["seconds"]:.2f}秒，保存布局及构建期间原预览保持。健康托管3×800ms累计扣2秒留400ms；'
            '启动配置、原建角未知重查、消息可见池、配置产物与协议ID门禁已修。真实建角提交后丢响应再触屏核对通过；消息40条为明确隔离UI前态，不能当业务消息来源。'
            'rule23与唯一Excel保持，完整P0、真机与容量仍待验。见[修复验收](docs/六项P2修复阶段验收.json)。')
    for path in ['README.md', 'docs/正式开发推进记录.md', 'docs/架构适配与规则优先级.md', 'docs/开发阶段与验收清单.md']:
        prepend(path, text if path == 'README.md' else text.replace('(docs/六项P2', '(六项P2'))


def recovery():
    source = ROOT / '.work/startup-recovery-20261030'
    qualified = source / '20261030T144443Z-52c75df3/report.json'
    actual = load(qualified)
    for key in ['passed', 'source_stable', 'main_preserved', 'mongo_preserved', 'own_processes_stopped', 'new_database_dropped', 'temporary_config_removed']:
        assert actual[key]
    assert not actual['cleanup_errors']
    destination = ROOT / 'outputs/startup-recovery-20261030'
    destination.mkdir(exist_ok=False)
    archive_tree(source, destination, skip=('__pycache__',))
    for name in ['verify_startup_recovery.py', 'recovery_probe.cjs']:
        archive_file(ROOT / 'tools/server' / name, destination / 'sources' / name)
    result = {'status': 'limited_passed', 'recorded_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'input_base_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
              'runtime_rule_version': 23, 'source_inputs': len(actual['source_before']),
              'scope': '独立Windows实际进程、PB/HTTP/WS与新Mongo测试库的启动/入场/恢复子矩阵',
              'checks': actual['checks'],
              'main_preserved': True, 'mongo_preserved': True, 'new_database_dropped': True, 'temporary_config_removed': True,
              'own_processes_stopped': True, 'cleanup_errors': [], 'race_binary': True,
              'qualification_seeded': False, 'lease_modified_by_probe': False,
              'invalid_ticket_scope': 'closed_before_success仅证明未成功Entry/Ready，不声称观察到特定错误码',
              'injured_hp_reset_verified': False, 'skill_cooldown_verified': False,
              'pending_commit_fault_verified': False, 'managed_fault_billing_verified': False,
              'full_fault_matrix_verified': False, 'physical_mobile_verified': False, 'capacity_verified': False,
              'full_p0_completed': False, 'production_ready': False,
              'history': '前三attempt为探针接口/拒票关闭/出生采样断言问题，第四为旧冻结输入下有限通过，第五冻结285份最终输入后完整子范围通过；原报告全部保留'}
    result['evidence_sha256'] = manifest(destination)
    save(ROOT / 'docs/启动入场与故障恢复阶段验收.json', result)
    text = ('2026-10-30：启动、入场与恢复子矩阵已通过独立Windows实际进程限定验收，285份输入冻结；'
            'supervisor自定义配置与三子进程退出、首基线/错ACK/重复ACK、account停复机及预签票、game强杀后自然租约到期与run_epoch换代通过。'
            '冷首基线回原出生，已确认库存/钱包/21600秒与原领取typed回执保留，原op不重发；自有进程、新库和临时配置已清理，主预览/Mongo身份保持。'
            '拒票只记录成功入场前关闭；受伤回血、技能CD、提交未知、托管故障计费、room panic、网络分区及完整故障矩阵仍待验。'
            'rule23及唯一Excel保持，完整P0、真机与容量未完成。见[限定验收](docs/启动入场与故障恢复阶段验收.json)。')
    for path in ['README.md', 'docs/正式开发推进记录.md', 'docs/架构适配与规则优先级.md', 'docs/开发阶段与验收清单.md']:
        prepend(path, text if path == 'README.md' else text.replace('(docs/启动', '(启动'))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('stage', choices=['p2', 'recovery'])
    parser.add_argument('--gui-report')
    parser.add_argument('--visual-reviewed', action='store_true')
    args = parser.parse_args()
    p2(args) if args.stage == 'p2' else recovery()
