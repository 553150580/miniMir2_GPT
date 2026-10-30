"""仅在实际阶段报告通过后更新七份滚动文档，不提前宣称业务验收。"""
from pathlib import Path
import argparse
import hashlib
import json
import re

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
FORMAL = ROOT / 'docs/双仓储与五角色并发阶段验收.json'
FILES = ['README.md', 'client/README.md', 'config/README.md', 'docs/开发阶段与验收清单.md',
         'docs/正式开发推进记录.md', 'docs/架构适配与规则优先级.md', 'docs/双仓储与跨角色资产实施边界.md']
MARKER = '双仓储与五角色并发已通过rule23限定开发验收：'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    stage = load(FORMAL)
    assert stage['status'] == 'limited_passed' and stage['full_p0_completed'] is False
    assert stage['runtime']['rule_version'] == 23 and stage['runtime']['skills'] == 22
    assert stage['production_ready'] is False and stage['runtime_ready'] is False
    verification = stage['verification']
    assert verification['go_failed'] == verification['go_skipped'] == 0
    assert verification['saved_layouts_unchanged_against_base'] == 9 and verification['saved_layouts_intentionally_changed'] == 1
    for name in ['publication_report', 'normal_gui_report', 'isolated_gui_report']:
        selected = stage['selectors'][name]
        path = ROOT / selected['path']
        assert path.resolve().is_relative_to(WORK.resolve()) and sha(path.read_bytes()) == selected['sha256']
        assert load(path)['passed'] is True
    for name, digest in stage['evidence_sha256'].items():
        assert sha((ROOT / name).read_bytes()) == digest, name
    links = {'README.md': 'docs/双仓储与五角色并发阶段验收.json', 'client/README.md': '../docs/双仓储与五角色并发阶段验收.json',
             'config/README.md': '../docs/双仓储与五角色并发阶段验收.json'}
    base = (MARKER + f"{verification['go_full_mongo_ws_race_passed_including_subtests']}项真实Mongo/WS全包Go race、"
            f"{verification['configuration_python_passed']}项配置Python、{verification['javascript_total_passed']}项JS"
            f"（{verification['javascript_ui_app_passed']} UI/app、{verification['javascript_scene_passed']}场景；"
            f"{verification['client_inputs']}份项目输入，另{verification['client_external_engine_inputs']}份锁定引擎源码和3份字体分别冻结）。"
            f"Creator3.8.8构建及后段SHA核对{verification['creator_seconds']:.2f}秒、exit36，构建期间原主包保持；"
            '9份旧保存布局原字节不变，Game仅追加仓库与入口，原1191对象除两个父children追加外逐字段保持。'
            '个人30格、每3000非绑元宝扩30格，共享按account+永久origin固定50格并校验当前本角色特权与非绑资产；'
            '完整/部分数量、原件版本、仓库CAS、四钱包和typed回执沿同一owner事务，原op优先核对。'
            '只读分页不占永久Pending，旧附着/页/回执不能覆盖当前观察，3504清共享缓存而不取消个人读槽。'
            '正常新角色个人仓及隔离特权/已有资金下共享与扩页分别通过触屏；后者不代表自然供给或实际付费。'
            '真实两角色抢最后格/同物、五角色同时附着、并发第五槽/拒第六槽及同run不同origin分仓通过限定组合。'
            f"当前预览`{stage['runtime']['preview']}`；唯一玩法表仅开发profile两格变化，460公式及架构书保持。"
            '主仍22技能、legacy免费未训宠、护符和17关闭。权益购买、真实支付、运营合服、完整装备成长、持久修理、真机、容量及完整P0仍待完成。')
    prepared = {}
    before = {}
    for name in FILES:
        path = ROOT / name
        raw = path.read_bytes()
        text = raw.decode('utf-8')
        assert MARKER not in text, '拒绝重复插入阶段摘要：' + name
        before[name] = raw
        lines = text.splitlines(keepends=True)
        assert lines and lines[0].startswith('# '), name
        summary = base + '见[阶段验收](' + links.get(name, '双仓储与五角色并发阶段验收.json') + ')。'
        text = lines[0].rstrip('\r\n') + '\n\n' + summary + '\n\n以下记录保留各自历史范围，当前以rule23限定验收为准。\n\n' + ''.join(lines[1:]).lstrip('\r\n')
        if name == 'docs/开发阶段与验收清单.md':
            replacements = {
                'T012': '| T012 | 限定开发组合已验，完整身份故障矩阵待验 | rule23真实并发第五槽/拒第六槽/原op稳定、五角色同时附着及同角色健康新票后其余角色仍可读各自仓；与既有会话围栏回归组合 | 唯一槽位不超5，各角色不互挤，当前操作者与旧授权隔离；跨节点、维护及完整故障矩阵仍按T010/T013/T073继续 | 策划§7.1；双仓储与五角色并发阶段验收.json |',
                'T014': '| T014 | 部分实现 | 已接四钱包、原操作回执/审计、kind4同源书页合并及rule23双仓原子跨容器存取；其它容器/全部堆叠类型/培养绑定链继续实施 | 无负余额或双归属，原件/来源/绑定/版本保持；共享及扩页隔离已有资金不代表自然非绑元宝供给，审计故障及全资产域仍待验 | 策划§4.3、4.7、7.1；架构§6.5 |',
                'T040': '| T040 | 双仓储限定开发已验，完整支付与故障链待验 | rule23个人30格/整页扩容、共享50格物币存取、部分拆分、两角色争同物/最后格、同run不同永久origin隔离及健康/冷重入 | 当前本角色特权与非绑校验；个人不存币；原op及仓库/角色CAS同事务；正常个人与隔离共享/扩页触屏分开，不替代权益购买、真实支付或运营合服 | 策划§4.2、7.1、10.2；双仓储与五角色并发阶段验收.json |'}
            for key, replacement in replacements.items():
                text, count = re.subn(r'^\| ' + key + r' \|.*$', lambda _: replacement, text, flags=re.MULTILINE)
                assert count == 1, key
        if name == 'docs/架构适配与规则优先级.md':
            assert 'rule23双仓储开发中：' in text and '尚未验收或切换主预览' in text
            text = text.replace('rule23双仓储开发中：', 'rule23双仓储通过限定开发验收：').replace('尚未验收或切换主预览', '已受控切换rule23主预览')
        if name == 'docs/双仓储与跨角色资产实施边界.md':
            old = '本阶段开发中，尚未验收、切换主预览或提交。'
            assert old in text
            text = text.replace(old, '本阶段已通过限定开发验收并受控切换主预览，详细范围见《双仓储与五角色并发阶段验收.json》。')
        if name == 'client/README.md':
            old = '`make dev-start`启动同源客户端`http://127.0.0.1:18080`和本地开发服务，`make dev-status`/`make dev-stop`查看和停止。'
            assert text.count(old) == 1
            text = text.replace(old, 'Windows在项目根目录运行`.\\tools\\dev.ps1 task dev-start`，然后用浏览器打开[游戏预览](http://127.0.0.1:18080/)；先注册账号，再选择职业、创建角色并进入。运行`.\\tools\\dev.ps1 task dev-status`查看状态，`.\\tools\\dev.ps1 task dev-stop`停止。macOS使用`make dev-start`、`make dev-status`、`make dev-stop`。')
        prepared[name] = text.encode('utf-8')
    out = WORK / 'documentation-update'
    assert not out.exists(), '拒绝覆盖已有文档更新记录'
    report = {'applied': args.apply, 'formal_sha256': sha(FORMAL.read_bytes()),
              'before': {name: sha(raw) for name, raw in before.items()},
              'after': {name: sha(raw) for name, raw in prepared.items()}}
    if args.apply:
        out.mkdir()
        for name, raw in before.items():
            target = out / 'before' / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(raw)
        for name, raw in prepared.items():
            path = ROOT / name
            assert path.read_bytes() == before[name], '文档被并发修改：' + name
            path.write_bytes(raw)
        (out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'applied': args.apply, 'files': len(prepared)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
