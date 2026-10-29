"""仅读取冻结源码与已存在窄测raw，更新peer报告；不运行测试或数据库。"""
from datetime import datetime, timezone
import argparse
import hashlib
import json
from pathlib import Path
import re
import runpy

ROOT = Path(__file__).resolve().parents[3]
WORK = ROOT / '.work/inventory-capacity-20261030'
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--full-go-report', type=Path)
args = parser.parse_args()


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


old = WORK / 'history/server-peer/before-offline-ws/review.json'
assert sha(old) == 'b68feab89f456ca611744c6193c19316fd1c8446c120760c877c8c6468fdde3e'
review = json.loads(old.read_text(encoding='utf-8'))
freeze_path = WORK / 'server/go-frozen.json'
frozen = json.loads(freeze_path.read_text(encoding='utf-8'))
assert frozen['frozen'] and len(frozen['go_sha256']) == 171 and len(frozen['runtime_input_sha256']) == 190
assert sha(freeze_path) == 'a1f987f9c4c40ac4c6534efb8545c16f9d3616665fcbfa366aefb6afca1331d8'
adaptations = WORK / 'server/full-1-test-adaptations'
previous_path = adaptations / 'go-frozen-before.json'
previous = json.loads(previous_path.read_text(encoding='utf-8'))
expected_adapted = {'server/internal/integration/level_growth_test.go', 'server/internal/integration/summon_pet_test.go',
                    'server/internal/integration/ownership_test.go'}
changed = {name for name, value in previous['runtime_input_sha256'].items() if frozen['runtime_input_sha256'].get(name) != value}
assert changed == expected_adapted
before_hashes = json.loads((adaptations / 'before-sha256.json').read_text(encoding='utf-8'))
for name in expected_adapted:
    assert sha(adaptations / 'before' / name) == before_hashes[name] == previous['runtime_input_sha256'][name]
for name in ['server/internal/game/combat.go', 'server/internal/game/runtime.go', 'server/internal/persistence/settlement.go']:
    assert previous['runtime_input_sha256'][name] == frozen['runtime_input_sha256'][name] == review['source_sha256'][name]
for group in ['runtime_input_sha256', 'go_sha256']:
    for name, value in frozen[group].items():
        assert sha(ROOT / name) == value, name
narrow = []
for folder, method in [('mongo-store-2', 'TestFullBagContinuousSourceRewardsAndHostingReceiptsMongo'),
                       ('mongo-auto-ws-1', 'TestAutoRecyclePolicyLockAndOnlineOfflineMongoWS'),
                       ('mongo-offline-full-ws-2', 'TestFullBagOfflineCombatAndChargingMongoWS')]:
    path = WORK / 'server' / folder / 'report.json'
    report = json.loads(path.read_text(encoding='utf-8'))
    events_path = path.parent / 'events.jsonl'
    assert report['exit_code'] == 0 and report['mongo_e2e_explicit'] and report['race_detector']
    assert report['source_changed_during_test'] is False and report['source_sha256'] == report['source_sha256_after']
    assert report['events_sha256'] == sha(events_path)
    events = [json.loads(line) for line in events_path.read_text(encoding='utf-8').splitlines() if line.strip()]
    terminal = [row for row in events if row.get('Test') == method and row['Action'] == 'pass']
    assert len(terminal) == 1 and not [row for row in events if row['Action'] in ['fail', 'skip', 'build-fail']]
    differences = [name for name, value in report['source_sha256_after'].items() if frozen['runtime_input_sha256'].get(name) != value]
    historical_test_only_changes = {'server/internal/integration/full_bag_offline_test.go',
        'server/internal/integration/summon_pet_test.go', 'server/internal/integration/level_growth_test.go',
        'server/internal/integration/ownership_test.go'}
    assert set(differences) <= historical_test_only_changes, differences
    narrow.append({'report': path.relative_to(ROOT).as_posix(), 'report_sha256': sha(path),
        'events': events_path.relative_to(ROOT).as_posix(), 'events_sha256': sha(events_path), 'method': method,
        'method_seconds': terminal[0]['Elapsed'], 'runner_seconds': report['elapsed_seconds'], 'exit_code': 0,
        'actual_mongo_and_race': True, 'inputs': len(report['source_sha256']), 'before_after_equal': True,
        'differences_from_final_freeze': differences,
        'scope': 'owner_executed_narrow_raw_recounted_by_read_only_peer_not_final_full_go'})
review['status'] = 'read_only_review_no_blocker_offline_ws_gap_closed_final_full_go_pending'
review['recorded_at_utc'] = datetime.now(timezone.utc).isoformat()
review['findings'] = []
review['resolved_findings'] = [{'id': 'offline_full_bag_ws_coverage', 'state': 'closed_by_independent_actual_ws',
    'evidence': narrow[-1]['report'],
    'scope': '正常断线后的原满包Auto由后台实际AI产生同一HostingSession的kill、EXP、扣时与弃物；不是直接Store提交或注入body。仅隔离前态下业务全链，不认证自然效率或容量。'}]
