"""记录Root已经逐张实际查看的仓储截图；本脚本不代替人工看图。"""
from pathlib import Path
import argparse
import datetime
import hashlib
import json

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
parser = argparse.ArgumentParser()
parser.add_argument('--report', type=Path, required=True)
parser.add_argument('--viewed-count', type=int, required=True)
args = parser.parse_args()
path = args.report.resolve()
assert path.is_relative_to(WORK) and path.name == 'report.json'
report = json.loads(path.read_text(encoding='utf-8'))
assert report['passed'] is True and report['errors'] == []
screenshots = report['screenshots']
assert len(screenshots) == len(set(screenshots)) == args.viewed_count
assert report['mode'] in ['isolated', 'normal']
for name in screenshots:
    assert Path(name).name == name and name.endswith('.png')
    assert (path.parent / name).read_bytes().startswith(b'\x89PNG\r\n\x1a\n')
visual = {
    'visual_passed': True,
    'reviewer': 'Root',
    'reviewed_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'report_sha256': sha(path),
    'screenshots': screenshots,
    'screenshot_sha256': {name: sha(path.parent / name) for name in screenshots},
    'actual_view_image_count': args.viewed_count,
    'scope': 'Root已逐张实际view_image检查本轮720×1280 Chrome仓储控件、原列表与确认文字；不是手机真机，也不宣称所有历史界面无问题。',
    'findings': [
        '个人双列表、物品详情、数量、存取与取消确认可读，无控件遮挡；末格提交后列表可见，取出原位确认正确。',
        '健康重入画面角色仍存活，原仓库内容由正常UI重新查询；图像只证明表现，资产和绑定以原WS观察为准。',
    ],
    'known_visual_limitations': [
        '00-town-confirm.png中原MapTravel确认Message单长行左右截断，保留旧源并单列后续修复；不是本轮仓储新增控件。'
    ],
    'physical_mobile_verified': False,
}
if report['mode'] == 'isolated':
    visual['findings'] += [
        '共享第50格确认、书页存2取1及第2页余量可见；金币存5取2、元宝存1.2取0.2文字清楚。',
        '个人扩容明确增加30格并扣3000非绑定元宝；容量变60，末位60确认及健康重入后的共享余量可见。',
    ]
else:
    visual['findings'].append('普通角色共享权限拒绝按本轮真实提示展示，未采用隔离权益或资金资格。')
target = path.parent / 'visual-review.json'
handoff = path.parent / 'handoff.json'
assert not target.exists() and not handoff.exists()
target.write_text(json.dumps(visual, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
handoff.write_text(json.dumps({
    'browser_passed': True,
    'report_sha256': sha(path),
    'visual_review_sha256': sha(target),
    'reviewed_screenshots': args.viewed_count,
    'scope': visual['scope'],
    'known_visual_limitations': visual['known_visual_limitations'],
    'natural_paid_chain_verified': False,
    'physical_mobile_verified': False,
}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'recorded': True, 'mode': report['mode'], 'screenshots': args.viewed_count,
                  'visual_sha256': sha(target), 'handoff_sha256': sha(handoff)}, ensure_ascii=False))
