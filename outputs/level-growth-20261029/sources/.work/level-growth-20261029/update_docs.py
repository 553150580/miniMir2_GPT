"""原始验收全部通过后更新当前说明；默认只检查，--apply才写入。"""
from pathlib import Path
import argparse, hashlib, json, re

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
read = lambda p: json.loads((ROOT / p).read_text(encoding='utf-8'))

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    stage = read('docs/自然成长与待用经验阶段验收.json')
    assert stage['status'] == 'limited_passed'
    go = read(stage['go_report'])
    client = read(stage['client_report'])
    build = read(stage['creator_report'])
    gui = read(stage['browser_report'])
    config = read('.work/level-growth-20261029/final-review.json')
    assert go['passed'] and go['mongo_e2e_explicit'] and go['race_detector'] and not go['failed_tests'] and not go['skipped_tests']
    assert client['passed'] and client['stable'] and build['success'] and build['exit_code'] == 36 and gui['passed'] and not gui['errors']
    for group in [go['source_sha256_after'], client['current'], build['asset_hashes'], build['script_hashes']]:
        for name, digest in group.items(): assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest, name
    counts = []
    for name in ['full-ui-app', 'scene']:
        raw = (ROOT / stage['client_report']).parent.joinpath(name + '.txt').read_text(encoding='utf-8')
        matches = re.findall(r'^(?:#|ℹ) pass (\d+)\s*$', raw, re.M)
        assert len(matches) == 1
        counts.append(int(matches[0]))
    tests = len(go['passed_tests'])
    summary = (f'自然成长与待用经验已通过rule20限定开发验收：{tests}项真实Mongo/WS全包Go race、120项配置Python、{sum(counts)}项JS（{counts[0]} UI/app、{counts[1]}场景；{len(client["current"])}份项目输入与另9份锁定引擎源码分别冻结）；'
               f'Creator3.8.8实构{build["seconds"]}秒、exit36，10份保存布局不变。正常新道士原装备、原书、原稻草人奖励自然升至{gui["observations"]["final"]["level"]}级并跨过旧21级断点；独立明确前态另验50级有界池、满池金币物品继续、原Source回执、旧余额无损封存与旧高等级保留。'
               '40019只显示确认后的原奖励/实际接收/入池/拒收，战报和宠物训练使用实际新经验；升级只恢复原场景原出生的存活角色一次，迁移和健康新票不回血或重播。装备提交校验权威需求等级，Role复用保存的可滚动成长详情。kind4沿40011/12/13真实报价手动合并同源书页，保留来源/绑定/品质及总数，完整输入输出写原typed回执与审计；背包复用上下文按钮，10布局不变。'
               f'当前预览`{config["preview"]}`；MYF经验目录仍为开发候选，不代表自然1至50周期或平衡通过；现有满包额外停战/托管仍须按4.2对齐，不据此认证完整容量策略。转生挑战/费用、完整装备内容、护符规格、真机、容量及完整P0仍待完成。')
    targets = ['README.md', 'client/README.md', 'config/README.md', 'shared/proto/README.md', 'docs/开发阶段与验收清单.md', 'docs/正式开发推进记录.md', 'docs/架构适配与规则优先级.md']
    outputs = {}
    for name in targets:
        old = (ROOT / name).read_text(encoding='utf-8')
        assert '自然成长与待用经验已通过rule20限定开发验收' not in old, name
        first, body = old.split('\n', 1)
        link = ('docs/' if name == 'README.md' else '../docs/' if name in ['client/README.md', 'config/README.md'] else '../../docs/' if name == 'shared/proto/README.md' else '') + '自然成长与待用经验阶段验收.json'
        extra = ''
        if name == 'shared/proto/README.md':
            verified = stage['verification']
            extra = (f'当前rule20为{verified["pb_messages"]}个PB消息、{verified["pb_protocol_ids"]}个消息号、31个错误码、'
                     f'{verified["pb_long_fields"]}个64位字段、{verified["pb_boundary_cases"]}个Long边界。Inventory附只读ProgressionState，'
                     '新增40019观察原SourceSeq已提交的经验分账；HostingReport追加原经验、未接收经验与入池经验，原experience改为实际接收量。既有技能资产协议新增kind4同源书页合并语义，无新增字段或消息号，仍由原报价/操作身份提交。\n\n')
        new = first + '\n\n' + extra + summary + f'见[阶段验收]({link})。\n\n以下记录保留各自历史阶段范围，当前以本节rule20与最新验收为准。\n' + body
        if name == 'client/README.md': new = new.replace('当前为rule19、22项开发候选技能', '当前为rule20、22项开发候选技能', 1)
        if name == 'docs/开发阶段与验收清单.md':
            oldrow = '| T062 | 待执行 | 等级与转生挑战凭证/费用/经验池；按首转与后续阶段模拟免费和持卡成长 |'
            newrow = '| T062 | 部分实现 | rule20连续50级候选、显式运营上限、有界待用池、原奖励实际分账及无损旧余额迁移；转生挑战凭证/费用与免费/持卡完整周期仍待实现验收 |'
            assert new.count(oldrow) == 1
            new = new.replace(oldrow, newrow, 1)
            oldattrs = '已接三职业默认成长及DC/MC/SC/AC/MAC、准确敏捷；继续完整统一属性'
            assert new.count(oldattrs) == 1
            new = new.replace(oldattrs, '已接三职业默认成长及DC/MC/SC/AC/MAC、准确敏捷，rule20补真实装备需求等级门禁；继续完整统一属性', 1)
            olditems = '| T014 | 待执行 | 实现四钱包、物品唯一归属/堆叠键、绑定链、操作结果回执和结构化审计outbox |'
            newitems = '| T014 | 部分实现 | 已接四钱包、原操作回执/审计与kind4同源书页完整等价键合并；跨容器完整归属、全部堆叠类型及绑定链仍待继续 |'
            assert new.count(olditems) == 1
            new = new.replace(olditems, newitems, 1)
        if name == 'docs/架构适配与规则优先级.md':
            assert '| A43 |' not in new
            anchor = next(line for line in new.splitlines() if line.startswith('| A42 |'))
            row = '| A43 | adapt | 成长由唯一Excel投影连续目录和显式上限；50后新经验仅入一层待用池、池外拒收且其它奖励继续。旧已确认余额原事务消费/入池/封存，超目录保级不授转生；曲线不符保留待迁移。原Source回执给出实际EXP，训练与战报不计算拒收/迁移；升级补满仅原World/handle/Life存活出生一次。HP/MP仍为临时现场；装备需求等级在资产事务校验。kind4沿原报价/操作合并同源且完整等价的书页，保留来源和绑定；旧拆书输出不追认合并，实际材料/输出留typed回执与审计。 | 最终策划4.2、4.4、4.8、5.1、7.4；架构完整堆叠等价键；《自然成长与待用经验实施边界.md》及阶段验收 |'
            new = new.replace(anchor, anchor + '\n' + row, 1)
            assert '| A44 |' not in new
            pending = '| A44 | 待修 | 最终策划4.2只要求普通满包先自动回收、仍无空间则停止拾取或弃新增掉落；现开发实现在自动开关、结算回调和离线推进中额外停战/托管。该差异未视为已批准产品规则，后续优先对齐；本阶段普通清包GUI和旧满包拒绝测试不作为完整政策通过证明。 | 最终策划4.2、4.1及7.4；《自然成长与待用经验实施边界.md》后续容量策略差异 |'
            new = new.replace(row, row + '\n' + pending, 1)
        outputs[name] = (old, new)
    name = 'docs/自然成长与待用经验实施边界.md'
    old = (ROOT / name).read_text(encoding='utf-8')
    anchor = '日期：2026-10-29；基线8abc50fb。本阶段实施中，尚未验收。'
    assert old.count(anchor) == 1
    outputs[name] = (old, old.replace(anchor, f'实施开始：2026-10-29；验收日期：{stage["date"]}；基线8abc50fb。本阶段已通过限定开发验收，实际证据见《自然成长与待用经验阶段验收.json》；完整P0继续推进。', 1))
    if args.apply:
        for name, (old, new) in outputs.items():
            dest = WORK / 'docs-before' / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            with dest.open('x', encoding='utf-8', newline='\n') as stream: stream.write(old)
            assert (ROOT / name).read_text(encoding='utf-8') == old
            (ROOT / name).write_text(new, encoding='utf-8', newline='\n')
    print(json.dumps(dict(passed=True, applied=args.apply, documents=len(outputs), go_tests=tests, javascript_tests=sum(counts), gui_checks=len(gui['checks'])), ensure_ascii=False))

if __name__ == '__main__': main()
