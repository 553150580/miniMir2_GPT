"""更新当前限定状态，历史段落明确保留各自范围。"""
from pathlib import Path
import json
ROOT=Path(__file__).resolve().parents[2]
stage=json.loads((ROOT/'docs/宠物尸体与训练引擎阶段验收.json').read_text(encoding='utf-8'));v=stage['verification']
summary=f"宠物尸体与训练引擎已通过rule16限定开发验收：{v['go_full_mongo_ws_race_passed_including_subtests']}项全包Go race（开启真实Mongo/WS，0跳过、{v['go_inputs']}份输入稳定）、90项配置Python、{v['javascript_total_passed']}项JS、444个PB边界及{v['creator_seconds']}秒Creator实构通过。原对象尸体按实际死亡绝对期限保留，跨World共享128身体预算满额只拒新生；逐阶训练只在确认永久EXP后作用于原birth，单次最多升一级并保留超额。正常UI隔离尸体5项及主配置道士城镇7项通过；通用UI6项保留1条方向警告。当前22技能不含17，护符关闭，主profile仍为免费legacy开发宠且不训练；正常购符/17/500ms出生/Recall继续实施，护符包单位与售价仍待用户答复。"
def save(path,text):(ROOT/path).write_text(text,encoding='utf-8',newline='\n')
def prepend(path,text):
    original=(ROOT/path).read_text(encoding='utf-8');heading,rest=original.split('\n\n',1)
    assert '宠物尸体与训练引擎已通过rule16' not in rest[:120]
    save(path,heading+'\n\n'+text+'\n\n以下段落保留各自历史阶段范围；当前运行状态以上述rule16和最新验收为准。\n\n'+rest)
for path,prefix in [('README.md','docs/'),('docs/开发阶段与验收清单.md',''),('docs/架构适配与规则优先级.md','')]:
    prepend(path,summary+f'见[本阶段验收]({prefix}宠物尸体与训练引擎阶段验收.json)。')
path='docs/架构适配与规则优先级.md';lines=(ROOT/path).read_text(encoding='utf-8').splitlines();i=next(i for i,s in enumerate(lines) if s.startswith('| A38 |'))
lines.insert(i+1,'| A39 | adapt | rule16把原宠物死亡对象移入所属World尸体容器，清Master后不再友方，绝对TTL不因健康重入/Pending刷新。共享128身体含活体/尸体/预留，满额只拒新生；新birth handle/Life单调，原bootstrap Life0合法。训练规则来自唯一Excel，资格冻结原birth，永久EXP确认后一次应用，单次一级、excess保留、满级忽略且uint64检查；此提交等待区别来源即时GainExp。基线按真实64KiB编码整组拆分，退出等原请求完整分块ACK。主profile仍legacy不训练，17/护符/原图500ms/Recall未开放，跨来源AI/TTL不等于原服认证。 | 策划4.20单活动宠、已登记架构提交屏障；《职业宠物生命周期与召唤实施边界.md》及尸体与训练引擎阶段验收 |')
save(path,'\n'.join(lines)+'\n')
path='README.md';text=(ROOT/path).read_text(encoding='utf-8');marker='## 当前开发状态\n\n';start=text.index(marker);end=text.index('\n首组技能阶段',start);rows=text[start:end].splitlines()
for i,line in enumerate(rows):
    if line.startswith('- 宠物底座：'):rows[i]='- 宠物底座：rule16原对象尸体与原截止、跨World共享身体预算、独立训练状态及完整基线传输已接；主profile保留原免费legacy宠且不训练，17、购符、500ms出生与Recall继续实施。'
    if line.startswith('- 验证：'):rows[i]=f"- 验证：{v['go_full_mongo_ws_race_passed_including_subtests']}条真实Mongo/WS全包race、90项配置Python、{v['javascript_total_passed']}项JS、444个PB边界及{v['creator_seconds']}秒Creator实构通过；0跳过、10份布局和6572份来源资产保持。两本唯一Excel更新82单元格，派生配置可复核。"
    if line.startswith('- 本机观察：'):rows[i]='- 本机观察：隔离HP1/15秒TTL配置正常触屏尸体5项通过；主配置道士城镇/绑定7项0错误0警告，通用UI6项保留1条方向警告。21购学施放9项沿历史证据并由本轮全Go回归，不把局部样本写成自然训练、正常17或180秒墙钟验收。'
