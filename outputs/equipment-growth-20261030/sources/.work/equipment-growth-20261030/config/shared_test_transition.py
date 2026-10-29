"""只核阶段22共享测试文件三处版本字段迁移；不放宽其余原配置SHA。"""
from collections import Counter
import hashlib
import json


DOCUMENT = '.work/equipment-growth-20261030/config/shared-test-transition.json'
CONFIG_REPORT = '.work/equipment-growth-20261030/config/final-review.json'
CONFIG_SHA = 'a65b2b56f7aea010f110133ee6e493e9982d35e5957fcb3a714dafd3389b06e5'
TEST_PATH = 'server/internal/integration/level_growth_test.go'
OLD_SHA = 'ddd46cea404761e8a288279a0724b7d4269c9b7448c1d663ec3183b9c654342c'
NEW_SHA = 'c1b7a7e1ffa2fc044c53017bb05822a282d6d997dffae22a534375dda163200d'
GO_REPORT = '.work/equipment-growth-20261030/go/full-3/report.json'
GO_SHA = 'a922746463223c2da886cdc85db65c6e57eaceb3078f2a258f5d26bf913dbd3f'
GO_EVENTS = '.work/equipment-growth-20261030/go/full-3/events.jsonl'
EVENTS_SHA = 'c209fd1235344b06043e1407b4ddb9c4b5c226bf7c7b2f9303a5fb516b4f12cd'
REMOVALS = [{'utf8': ', ItemVersion: uint64(cloth.Version)', 'occurrences': 2},
            {'utf8': ', ItemVersion: wood.Version', 'occurrences': 1}]
REQUIRED = [{'package': 'minimir/internal/integration', 'test': name} for name in [
    'TestLevelGrowthMigrationCapReceiptsAndEquipmentMongo',
    'TestLevelGrowthNaturalStarterLearningRecoveryAndReconnectMongoWS']]


def need(ok, message):
    if not ok:
        raise ValueError(message)


def digest(raw):
    need(type(raw) is bytes, '共享迁移只接受原始bytes，不进行EOL转换')
    return hashlib.sha256(raw).hexdigest()


def verify(reader):
    """reader(repoPath)->原始bytes；归档器读工作树，索引器读index或已核归档原blob。"""
    document = json.loads(reader(DOCUMENT))
    need(set(document) == {'schema_version', 'recorded_at_utc', 'scope', 'original_configuration_report', 'shared_test', 'go3', 'invariants'}
         and type(document['schema_version']) is int and document['schema_version'] == 1, '共享测试迁移证据字段不同')
    need(document['original_configuration_report'] == {'path': CONFIG_REPORT, 'sha256': CONFIG_SHA, 'python_tests': 130}, '原130配置报告身份不同')
    need(document['shared_test'] == {'path': TEST_PATH, 'configuration_sha256': OLD_SHA, 'go3_sha256': NEW_SHA,
         'reconstruction_remove': REMOVALS, 'total_insertions': 3}, '共享迁移不能泛化为其它路径、片段或SHA')
    need(document['go3'] == {'report_path': GO_REPORT, 'report_sha256': GO_SHA, 'events_path': GO_EVENTS,
         'events_sha256': EVENTS_SHA, 'required_raw_passes': REQUIRED}, '共享迁移没有绑定真实Go3及两原测试')
    need(document['invariants'] == {'other_configuration_inputs': 'strict_original_hashes',
         'original_report_or_logs_rewritten': False, 'python130_covers_new_go_test_bytes': False}, '共享迁移扩大旧130范围')
    original_report = reader(CONFIG_REPORT)
    need(digest(original_report) == CONFIG_SHA, '原配置报告字节变化')
    configuration = json.loads(original_report)
    need(configuration['python_tests'] == 130 and configuration['current_hashes'][TEST_PATH] == OLD_SHA, '原130及共享旧SHA不符')
    raw = reader(TEST_PATH)
    need(digest(raw) == NEW_SHA, '共享测试当前字节不是已核Go3版本')
    reconstructed = raw
    for removal in REMOVALS:
        fragment = removal['utf8'].encode('utf-8')
        need(reconstructed.count(fragment) == removal['occurrences'], '共享测试3片段实际次数不同')
        reconstructed = reconstructed.replace(fragment, b'')
    need(digest(reconstructed) == OLD_SHA, '精确剔3处ItemVersion未还原原配置SHA')
    report_raw, events_raw = reader(GO_REPORT), reader(GO_EVENTS)
    need(digest(report_raw) == GO_SHA and digest(events_raw) == EVENTS_SHA, '共享迁移Go3原report/events字节变化')
    go = json.loads(report_raw)
    need(go['passed'] is True and go['exit_code'] == 0 and go['mongo_e2e_explicit'] is True and go['race_detector'] is True
         and go['failed_tests'] == [] and go['skipped_tests'] == [] and go['source_changed_during_test'] is False, '共享迁移Go3非真实全包Mongo/race无失败跳过终态')
    need(go['source_sha256'] == go['source_sha256_after'] and go['source_sha256'][TEST_PATH] == NEW_SHA
         and go['events_sha256'] == EVENTS_SHA, '共享测试未锁到Go3前后源与raw事件')
    required = {(row['package'], row['test']) for row in REQUIRED}
    need(required <= {(row['package'], row['test']) for row in go['passed_tests']}, 'Go3报告缺两条成长测试原PASS')
    passes = Counter()
    for line in events_raw.splitlines():
        if not line.strip():
            continue
        event = json.loads(line)
        key = (event.get('Package'), event.get('Test'))
        if key in required:
            need(event['Action'] not in {'fail', 'build-fail', 'skip'}, '两条共享测试原raw有失败/跳过')
            if event['Action'] == 'pass':
                passes[key] += 1
    need(passes == Counter({key: 1 for key in required}), '两条共享测试raw PASS缺失或重复')
    effective = dict(configuration['current_hashes'])
    effective[TEST_PATH] = NEW_SHA
    for name, expected in effective.items():
        need(digest(reader(name)) == expected, '配置输入原字节变化（仅已核单路径允许迁移）：' + name)
    return effective
