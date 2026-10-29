"""将rule17限定状态登记至当前文档；主17关闭与隔离验收不混写。"""
from pathlib import Path
import json
ROOT=Path(__file__).resolve().parents[2]
s=json.loads((ROOT/'docs/召唤与跨图宠物规则阶段验收.json').read_text(encoding='utf-8'));v=s['verification']
summary=f"召唤与跨图宠物规则已通过rule17限定验收：{v['go_full_mongo_ws_race_passed_including_subtests']}项全包Go race（开启真实Mongo/WS，0跳过、{v['go_inputs']}份输入稳定）、100项配置Python、{v['javascript_total_passed']}项JS、458个PB边界及{v['creator_seconds']}秒Creator实构通过。明确隔离输入后的正常协议覆盖原书学习、扣一Count新生、确认EXP训练、升级、无符Recall、NoPets异图驻留、旧回执及冷热边界；Owned完整基线观察支持异图宠物面板。当前主仍22技能、原初始书与商店、legacy免费未训宠及关闭护符，17只在独立关闭目录；护符包数量/售价仍待用户答复。尸体GUI5项、主道士Town7项及通用UI6项通过，后者保留1条方向警告；不把旧宠GUI写成正常17施放。"
def save(path,text):(ROOT/path).write_text(text,encoding='utf-8',newline='\n')
def prepend(path,text):
 original=(ROOT/path).read_text(encoding='utf-8');heading,rest=original.split('\n\n',1)
 assert '召唤与跨图宠物规则已通过rule17' not in rest[:100]
 save(path,heading+'\n\n'+text+'\n\n以下保留各自历史阶段范围；当前运行状态以上述rule17与最新验收为准。\n\n'+rest)
for path,prefix in [('README.md','docs/'),('docs/开发阶段与验收清单.md',''),('docs/架构适配与规则优先级.md','')]:prepend(path,summary+f'见[阶段验收]({prefix}召唤与跨图宠物规则阶段验收.json)。')
path='docs/架构适配与规则优先级.md';lines=(ROOT/path).read_text(encoding='utf-8').splitlines();i=next(i for i,t in enumerate(lines) if t.startswith('| A39 |'))
lines.insert(i+1,'| A40 | adapt | rule17按冻结RecallName匹配同名活宠，普通Recall不耗符，Fresh沿原owner材料事务/预留/原图500ms动作；观察过期导致协议模式变化明确失败不扣耗，旧成功回执不重排。原Front按合法请求dir，静态Walk不加动态/门；到期Fresh不重查NoPets。新生2000ms只锁走攻，自动Recall与原宠异图迁移保留三时钟；禁宠初始/真跨图一次入图政策，同图不误冻，Frozen恢复仅本宠本帧return。主人Die直接归档居民宠并一次终态；共享ScheduleID拒回绕。Owned活宠元数据全块一致，不渲染异图实体；升级比较只指后续新生。主22/legacy/材料关闭，等级/书/Count隔离样本不等于自然渠道、主pack或17触屏开放。 | 策划4.8/4.20、既有单宠及经济围栏；《职业宠物生命周期与召唤实施边界.md》与召唤跨图规则阶段验收 |')
save(path,'\n'.join(lines)+'\n')
path='README.md';text=(ROOT/path).read_text(encoding='utf-8');a=text.index('## 当前开发状态\n\n');b=text.index('\n首组技能阶段',a);rows=text[a:b].splitlines()
for i,t in enumerate(rows):
 if t.startswith('- 宠物底座：'):rows[i]='- 宠物底座：rule17接入500ms召唤/Recall、NoPets居民和Owned观察，主人死亡与原身体/训练/三时钟保持；明确隔离的原子材料和升级链已过。主仍免费legacy不训练，17及购符关闭。'
 if t.startswith('- 验证：'):rows[i]=f"- 验证：{v['go_full_mongo_ws_race_passed_including_subtests']}项全包Go race（含真实Mongo/WS）、100项配置Python、{v['javascript_total_passed']}项JS、458个PB边界和{v['creator_seconds']}秒Creator实构通过；10布局及6572来源资产保持，玩法82格受控编辑、架构书未变。"
 if t.startswith('- 本机观察：'):rows[i]='- 本机观察：本轮rule17旧宠尸体5项0错误0警告、主道士城镇7项0错误0警告、通用UI6项保留1条方向警告。正常17新生视觉与购书购符/自然养成未验收；隔离WS不作为主包规格或最终1.80认证。'