text=text[:start]+'\n'.join(rows)+text[end:];save(path,text)
path='client/README.md';text=(ROOT/path).read_text(encoding='utf-8').replace('当前为rule15、22项开发候选技能','当前为rule16、22项开发候选技能',1)
start=text.index('当前宠物驻留与替换表现基础');end=text.index('\n\n开发前先读',start)
text=text[:start]+f'''当前尸体与训练观察通过{v['javascript_total_passed']}项JS（348应用/UI、16场景）、444个PB边界与{v['creator_seconds']}秒Windows Creator实构，10份保存布局保持。训练只展示完整权威基线的Long经验/门槛，允许超额；尸体不混入活宠概览，保存详情随原handle/Life离场更新。基线与退出取消均完整组装分块，旧请求不关闭新附着。隔离HP1/15秒配置通过5项正常触屏尸体流程，主配置道士城镇7项0错误0警告，通用UI6项保留1条方向警告；不等于180秒原参数、128容量或真机。主profile仍免费legacy不训练，17/护符/500ms出生/Recall待接。见[本阶段验收](../docs/宠物尸体与训练引擎阶段验收.json)及[实施边界](../docs/职业宠物生命周期与召唤实施边界.md)。'''+text[end:];save(path,text)
path='docs/开发阶段与验收清单.md';lines=(ROOT/path).read_text(encoding='utf-8').splitlines()
for i,line in enumerate(lines):
    if line.startswith('| T030 |'):lines[i]='| T030 | 部分实现，整体待核验 | 22项开发候选及购书/学习/合法拆页/0–3阶升级，含防御状态、8/27和21。rule16原对象尸体、独立训练引擎及完整分块已接；主profile仍免费legacy不训练，14/15护符与17正常业务关闭 | 本阶段真实WS和隔离GUI只验尸体及引擎边界；完整职业、购符召唤/500ms/Recall/死亡再召、正式渠道、完整Buff和1.80继续待验 | 策划4.8/4.20、5.11；宠物尸体与训练引擎阶段验收.json |'
    if line.startswith('| T035 |') and '宠物尸体与训练引擎' not in line:lines[i]=line.replace('未定义处罚/付费复活不擅自开放；','rule16尸体与训练引擎仅限定回归，宠物死亡再召和自然养成仍待验；未定义处罚/付费复活不擅自开放；',1)
save(path,'\n'.join(lines)+'\n')
prepend('docs/正式开发推进记录.md','2026-10-29：'+summary+'两书82单元格及schema同源生成主fixture与关闭的候选目录；修复首行空表编辑与JSON maximum校验，并保留失败。修复同批奖励达到宠物上限后本应忽略的溢出加法，未放宽单RPC/事务/续租时限。详见[阶段验收](宠物尸体与训练引擎阶段验收.json)。完整P0目标保持。')
path='config/README.md';text=(ROOT/path).read_text(encoding='utf-8');text=text.replace('## 当前rule15瞬息移动与世界执行身份','## rule15瞬息移动与世界执行身份：历史验收',1)
heading,rest=text.split('\n\n',1)
save(path,heading+'''\n\n## 当前rule16宠物尸体与训练定义

当前预览`dev-pet-runtime-v0.16-5f25126dac61`，98表960记录（candidate461/confirmed393/historical36/unverified70）、709项正式阻断，22技能与4状态。两本唯一Excel本轮更新82单元格；PetDefs引用Monster基础属性并登记训练、速度、生命周期，PetGrowthLevels保存0–7级门槛与加成；架构Budget40为跨World共享128身体技术候选。不能用结构通过替代容量认证。

`tools/resources/prepare_pet_runtime.py`同一个编译器生成`config/development/pet-runtime-candidates.json`与主fixture的`pet_runtime`。独立候选文件始终runtime_enabled=false；主profile显式enabled和legacy_untrained_pet，原免费宠不训练。rule16必需完整typed对象，缺失/null/未知字段拒绝，成长表终态required_experience必须显式null为唯一例外；raw目录同样严格读取，无零值补缺。17未在skill_ids，护符Count单位和售价保持未知；不从DuraMax猜张数，不添加初始召唤书或材料商店。JSON schema的maximum现在实际执行，空表首行编辑保留原类型及公式结构，相关反例已验。

尸体TTL180000ms、训练范围/增量/门槛和速度均由唯一表投影，game不散落第二份数值。来源映射与AI23/模板对应仍为跨来源开发候选。主配置与隔离HP1/10秒WS、HP1/15秒GUI分别验收，见[阶段验收](../docs/宠物尸体与训练引擎阶段验收.json)。以下保留历史规则范围。

'''+rest)
path='shared/proto/README.md';text=(ROOT/path).read_text(encoding='utf-8');heading,rest=text.split('\n\n',1)
save(path,heading+'''\n\n当前rule16为68个PB数据消息、53个消息号、31个错误码、136个64位字段及444个Long边界案例，仍为draft开发合同，未新增消息号。`EntityState.pet_training=23`为独立训练等级/上限与uint64经验/门槛，`is_pet_corpse=24`识别无主尸体；模板Level不等于训练等级。主legacy宠不带虚构训练状态；训练EXP可超门槛且单次只升一级，客户端不while重算。

训练和尸体观察均属于同一完整scene/baseline/state_sequence/handle/Life。基线在真实64KiB帧限制内整组预编码分块，单体过大/超过合同则整组拒绝，不删尸体凑包；显式退出等待原request/binding/state完整分块取消确认。64位继续Long/BigInt，不新增经济水位、客户端造宠入口或17技能授权。见[阶段验收](../../docs/宠物尸体与训练引擎阶段验收.json)。以下保留各自历史版本范围。

'''+rest)
print(json.dumps({'passed':True,'updated_documents':7,'rule_version':16},ensure_ascii=False))
