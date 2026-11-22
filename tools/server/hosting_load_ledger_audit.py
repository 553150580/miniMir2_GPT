"""压测服务退出后核对隔离库的收益来源、永久回执和实际托管扣时。"""

from collections import Counter
from dataclasses import dataclass
import time

from pymongo import MongoClient, timeout
from pymongo.errors import PyMongoError

from hosting_load_mongo_samples import isolated_mongo_uri


SETTLEMENT_ID_RANGE = {'_id': {'$gte': 'settle:', '$lt': 'settle;'}}
AUDIT_FIELDS = {'kind': 1, 'role_id': 1, 'source_seq': 1, 'host_seconds': 1, 'kills': 1,
                'experience': 1, 'gold': 1, 'hosting_session': 1, 'rule_version': 1}
RECEIPT_FIELDS = {'kind': 1, 'role_id': 1, 'source_seq': 1, 'digest': 1,
                  'settlement.rule_version': 1, 'settlement.experience_grant.raw_experience': 1,
                  'settlement.host_seconds': 1, 'settlement.kills': 1}


@dataclass
class Flow:
    count: int = 0
    min_seq: int = 0
    max_seq: int = 0
    debited_seconds: int = 0

    def record(self, seq, debited_seconds):
        self.min_seq = min(self.min_seq, seq) if self.count else seq
        self.max_seq = max(self.max_seq, seq)
        self.count += 1
        self.debited_seconds += debited_seconds


def _load_roles(db, seed, issue):
    flows = {}
    for index, role in enumerate(db.roles.find(
            {}, {'name_key': 1, 'source_seq': 1, 'hosting_seconds': 1}).sort('name_key', 1)):
        role_id = role['_id']
        if role.get('name_key') != f'load{index:05d}':
            issue('角色名不符合本轮预置', role_id)
        balance = seed['hosting_seconds_each'] if index < seed['hosting_roles_seeded'] else 0
        hosting = role.get('hosting_seconds', 0)
        seq = role.get('source_seq', 0)
        if not all(isinstance(value, int) and value >= 0 for value in (seq, hosting)):
            issue('最终角色水位或余额无效', role_id)
            continue
        flows[role_id] = (seq, balance - hosting, Flow())
    if len(flows) != seed['roles']:
        issue('最终角色数不符合本轮预置', len(flows))
    return flows


def _record_pair(event, receipt, flows, counts, issue):
    role_id = event.get('role_id')
    seq = event.get('source_seq')
    debited = event.get('host_seconds')
    kills = event.get('kills')
    experience = event.get('experience')
    gold = event.get('gold')
    key = event['_id']
    if (event.get('kind') != 'settlement' or receipt.get('kind') != 'settlement' or
            not all(isinstance(value, int) and value >= 0
                    for value in (seq, debited, kills, experience, gold)) or seq == 0 or
            not isinstance(role_id, str) or role_id not in flows or key != f'settle:{role_id}:{seq}' or
            (receipt.get('role_id'), receipt.get('source_seq')) != (role_id, seq) or
            not receipt.get('digest')):
        issue('收益回执或审计身份无效', key)
        return
    details = receipt.get('settlement')
    if (not isinstance(details, dict) or not details.get('rule_version') or
            details['rule_version'] != event.get('rule_version')):
        issue('收益回执缺少对应的规则版本', key)
        return
    expected_experience = (details.get('experience_grant') or {}).get('raw_experience', 0)
    if (expected_experience != experience or details.get('host_seconds', 0) != debited or
            details.get('kills', 0) != kills or
            debited and not event.get('hosting_session')):
        issue('收益回执与审计内容不一致', key)
        return
    flows[role_id][2].record(seq, debited)
    counts['matched_settlements'] += 1
    counts['host_seconds'] += debited
    counts['kills'] += kills
    counts['raw_experience'] += experience
    counts['direct_gold'] += gold
    if kills:
        counts['kill_settlements'] += 1
    if debited:
        counts['debit_settlements'] += 1
        counts['combined_kill_debit' if kills else 'debit_without_kill'] += 1


