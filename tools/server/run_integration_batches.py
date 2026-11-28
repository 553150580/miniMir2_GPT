"""分批跑完整 Go 联机竞态回归，每批使用独立且可清理的 MongoDB 副本集。"""

import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'tools'), str(ROOT / 'tools/environment')]
import mongodb_hosting_load as mongo


GROUP_PREFIXES = {
    'identity': ('Authentication', 'RoleBinding', 'Rule24', 'MongoOwnership', 'DormantRoom',
                 'RetiredRoom', 'SafeZone', 'ServerDirectory'),
    'inventory': ('AutoRecycle', 'Equipment', 'FullBagContinuous', 'LevelGrowthMigration',
                  'MongoPermanent', 'MongoReceipt', 'PotionMongo'),
    'warehouses': ('Warehouse', 'SkillAsset', 'SkillPageMerge'),
    'hosting': ('ManaPotion', 'AutoTarget', 'CompactTown', 'FirstLoop', 'FullBagOffline',
                'HostingPurchase', 'HostingRoleCards',
                'LevelGrowthNatural', 'MapTravel'),
    'scene-lifecycle': ('MonsterRespawn', 'SharedTown'),
    'skills': ('Combat', 'Displacement', 'PetRuntime', 'SkillAttackSlots', 'SkillLearning', 'SummonPet', 'Taoist', 'Teleport'),
    'bosses': ('PersonalBoss', 'SharedWorldBoss', 'Dungeon'),
    'mirror-arena': ('MirrorArena',),
    'market': ('ItemMarket', 'GoldMarket'),
    'payments': ('Payment',),
    'mining': ('Mining',),
    'forge': ('Forge',),
    'luck': ('Lucky',),
    'weapon-oil': ('WeaponOil',),
    'recycle-oil': ('RecycleOil',),
    'wing': ('WingWorldBoss',),
    'wing-oil': ('WingOil',),
    'equipment-enhancement': ('EnhanceEquipmentSlot',),
    'equipment-refine': ('RefineEquipment',),
    'medal': ('Medal',),
    'rebirth': ('Rebirth',),
    'body-training': ('BodyTraining', 'BodyHits'),
}
# 市场的跨进程用例与实际数据库围栏、撮合和清退对账共用一轮隔离验收。
MARKET_PERSISTENCE_CASES = (
    'TestItemMarketBackgroundMongo', 'TestGoldMarketBackgroundPriorityMongo', 'TestGoldMarketBackgroundExpiryMongo',
    'TestMarketMergeAssetsMongo', 'TestMarketMergePlanAuthorityMongo', 'TestMarketMergeWriteFenceMongo',
)
# 支付另验订单归属和幂等履约，测试组使用与现金卡工作簿一致的隔离运行配置。
PAYMENT_PERSISTENCE_CASES = ('TestPaymentOrderAuthorityMongo', 'TestPaymentGrantMongo')
# 挖矿联机与原矿来源、扣费、工具耐久共享同一份隔离投影。
MINING_PERSISTENCE_CASES = ('TestMiningSettlementMongo', 'TestMiningAssetsKeepSourceAndPurity', 'TestMiningAutoRecycleSettlementMongo')
# 打造专项核对原报价和原材料、两个金币钱包及真实结算回执。
FORGE_PERSISTENCE_CASES = ('TestForgeSettlementMongo',)
LUCK_PERSISTENCE_CASES = ('TestLuckDropSettlementMongo',)
WEAPON_OIL_PERSISTENCE_CASES = ('TestWeaponOilSettlementMongo',)
RECYCLE_OIL_PERSISTENCE_CASES = ('TestRecycleOilSettlementMongo',)
WING_PERSISTENCE_CASES = ('TestWingSettlementMongo',)
WING_OIL_PERSISTENCE_CASES = ('TestWingOilSettlementMongo',)
EQUIPMENT_ENHANCEMENT_PERSISTENCE_CASES = ('TestEnhanceEquipmentSlotSettlementMongo',)
EQUIPMENT_REFINE_PERSISTENCE_CASES = ('TestEquipmentRefineSettlementMongo', 'TestRefineRecycleSettlementMongo')
MEDAL_PERSISTENCE_CASES = ('TestMedalSettlementMongo',)
REBIRTH_PERSISTENCE_CASES = ('TestRebirthSettlementMongo',)
BODY_TRAINING_PERSISTENCE_CASES = ('TestBodyTrainingSettlementMongo',)
# 子进程入口由父用例启动；浏览器专项另传真实 Creator 构建并独立生成报告。
EXTERNAL_CASES = {
    'TestServerDirectoryNodeProcess': '由四服联机父用例启动',
    'TestWorldBossTwoClientBrowserMongoWS': '由双客户端真实构建专项执行',
    'TestWorldBossRewardsBrowserMongoWS': '由世界首领奖励真实构建专项执行',
    'TestHostingPurchaseBrowserMongoWS': '由托管补时真实构建专项执行',
}
GO = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test']


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def suites():
    listing = subprocess.run([*GO, '-list', '^Test', './internal/integration'], cwd=ROOT, check=True,
                             capture_output=True, encoding='utf-8').stdout
    discovered = [name for name in listing.splitlines() if re.fullmatch(r'Test\w+', name)]
    if not discovered:
        raise RuntimeError('没有发现联机用例')
    groups = {group: [] for group in GROUP_PREFIXES}
    external = {}
    for name in discovered:
        if name in EXTERNAL_CASES:
            external[name] = EXTERNAL_CASES[name]
            continue
        match = [group for group, prefixes in GROUP_PREFIXES.items() if name[4:].startswith(prefixes)]
        if len(match) != 1:
            raise ValueError(f'联机用例 {name} 需要明确的单个验收分组')
        groups[match[0]].append(name)
    groups['market'].extend(MARKET_PERSISTENCE_CASES)
    groups['payments'].extend(PAYMENT_PERSISTENCE_CASES)
    groups['mining'].extend(MINING_PERSISTENCE_CASES)
    groups['forge'].extend(FORGE_PERSISTENCE_CASES)
    groups['luck'].extend(LUCK_PERSISTENCE_CASES)
    groups['weapon-oil'].extend(WEAPON_OIL_PERSISTENCE_CASES)
    groups['recycle-oil'].extend(RECYCLE_OIL_PERSISTENCE_CASES)
    groups['wing'].extend(WING_PERSISTENCE_CASES)
    groups['wing-oil'].extend(WING_OIL_PERSISTENCE_CASES)
    groups['equipment-enhancement'].extend(EQUIPMENT_ENHANCEMENT_PERSISTENCE_CASES)
    groups['equipment-refine'].extend(EQUIPMENT_REFINE_PERSISTENCE_CASES)
    groups['medal'].extend(MEDAL_PERSISTENCE_CASES)
    groups['rebirth'].extend(REBIRTH_PERSISTENCE_CASES)
    groups['body-training'].extend(BODY_TRAINING_PERSISTENCE_CASES)
    return groups, external