review['evidence_layers'].append({'path': 'server/internal/integration/full_bag_offline_test.go',
    'lines': [43, 61, 107, 130, 140, 167, 180, 188, 205, 230, 235, 237],
    'scope': '正常Create/Equip/Claim及原报价购书填满；Auto开启且首实际弃物后正常close，后台真实离线满包继续；固定SourceSeq/HostSession逐审计与typed原回执核对并健康新票同scene/Life，不补播旧弃物。测试中500初始绑金来自offer8实际报价，消费回0，1HP/零攻/零敏均为明确局部资格，非主数值或新经济来源。'})
review['narrow_evidence_verified'] = narrow
output = ''.join(row.get('Output', '') for row in events)
observed = re.search(r'SourceSeq=(\d+) kills=(\d+) used=(\d+) discard=(\d+) EXP=(\d+) raw=(\d+) gold=(\d+) bound_gold=(\d+) sameScene=(\d+) sameLife=(\d+)', output)
assert observed, '缺少独立离线WS原始观测行'
metrics = dict(zip(['source_seq', 'kills', 'used_seconds', 'discard_count', 'actual_experience', 'raw_experience',
                    'gold', 'bound_gold', 'scene', 'life'], map(int, observed.groups())))
review['offline_ws_observed_narrow_only'] = {**metrics, 'same_scene': True, 'same_life': True,
    'scope': '仅mongo-offline-full-ws-2原日志这一次随机观察，不作为后续full Go或GUI固定期待'}
review['go_freeze'] = {'path': freeze_path.relative_to(ROOT).as_posix(), 'sha256': sha(freeze_path),
    'production_and_tests_go_files': 171, 'runtime_inputs': 190, 'owned_paths': len(frozen['owned_go_paths']),
    'all_runtime_current_sha_verified': True, 'final_full_go_status': 'running_not_accepted'}
review['source_sha256'] = {name: sha(ROOT / name) for name in [*review['source_sha256'], 'server/internal/integration/full_bag_offline_test.go', *sorted(expected_adapted)]}
review['test_only_adaptations_after_failed_full_1'] = {'changed_paths': sorted(changed),
    'before_marker': previous_path.relative_to(ROOT).as_posix(), 'before_marker_sha256': sha(previous_path),
    'current_marker_sha256': sha(freeze_path), 'before_original_sources_sha256_verified': before_hashes,
    'all_other_runtime_inputs_unchanged': True, 'production_three_files_unchanged': True,
    'scope': '两份当前主配置测试guard/文案改21；旧ownership例无托管余额，预期由满包3改真正耗尽1并加强四钱包/原库存/弃物事实，不放宽实现或事务'}
review['pending_selected_full_go'] = 'go/full-2'
review['historical_review'] = {'path': old.relative_to(ROOT).as_posix(), 'sha256': sha(old), 'original_bytes_preserved': True}
review['limits'].append('窄测与最终冻结的具体测试文件差异逐报告列明：新offline账号长度修正，以及full-1终态后另3文件旧当前版本/StopReason断言适配；生产三文件和实际WS方法不借此放宽。窄测before/after稳定不等于最终全190 current。全包Go终态由Root另选，不能用本窄测追认。')
if args.full_go_report:
    selected = args.full_go_report.resolve()
    assert selected.is_relative_to(WORK / 'go')
    module = runpy.run_path(str(WORK / 'finalize_evidence.py'), run_name='readonly_go_peer_verification')
    full, counts = module['go_evidence'](selected, selected.parent / 'events.jsonl')
    assert full['source_sha256'] == frozen['runtime_input_sha256']
    review['status'] = 'read_only_review_complete_no_blocker_full_mongo_race_raw_current_verified'
    review['go_freeze']['final_full_go_status'] = 'raw_verified_passed'
    review.pop('pending_selected_full_go')
    review['selected_full_go'] = {'report': selected.relative_to(ROOT).as_posix(), 'report_sha256': sha(selected),
        'events_sha256': sha(selected.parent / 'events.jsonl'), 'wrapper_sha256': sha(selected.parent / 'wrapper.json'),
        'counts': counts, 'source_before_after_marker_current_equal': True,
        'scope': 'Root实际执行完整Mongo/WS race；本peer只读独立重数raw并核当前输入，不冒称自己重跑'}
target = WORK / 'server-peer/review.json'
target.write_text(json.dumps(review, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'review_sha256': sha(target), 'source_files': len(review['source_sha256']),
    'offline_gap': 'closed_by_actual_narrow_ws', 'full_go': review['go_freeze']['final_full_go_status']}, ensure_ascii=False))