def _scan(db, seed, counts, issue):
    flows = _load_roles(db, seed, issue)
    counts['roles'] = len(flows)
    counts['roles_with_source'] = sum(seq > 0 for seq, _, _ in flows.values())
    # 该档战士只走挂机结算来源。若脚本引入技能耗材等其他来源，先记录再拒绝套用此口径。
    extra = db.operations.find_one(
        {'$or': [{'_id': {'$lt': 'settle:'}}, {'_id': {'$gte': 'settle;'}}],
         'source_seq': {'$gt': 0}}, {'kind': 1})
    if extra is not None:
        issue('本轮存在收益以外的角色来源', extra.get('kind'))
    with db.audit_outbox.find(SETTLEMENT_ID_RANGE, AUDIT_FIELDS).sort('_id', 1).batch_size(8192) as audit:
        with db.operations.find(SETTLEMENT_ID_RANGE, RECEIPT_FIELDS).sort('_id', 1).batch_size(8192) as receipts:
            # _id唯一索引上的归并保持固定批次内存，并准确定位仅存在一侧的记录。
            event, receipt = next(audit, None), next(receipts, None)
            while event is not None or receipt is not None:
                if receipt is None or (event is not None and event['_id'] < receipt['_id']):
                    issue('收益审计缺少永久回执', event['_id'])
                    counts['audit_rows'] += 1
                    event = next(audit, None)
                elif event is None or receipt['_id'] < event['_id']:
                    issue('收益回执缺少审计', receipt['_id'])
                    counts['receipt_rows'] += 1
                    receipt = next(receipts, None)
                else:
                    _record_pair(event, receipt, flows, counts, issue)
                    counts['audit_rows'] += 1
                    counts['receipt_rows'] += 1
                    event, receipt = next(audit, None), next(receipts, None)
    # 全部kind=settlement应带可归并前缀。另核角色来源水位与按角色汇总的实际扣时。
    for collection, field in ((db.operations, 'receipt_rows'), (db.audit_outbox, 'audit_rows')):
        if collection.count_documents({'kind': 'settlement'}) != counts[field]:
            issue('存在不符合收益记录身份的文档', collection.name)
    for role_id, (seq, expected_debit, flow) in flows.items():
        if flow.count != seq or flow.max_seq != seq or flow.min_seq != (1 if seq else 0):
            issue('角色收益流水不连续或与持久水位不符', role_id)
        if flow.debited_seconds != expected_debit:
            issue('角色实际扣时与已确认审计不符', role_id)
        counts['roles_with_debit'] += int(flow.debited_seconds > 0)
        counts['host_seconds_from_balances'] += expected_debit


def audit_soak_ledger(endpoint, seed):
    started = time.monotonic()
    counts = Counter()
    errors = Counter()
    examples = []

    def issue(kind, sample):
        errors[kind] += 1
        if len(examples) < 20:
            examples.append({'kind': kind, 'sample': sample})

    uri = isolated_mongo_uri(endpoint)
    try:
        with MongoClient(uri, appname='hosting-load-ledger-audit', maxPoolSize=1,
                         connectTimeoutMS=3000, serverSelectionTimeoutMS=3000,
                         retryReads=False) as client, timeout(180):
            status = client.admin.command({'serverStatus': 1, 'wiredTiger': 0})
            if status.get('pid') != endpoint['pid'] or status.get('version') != endpoint['version']:
                issue('MongoDB 核账实例身份变化', status.get('pid'))
            else:
                _scan(client[endpoint['database']], seed, counts, issue)
    except PyMongoError as error:
        issue('未完成隔离账本复核', type(error).__name__)
    return {
        'passed': not errors,
        'seconds': time.monotonic() - started,
        'counts': dict(counts),
        'errors': dict(errors),
        'examples': examples,
    }