def source_snapshot():
    paths = [ROOT / 'server/go.mod', ROOT / 'server/go.sum', ROOT / 'conf/dev/server.json']
    paths.extend((ROOT / 'config/development').glob('*.json'))
    paths.extend((ROOT / 'server/internal').rglob('*.go'))
    paths.extend((ROOT / 'server/api/generated').rglob('*.go'))
    return {path.relative_to(ROOT).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(paths)}


def run_batch(output, group, cases, timeout, mongo_storage=mongo.DEFAULT_STORAGE):
    identity = uuid.uuid4().hex
    mongo_directory = mongo.run_directory(identity, mongo_storage)
    installation, _ = mongo.installation()
    report = {'batch': group, 'cases': cases, 'selected': len(cases), 'passed': False, 'mongo_cleaned': False}
    destination = output / group
    destination.mkdir()
    print(f'{group} 开始，{len(cases)} 个联机用例', flush=True)
    began = time.monotonic()
    try:
        endpoint = mongo.start(installation, mongo_directory, identity, storage=mongo_storage)
        if not endpoint['endpoint_verified'] or not endpoint['ownership_verified']:
            raise RuntimeError('独立 MongoDB 端点或所有权核验失败')
        report['mongo'] = {key: endpoint[key] for key in ('version', 'port', 'pid', 'replica_set', 'storage', 'log')}
        environment = os.environ.copy()
        environment.update(
            MINIMIR_MONGO_E2E='1', MINIMIR_INTEGRATION_TEST_MONGO_URI=endpoint['uri'],
            MINIMIR_SERVER_DIRECTORY_TEST_MONGO_URI=endpoint['uri'],
        )
        environment.pop('MINIMIR_SERVER_DIRECTORY_WEB_ROOT', None)
        environment.pop('MINIMIR_WORLD_BOSS_BROWSER_WEB_ROOT', None)
        if group == 'payments':
            fixture = destination / 'cash-cards-fixture.json'
            cash_export = [sys.executable, str(ROOT / 'tools/resources/prepare_payment_cash_cards.py'), '--fixture-out', str(fixture)]
            subprocess.run(cash_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*cash_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_CASH_CARD_TEST_FIXTURE'] = str(fixture)
            report['payment_fixture_sha256'] = hashlib.sha256(fixture.read_bytes()).hexdigest()
        if group == 'mining':
            # 只在当前验收批次生成可加载配置，默认试玩与发布仍读取原配置。
            assembly = destination / 'mining-assembly.json'
            fixture = destination / 'mining-fixture.json'
            public = destination / 'mining-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/mining-isolated.json').read_bytes())
            mining_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_mining.py'),
                             '--assembly', str(assembly), '--fixture-out', str(fixture), '--public-out', str(public)]
            subprocess.run(mining_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*mining_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_MINING_TEST_FIXTURE'] = str(fixture)
            report['mining_assembly_sha256'] = hashlib.sha256(assembly.read_bytes()).hexdigest()
            report['mining_fixture_sha256'] = hashlib.sha256(fixture.read_bytes()).hexdigest()
            report['mining_public_sha256'] = hashlib.sha256(public.read_bytes()).hexdigest()
        if group == 'forge':
            # 同一候选串联黑铁矿来源、首领图纸与成品配方，禁止引用试玩数据库。
            assembly = destination / 'forge-assembly.json'
            mining_assembly = destination / 'mining-assembly.json'
            fixture = destination / 'forge-fixture.json'
            public = destination / 'forge-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/forge-isolated.json').read_bytes())
            mining_assembly.write_bytes((ROOT / 'tools/resources/testdata/mining-isolated.json').read_bytes())
            forge_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_forge.py'), '--assembly', str(assembly),
                            '--mining-assembly', str(mining_assembly), '--fixture-out', str(fixture), '--public-out', str(public)]
            subprocess.run(forge_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*forge_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_FORGE_TEST_FIXTURE'] = str(fixture)
            for label, artifact in (('assembly', assembly), ('mining_assembly', mining_assembly), ('fixture', fixture), ('public', public)):
                report[f'forge_{label}_sha256'] = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if group == 'luck':
            # 幸运验收沿同一隔离图纸、矿石和装备来源，抵抗只在本轮候选开启。
            assembly = destination / 'luck-assembly.json'
            mining_assembly = destination / 'mining-assembly.json'
            forge_assembly = destination / 'forge-assembly.json'
            fixture = destination / 'luck-fixture.json'
            public = destination / 'luck-gameplay.json'
            for kind, path in (('combat-luck', assembly), ('mining', mining_assembly), ('forge', forge_assembly)):
                path.write_bytes((ROOT / f'tools/resources/testdata/{kind}-isolated.json').read_bytes())
            luck_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_combat_luck.py'), '--assembly', str(assembly),
                           '--mining-assembly', str(mining_assembly), '--forge-assembly', str(forge_assembly),
                           '--fixture-out', str(fixture), '--public-out', str(public)]
            subprocess.run(luck_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*luck_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_LUCK_TEST_FIXTURE'] = str(fixture)
            for label, artifact in (('assembly', assembly), ('mining_assembly', mining_assembly), ('forge_assembly', forge_assembly),
                                    ('fixture', fixture), ('public', public)):
                report[f'luck_{label}_sha256'] = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if group in ('weapon-oil', 'recycle-oil', 'wing', 'wing-oil', 'equipment-enhancement', 'equipment-refine', 'medal', 'rebirth', 'body-training'):
            # 隔离原武器培养规则及原三项前置配置，便于同源重放全部材料。
            export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_luck_oil.py')]
            for arg, source in (('assembly', 'luck-oil'), ('luck-assembly', 'combat-luck'),
                                ('mining-assembly', 'mining'), ('forge-assembly', 'forge')):
                path = destination / f'{source}-assembly.json'
                path.write_bytes((ROOT / f'tools/resources/testdata/{source}-isolated.json').read_bytes())
                export.extend(['--' + arg, str(path)])
                report['weapon_oil_' + source.replace('-', '_') + '_assembly_sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
            fixture, public = destination / 'luck-oil-fixture.json', destination / 'luck-oil-gameplay.json'
            export.extend(['--fixture-out', str(fixture), '--public-out', str(public)])
            subprocess.run(export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_LUCK_OIL_TEST_FIXTURE'] = str(fixture)
            report['weapon_oil_fixture_sha256'] = hashlib.sha256(fixture.read_bytes()).hexdigest()
            report['weapon_oil_public_sha256'] = hashlib.sha256(public.read_bytes()).hexdigest()
            if group in ('recycle-oil', 'wing', 'wing-oil', 'equipment-enhancement', 'equipment-refine', 'medal', 'rebirth', 'body-training'):
                # 手动分解在同一候选上使用表格投影的准确分母。
                recycle_fixture = destination / 'recycle-oil-fixture.json'
                recycle_public = destination / 'recycle-oil-gameplay.json'
                recycle_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_recycle_oil.py'),
                                  '--oil-fixture', str(fixture), '--oil-public', str(public),
                                  '--fixture-out', str(recycle_fixture), '--public-out', str(recycle_public)]
                subprocess.run(recycle_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
                subprocess.run([*recycle_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
                environment['MINIMIR_RECYCLE_OIL_TEST_FIXTURE'] = str(recycle_fixture)
                report['recycle_oil_fixture_sha256'] = hashlib.sha256(recycle_fixture.read_bytes()).hexdigest()
                report['recycle_oil_public_sha256'] = hashlib.sha256(recycle_public.read_bytes()).hexdigest()
                if group in ('wing', 'wing-oil', 'equipment-enhancement', 'equipment-refine', 'medal', 'rebirth', 'body-training'):
                    # 翅膀沿原世界首领模板真实发奖；唯一工作簿和隔离费用一起固定。
                    wing_assembly = destination / 'wings-assembly.json'
                    wing_assembly.write_bytes((ROOT / 'tools/resources/testdata/wings-isolated.json').read_bytes())
                    wing_fixture, wing_public = destination / 'wings-fixture.json', destination / 'wings-gameplay.json'
                    wing_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_wings.py'),
                                   '--assembly', str(wing_assembly), '--recycle-fixture', str(recycle_fixture),
                                   '--recycle-public', str(recycle_public),
                                   '--fixture-out', str(wing_fixture), '--public-out', str(wing_public)]
                    subprocess.run(wing_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
                    subprocess.run([*wing_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
                    environment['MINIMIR_WING_TEST_FIXTURE'] = str(wing_fixture)
                    report['wing_assembly_sha256'] = hashlib.sha256(wing_assembly.read_bytes()).hexdigest()
                    report['wing_fixture_sha256'] = hashlib.sha256(wing_fixture.read_bytes()).hexdigest()
                    report['wing_public_sha256'] = hashlib.sha256(wing_public.read_bytes()).hexdigest()
                    if group in ('wing-oil', 'equipment-enhancement', 'equipment-refine', 'medal', 'rebirth', 'body-training'):
                        # 复用同一翅膀前态规则，仅在独立候选授权该对象使用原祝福油。
                        wing_oil_fixture = destination / 'wing-oil-fixture.json'
                        wing_oil_public = destination / 'wing-oil-gameplay.json'
                        wing_oil_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_wing_oil.py'),
                                           '--wing-fixture', str(wing_fixture), '--wing-public', str(wing_public),
                                           '--fixture-out', str(wing_oil_fixture), '--public-out', str(wing_oil_public)]
                        subprocess.run(wing_oil_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
                        subprocess.run([*wing_oil_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
                        environment['MINIMIR_WING_OIL_TEST_FIXTURE'] = str(wing_oil_fixture)
                        report['wing_oil_fixture_sha256'] = hashlib.sha256(wing_oil_fixture.read_bytes()).hexdigest()
                        report['wing_oil_public_sha256'] = hashlib.sha256(wing_oil_public.read_bytes()).hexdigest()
        if group in ('equipment-enhancement', 'equipment-refine', 'medal', 'rebirth', 'body-training'):
            # 从本批校验过的翅膀用油前置重建首领、原石和强化等级目录。
            assembly = destination / 'equipment-enhancement-assembly.json'
            enhancement_fixture = destination / 'equipment-enhancement-fixture.json'
            enhancement_public = destination / 'equipment-enhancement-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/equipment-enhancement-isolated.json').read_bytes())
            enhancement_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_equipment_enhancement.py'),
                                  '--assembly', str(assembly), '--base-fixture', str(wing_oil_fixture),
                                  '--base-public', str(wing_oil_public), '--fixture-out', str(enhancement_fixture),
                                  '--public-out', str(enhancement_public)]
            subprocess.run(enhancement_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*enhancement_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_EQUIPMENT_ENHANCEMENT_TEST_FIXTURE'] = str(enhancement_fixture)
            for label, artifact in (('assembly', assembly), ('fixture', enhancement_fixture), ('public', enhancement_public)):
                report[f'equipment_enhancement_{label}_sha256'] = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if group in ('equipment-refine', 'medal', 'rebirth', 'body-training'):
            # 当前批次从同一强化候选和唯一工作簿重建原分解、材料和洗炼参数。
            assembly = destination / 'equipment-refine-assembly.json'
            refine_fixture = destination / 'equipment-refine-fixture.json'
            refine_public = destination / 'equipment-refine-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/equipment-refine-isolated.json').read_bytes())
            refine_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_equipment_refine.py'),
                             '--assembly', str(assembly), '--base-fixture', str(enhancement_fixture),
                             '--base-public', str(enhancement_public), '--fixture-out', str(refine_fixture),
                             '--public-out', str(refine_public)]
            subprocess.run(refine_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*refine_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_EQUIPMENT_REFINE_TEST_FIXTURE'] = str(refine_fixture)
            for label, artifact in (('assembly', assembly), ('fixture', refine_fixture), ('public', refine_public)):
                report[f'equipment_refine_{label}_sha256'] = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if group in ('medal', 'rebirth', 'body-training'):
            # 用本批已验收的前置候选重投角色勋章，并记录世界首领奖励和逐档配置摘要。
            assembly = destination / 'medal-assembly.json'
            medal_fixture = destination / 'medal-fixture.json'
            medal_public = destination / 'medal-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/medal-isolated.json').read_bytes())
            medal_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_medal.py'),
                            '--assembly', str(assembly), '--base-fixture', str(refine_fixture),
                            '--base-public', str(refine_public), '--fixture-out', str(medal_fixture), '--public-out', str(medal_public)]
            subprocess.run(medal_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*medal_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_MEDAL_TEST_FIXTURE'] = str(medal_fixture)
            for label, artifact in (('assembly', assembly), ('fixture', medal_fixture), ('public', medal_public)):
                report[f'medal_{label}_sha256'] = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if group in ('rebirth', 'body-training'):
            # 转生沿用已核对的前置进度，在同一夹具上验证胜利、扣费和重放。
            assembly = destination / 'rebirth-assembly.json'
            rebirth_fixture = destination / 'rebirth-fixture.json'
            rebirth_public = destination / 'rebirth-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/rebirth-isolated.json').read_bytes())
            rebirth_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_rebirth.py'),
                              '--assembly', str(assembly), '--base-fixture', str(medal_fixture),
                              '--base-public', str(medal_public), '--fixture-out', str(rebirth_fixture), '--public-out', str(rebirth_public)]
            subprocess.run(rebirth_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*rebirth_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_REBIRTH_TEST_FIXTURE'] = str(rebirth_fixture)
            for label, artifact in (('assembly', assembly), ('fixture', rebirth_fixture), ('public', rebirth_public)):
                report[f'rebirth_{label}_sha256'] = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if group == 'body-training':
            # 从同一工作簿及前置目录重投炼体概率、费用与属性预算。
            assembly = destination / 'body-training-assembly.json'
            fixture = destination / 'body-training-fixture.json'
            public = destination / 'body-training-gameplay.json'
            assembly.write_bytes((ROOT / 'tools/resources/testdata/body-training-isolated.json').read_bytes())
            export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_body_training.py'), '--assembly', str(assembly),
                      '--base-fixture', str(rebirth_fixture), '--base-public', str(rebirth_public),
                      '--fixture-out', str(fixture), '--public-out', str(public)]
            subprocess.run(export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_BODY_TRAINING_TEST_FIXTURE'] = str(fixture)
            for label, path in (('assembly', assembly), ('fixture', fixture), ('public', public)):
                report[f'body_training_{label}_sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
            # 在已核对的生命和魔法前置上追加伤害、吸收及两组统一封顶。
            hits_assembly = destination / 'body-hits-assembly.json'
            hits_fixture, hits_public = destination / 'body-hits-fixture.json', destination / 'body-hits-gameplay.json'
            hits_assembly.write_bytes((ROOT / 'tools/resources/testdata/body-hits-isolated.json').read_bytes())
            hits_export = [sys.executable, '-B', str(ROOT / 'tools/resources/prepare_body_hits.py'), '--assembly', str(hits_assembly),
                           '--base-fixture', str(fixture), '--base-public', str(public),
                           '--fixture-out', str(hits_fixture), '--public-out', str(hits_public)]
            subprocess.run(hits_export, cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            subprocess.run([*hits_export, '--check'], cwd=ROOT, env=environment, check=True, timeout=60, capture_output=True)
            environment['MINIMIR_BODY_HITS_TEST_FIXTURE'] = str(hits_fixture)
            for label, path in (('assembly', hits_assembly), ('fixture', hits_fixture), ('public', hits_public)):
                report[f'body_hits_{label}_sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
        expression = '^(' + '|'.join(cases) + ')$'
        packages = ['./internal/integration']
        if group in ('market', 'payments', 'mining', 'forge', 'luck', 'weapon-oil', 'recycle-oil', 'wing', 'wing-oil',
                     'equipment-enhancement', 'equipment-refine', 'medal', 'rebirth', 'body-training'):
            packages.append('./internal/persistence')
        command = [*GO, '-json', '-race', '-p', '1', *packages, '-run', expression,
                   '-count=1', f'-timeout={timeout}s']
        report['command'] = command
        log = destination / 'events.jsonl'
        with log.open('wb') as results, (destination / 'stderr.log').open('wb') as errors:
            result = subprocess.run(command, cwd=ROOT, env=environment, stdout=results, stderr=errors,
                                    timeout=timeout + 90)
        report['exit_code'] = result.returncode
        events = [json.loads(line) for line in log.read_text(encoding='utf-8').splitlines() if line.startswith('{')]
        report['passing'] = [e['Test'] for e in events if e.get('Test') and e['Action'] == 'pass']
        report['skipped'] = [e['Test'] for e in events if e.get('Test') and e['Action'] == 'skip']
        report['failing'] = [e.get('Test') or e.get('Package') for e in events if e['Action'] in ('fail', 'build-fail')]
        report['unverified'] = sorted(set(cases) - set(report['passing']))
        report['passed'] = result.returncode == 0 and not report['skipped'] and not report['unverified']
    except (OSError, RuntimeError, subprocess.SubprocessError, ValueError) as error:
        report['error'] = str(error)
    finally:
        if mongo_directory.exists():
            try:
                stopped = mongo.stop(installation, mongo_directory, identity, storage=mongo_storage)
                report['mongo_cleaned'] = not stopped['running'] and stopped['data_removed']
            except (OSError, RuntimeError) as error:
                report['cleanup_error'] = str(error)
        report['passed'] = report['passed'] and report['mongo_cleaned']
        report['seconds'] = round(time.monotonic() - began, 1)
        save(destination / 'result.json', report)
        print(f'{group} 完成，通过 {len(report.get("passing", []))} 项，耗时 {report["seconds"]} 秒，'
              f'验收通过={report["passed"]}', flush=True)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--parallel', type=int, choices=(1, 2), default=2)
    parser.add_argument('--timeout', type=int, default=300, help='每个 Go 测试分组的总时限，单位秒')
    parser.add_argument('--group', action='append', choices=GROUP_PREFIXES, help='只跑指定分组，可重复指定')
    mongo.add_storage_argument(parser)
    args = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1' or not 30 <= args.timeout <= 900:
        parser.error('请通过 tools/dev.ps1 运行，单批总时限须为 30 至 900 秒')
    groups, external = suites()
    selected = [group for group in groups if not args.group or group in args.group]
    output = ROOT / '.work/integration-batches' / uuid.uuid4().hex
    output.mkdir(parents=True)
    print(f'分批联机验收目录 {output}', flush=True)
    before = source_snapshot()
    with ThreadPoolExecutor(max_workers=args.parallel) as workers:
        futures = [workers.submit(run_batch, output, group, groups[group], args.timeout, args.mongo_storage)
                   for group in selected]
        reports = [future.result() for future in futures]
    stable = source_snapshot() == before
    passed = stable and all(report['passed'] for report in reports)
    save(output / 'result.json', {
        'passed': passed, 'all_groups': selected == list(groups), 'source_stable': stable,
        'fixture_sha256': before['config/development/first-loop.json'],
        'source_sha256': before, 'selected': sum(report['selected'] for report in reports),
        'top_level_passes': sum(len(set(report.get('passing', [])) & set(report['cases'])) for report in reports),
        'external_cases': external, 'groups': reports,
    })
    print(f'分批联机验收报告 {output / "result.json"}', flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