save(path,text[:a]+'\n'.join(rows)+text[b:])
path='client/README.md';text=(ROOT/path).read_text(encoding='utf-8').replace('当前为rule16、22项','当前为rule17、22项',1);a=text.index('当前尸体与训练观察');b=text.index('\n\n开发前先读',a)
save(path,text[:a]+f'''当前Owned自有活宠与跨图观察通过{v['javascript_total_passed']}项JS（358 UI/app、16场景）、458个PB边界及{v['creator_seconds']}秒Windows Creator实构；10份保存布局保持。Owned随完整基线/取消ACK逐块比对，MapID/Life/HP/训练均Long；异图不造body，保存PetPanel显示驻留/抑制/训练，同图与实际实体交叉核对。17若获配置选择，界面按权威recall_skill_ids分普通无符Recall与原材料RPC，不按名字或模板猜；升级比较明确只影响后续新生。当前主仍22技能/免费legacy不训练/护符关闭，正常17视觉与购符未验收。rule17尸体5项、Town7项0错误0警告，UI6项仅方向警告。见[阶段验收](../docs/召唤与跨图宠物规则阶段验收.json)。'''+text[b:])
path='docs/开发阶段与验收清单.md';lines=(ROOT/path).read_text(encoding='utf-8').splitlines()
for i,t in enumerate(lines):
 if t.startswith('| T030 |'):lines[i]='| T030 | 部分实现，整体待核验 | 主22项与原获取升级保持；rule17接入完整召唤候选、原图500ms/Recall、NoPets、主人死亡/尸体/训练及Owned，含真实隔离WS0→1升级 | 主17/购符仍闭门，Count包规格待用户答复；初始书/19级/Count2及升级输入只是隔离资格，正常17视觉/供给/完整三职业与1.80仍待验 | 策划4.8/4.20；召唤与跨图宠物规则阶段验收.json |'
 if t.startswith('| T035 |'):lines[i]=t.replace('rule16尸体与训练引擎仅限定回归，宠物死亡再召和自然养成仍待验；','rule17源Owner.Die杀宠/原尸体及跨图规则已接，正常死亡再召与自然养成仍待验；')
save(path,'\n'.join(lines)+'\n')
prepend('docs/正式开发推进记录.md','2026-10-29：'+summary+'源审查纠正同图NoPets误冻、初始NoPets漏冻、Frozen恢复多跑AI及共享事件序号回绕，均保留真实红绿。严格JSON拒旧版空召回名，目标阶+1原书公式贯通独立目录/Fixture；预览中文明确后续新生。完整P0与自然养成、经济闭环目标保持，见[阶段验收](召唤与跨图宠物规则阶段验收.json)。')
path='config/README.md';text=(ROOT/path).read_text(encoding='utf-8').replace('## 当前rule16宠物尸体与训练定义','## rule16宠物尸体与训练定义：历史验收',1);heading,rest=text.split('\n\n',1)
save(path,heading+'''\n\n## 当前rule17召唤与地图禁宠候选

预览`dev-pet-summon-v0.17-9cfc98f544bf`，98表965记录（candidate466/confirmed393/historical36/unverified70）、715项正式阻断。主22技能、原书/升级/商店/初始赠书/材料关闭及legacy宠基础保持；玩法82格受控修改、460公式和其余ZIP成员保持，架构书不变。NoPets与NoTeleport独立，两个现图显式false为技术候选，不反推原服实际政策。

独立`config/development/summon-candidates.json`由共同Skill/Book/Upgrade投影导出原书25、17及三阶，runtime_enabled=false；缺任一阶不可绕过完整Fixture.Validate。delay500、Count1及spawn2000复用既有唯一字段，不另建数值源。RecallName为明确跨来源候选BoneFamiliar，大小写敏感、不用展示名或定义ID猜同名；pet-runtime独立目录schema2必需此名，历史schema1仅可省略，新键即使空/null也拒。

升级书成本须为唯一原书的目标阶+1；资格采用Crystal19/22/26，MYF19/23/26差异在来源记录保留。候选17不加入主运行选择，不猜护符包Count、售价或来源DuraMax含义，不追加商店/赠书。隔离WS局部19级/原书/升级材料/两Count证明资格后的业务，不能写成正式供给/自然成长/主pack。详见[阶段验收](../docs/召唤与跨图宠物规则阶段验收.json)。以下为历史范围。

'''+rest)
path='shared/proto/README.md';text=(ROOT/path).read_text(encoding='utf-8');heading,rest=text.split('\n\n',1)
save(path,heading+'''\n\n当前rule17共69个PB数据消息、53个消息号、31个错误码、140个64位字段及458个Long边界案例，无新消息号，仍属draft开发合同。SceneBaselineChunk.owned_pet=10新增OwnedPetState：原活宠handle/Life/definition、实际MapID、HP/上限、训练、recall_skill_ids及follow_suppressed；无落点、预生或尸体，不把异图实体加入entities。

全部分块及退出取消ACK须完全一致并完整组装；同图时交叉核对原实体，64位保持Long。客户端只读召回技能集合选择原CastIntent/材料RPC，服务端重新判断；观察过期则正式拒绝模式变化、不扣符、不伪造经济成功。永久回执仍只记原消耗事实，不能从历史回执排新Recall/新生。主仍22技能/材料关闭，协议能力不授予17或购符，见[阶段验收](../../docs/召唤与跨图宠物规则阶段验收.json)。以下保留历史版本。

'''+rest)
print(json.dumps({'passed':True,'updated_documents':7,'rule_version':17},ensure_ascii=False))
