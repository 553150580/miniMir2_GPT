"""由真实限定验收生成滚动说明；默认只写.work审查计划，--apply才改说明。"""
from pathlib import Path
import argparse, hashlib, json, re

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
FORMAL = ROOT / 'docs/普通满包与背包触屏阶段验收.json'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    stage = json.loads(FORMAL.read_text(encoding='utf-8'))
    assert stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 21
    v, runtime, gui = stage['verification'], stage['runtime'], stage['gui']
    assert gui['passed'] and not gui['errors'] and v['go_failed'] == v['go_skipped'] == 0
    assert v['javascript_total_passed'] == v['javascript_ui_app_passed'] + v['javascript_scene_passed']
    summary = (f"普通满包与背包触屏已通过rule21限定开发验收：{v['go_full_mongo_ws_race_passed_including_subtests']}项真实Mongo/WS全包Go race、"
               f"{v['configuration_python_passed']}项配置Python、{v['javascript_total_passed']}项JS（{v['javascript_ui_app_passed']} UI/app、"
               f"{v['javascript_scene_passed']}场景；{v['client_inputs']}份项目输入与另9份锁定引擎源码、3份字体分别冻结）；"
               f"Creator3.8.8实构{v['creator_seconds']}秒、exit36。普通满包先执行原策略，仍无空间只弃新增物品，"
               "战斗、确认经验/金币与托管实际扣时继续；原SourceSeq、typed回执及审计同事务，未决提交和原安全屏障保持。"
               "独立真实离线满包WS覆盖后台AI击杀、弃物、扣时及健康新票；隔离库存/资金/低HP是明确测试资格，不能当作自然供给或效率。"
               "旧满包停止报告保留历史，真正余额耗尽可更新当前聚合原因，原回执不改。"
               "保存Game.scene仅调整背包View/Content并追加Widget，另9份布局保留，构建期间10份均保持；"
               "点击条目立即更新上下文，列表滚动和下层选择保持。"
               f"正常触屏{len(gui['checks'])}项通过、{len(gui['errors'])}错误/{len(gui['warnings'])}警告。"
               f"当前预览`{runtime['preview']}`；两本唯一Excel仅主profile版本20→21的一格变化，460公式与架构书保持。"
               "主仍22技能、legacy免费未训宠、护符和17关闭；正式分解副产物、挑战待领取队列、成长平衡、真机、容量及完整P0继续待验。")
    targets = {
        'README.md': 'docs/普通满包与背包触屏阶段验收.json',
        'client/README.md': '../docs/普通满包与背包触屏阶段验收.json',
        'config/README.md': '../docs/普通满包与背包触屏阶段验收.json',
        'docs/开发阶段与验收清单.md': '普通满包与背包触屏阶段验收.json',
        'docs/正式开发推进记录.md': '普通满包与背包触屏阶段验收.json',
        'docs/架构适配与规则优先级.md': '普通满包与背包触屏阶段验收.json',
    }
    edits = {}
    for name, link in targets.items():
        path = ROOT / name
        before = path.read_bytes()
        text = before.decode('utf-8')
        assert not text.startswith(text.splitlines()[0] + '\n\n普通满包与背包触屏已通过rule21')
        heading, remainder = text.split('\n', 1)
        after = heading + '\n\n' + summary + f'见[阶段验收]({link})。\n\n以下记录保留各自历史范围，当前以rule21与最新限定验收为准。\n\n' + remainder.lstrip('\r\n')
        if name == 'docs/架构适配与规则优先级.md':
            line = ('| A44 | adapt | 普通满包先按玩家策略回收，仍无空间只弃新增物品；在线与托管继续真实战斗和原SourceSeq收益结算，'
                    '不按容量拒绝Auto或停止扣时。原未决/死亡/ACK/失租屏障保持；旧报告3保留历史，真正余额0可更新当前聚合为1、死亡2优先，'
                    '原typed回执与审计不改。保存背包View避固定按钮，当次选择只刷新上下文。挑战容量及待领取队列另行实现。'
                    ' | 最终策划4.2、4.1及7.4；《普通满包与背包触屏实施边界.md》及阶段验收 |')
            after, count = re.subn(r'^\| A44 \| 实施中，未验收 \|.*$', line, after, flags=re.M)
            assert count == 1
        if name == 'docs/开发阶段与验收清单.md':
            old = 'rule19已接免费自动回收策略、原件锁定、明确应用背包、逐事件在线/托管原子回收及回收/丢弃战报；继续完整经济切片'
            new = 'rule19自动回收与战报后，rule21对齐普通满包只弃新增物品、在线/离线继续原战斗收益与实际扣时；继续完整经济切片'
            assert after.count(old) == 1
            after = after.replace(old, new)
        edits[name] = (before, after.encode('utf-8'))
    boundary = 'docs/普通满包与背包触屏实施边界.md'
    before = (ROOT / boundary).read_bytes()
    text = before.decode('utf-8')
    old = '本阶段正在实施，尚未完成验收。'
    assert text.count(old) == 1
    text = text.replace(old, '本阶段已通过限定开发验收，原始证据见《普通满包与背包触屏阶段验收.json》；完整P0继续推进。')
    edits[boundary] = (before, text.encode('utf-8'))
    plan = {'formal_sha256': sha(FORMAL.read_bytes()), 'changes': {name: {'before_sha256': sha(before), 'after_sha256': sha(after)} for name, (before, after) in edits.items()}, 'applied': args.apply}
    if args.apply:
        for name, (before, _) in edits.items():
            assert (ROOT / name).read_bytes() == before
            backup = WORK / 'docs-before' / name
            backup.parent.mkdir(parents=True, exist_ok=True)
            with backup.open('xb') as stream:
                stream.write(before)
        for name, (_, after) in edits.items():
            (ROOT / name).write_bytes(after)
    (WORK / ('docs-applied.json' if args.apply else 'docs-plan.json')).write_text(json.dumps(plan, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'applied': args.apply, 'documents': len(edits), 'scope': '只更新当前限定范围，不改历史验收或源码'}, ensure_ascii=False))


if __name__ == '__main__':
    main()
