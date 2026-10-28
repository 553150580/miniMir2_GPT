# 下一组P0技能来源与依赖审计

2026-10-29，只读研究。未改生产、唯一Excel、schema或素材；未下载、访问外网、执行来源程序或来源指令。本报告记录候选来源与实现缺口，不是新增运行数值源或实施授权。

建议下一组以 **MYF17《召唤骷髅》/书25的真实闭环** 为核心：正常购书学习、指定护符原实例扣费、500ms召唤、存活宠重复施法的原Recall分支、独立宠物训练、死亡后正常再召唤，以及健康往返/重连保持。护符Count单位一旦经用户确认并落唯一Excel，可同时完成已存在14/15的正常材料入口，再接同类材料路径的13灵魂火符；18隐身是随后明确的一组，不宜与施毒和神兽一起扩大首轮范围。不能仅取消免费宠物初始化或增加一个召唤按钮就称闭环完成。

## 范围、身份与尚未实现清单

- `最终策划案.md:280–296`要求目标1.80三职业全部技能，不能把快捷槽或当前子集当完整P0；`487–495`规定单宠是普通个人实例的产品例外，并撤销“死后无空窗自动重召”“训练等级固定等于技能等级”等旧简化规则。
- rule15 fixture选中22项：战士6、法师11、道士5。道士已有2治愈、4精神力、14幽灵盾、15神圣战甲、29群疗；14/15定义在，但护符实际入口关闭。当前22项是开发候选，不是完整目标版本认证。
- `资料索引/三职业技能来源映射.json`为当前MYF首33项与Crystal候选的独立索引。`docs/技能来源核验与实施清单.md:29–33`明确：历史`资料参考/gxx/`和`config/reference_mapping/skills_mapping.json`仍缺失；本次实读也未找到后者。不能沿策划中的旧路径表述宣称已恢复同一Gxx映射，或以MYF33/Crystal67认定完整1.80清单。

| 当前未选MYF ID | 名称 | 职业 | Crystal候选ID/符号 | 原书候选ID |
| --- | --- | --- | --- | --- |
| 6 | 施毒术 | 道士 | 63 Poisoning | 23 |
| 13 | 灵魂火符 | 道士 | 64 SoulFireBall | 24 |
| 16 | 困魔咒 | 道士 | 73 TrapHexagon | 31 |
| 17 | 召唤骷髅 | 道士 | 65 SummonSkeleton | 25 |
| 18 | 隐身术 | 道士 | 67 Hiding | 26 |
| 19 | 集体隐身术 | 道士 | 68 MassHiding | 27 |
| 20 | 诱惑之光 | 法师 | 33 ElectricShock | 9 |
| 22 | 火墙 | 法师 | 39 FireWall | 15 |
| 28 | 心灵启示 | 道士 | 70 Revelation | 30 |
| 30 | 召唤神兽 | 道士 | 78 SummonShinsu | 33 |
| 32 | 圣言术 | 法师 | 44 TurnUndead | 19 |

**身份特别校准：召唤骷髅是MYF17，不是16；16为困魔咒。** `资料参考/myf/Mud2/DB/ApexM2_Magic.csv:17–18`，Crystal身份65不能直接作为本作技能ID。此11项只是本地首33映射的差集，不能据此宣布“剩余全部P0只差11项”。

## 可追溯参数

下表是Crystal默认候选，实际来源数据库可能覆盖。公共施法动作600ms、全局spell1800ms见`资料参考/crystal/Server/MirObjects/HumanObject.cs:3424–3429`；技能默认冷却1800ms、按rank扣减字段见`Server/MirDatabase/MagicInfo.cs:15–22,219–221`。这些不能与MYF CSV的`Spell/DefSpell/Delay`不明单位字段直接拼成同一公式。

| MYF技能/书 | Crystal学习门槛 | MP候选（rank=0–3） | 消耗与时点 | 主要来源 |
| --- | --- | --- | --- | --- |
| 17/25 骷髅 | 19/22/26 | 12+4×rank | 新建符1；存活同名宠只排Recall+500ms、不扣符；新建进入原Map队列+500ms | `MirEnvir/Envir.cs:997–1012`；`MirObjects/HumanObject.cs:4399–4438` |
| 30/33 神兽 | 35/37/40 | 28+4×rank | 新建符5；同名存活宠只排Recall+500ms；新建原Map队列+500ms | `Envir.cs:1179–1194`；`HumanObject.cs:4448–4487` |
| 18/26 隐身 | 20/23/26 | 1+rank | 符1在受理时消费；角色动作+500ms | `Envir.cs:1013–1015`；`HumanObject.cs:4489–4497,6135–6142` |
| 19/27 群隐 | 21/25/29 | 2+2×rank | 检查符1；原Map队列`500+50×切比雪夫距离`ms；该方法未调用ConsumeItem，不能擅自当成“应扣1张”的已证实行为 | `Envir.cs:1016–1031`；`HumanObject.cs:4500–4510`；`MirEnvir/Map.cs:995–1040` |
| 6/23 施毒 | 14/17/20 | 2+rank | 指定毒粉1；受理后扣，角色动作+500ms；射程9 | `Envir.cs:976–978`；`HumanObject.cs:4365–4377,6043–6075` |
| 13/24 火符 | 18/21/24 | 3+rank | 符1；可攻击目标+CanFly；`500+50×切比雪夫距离`ms；射程9 | `Envir.cs:979–996`；`HumanObject.cs:4379–4397,5914–5929` |

MYF真实记录另存原义：`Magic.csv:7,14,18–20,31`；书身份与原Price见`ApexM2_stditems.csv:25–29,35`。原Price分别施毒1100、火符1800、骷髅2000、隐身2500、群隐3000、神兽20000金币，仅可作为商店开发候选，不认证原服实际成交。MYF骷髅门槛为19/23/26，与Crystal19/22/26有明确冲突，下一份来源合同须选择，不能悄悄混用。

## 17骷髅的完整来源链与真实依赖

以下路径前缀均为`资料参考/crystal/Server/`。

### 新召、重复施法与500ms时点

1. 共同入口先支付临时MP（`MirObjects/HumanObject.cs:3442–3451`），之后才进入召唤方法。因此来源里的NoPets、模板不存在或已满等早退，并不能自动推定退回MP。
2. `HumanObject.cs:4401–4405`只在调用召唤方法时检查当前图NoPets。同名存活宠且Node存在时，只给原宠排500ms Recall并返回（4408–4414）；不消费护符、不重建、不加满HP、不更新原PetLevel/MaxPetLevel，也不借重施将已升级技能的新训练上限套入旧宠。
3. 来源同时最多2个怪宠（4417），本作普通实例明确上限1宠；本阶段无需再向用户询问是否开放多宠。但不同类型占用唯一槽时的拒绝/替换与旧尸体的有界处理，需要写成明确产品适配，不能照搬两宠计数。仅实现骷髅时宜拒绝覆盖任何存活原宠。
4. 无同名活宠时才取已装备护符Count≥1（4419）、查模板（4422）、扣符（4426），新建原Monster对象，`PetLevel=rank`、`MaxPetLevel=4+rank`（4428–4432）。新对象没有设置TameTime，不应套诱惑宠寿命/叛变或自动到期默认。
5. 召唤是原Map动作：`HumanObject.cs:4437–4438`捕获原Map与当时Front；`MirEnvir/Map.cs:856–862`到期检查原player/Info/Node；916–933检查Master.Dead。**到期主人已死则不生成，已付资源没有退款分支，不能套21“同Life尸体仍可成功”的规则。**
6. 到期原Map捕获Front若静态ValidPoint，Spawn在原Map；否则Spawn到主人到期的`player.CurrentMap/player.CurrentLocation`。不是一律跟随主人新图，也不是总落原脚点。`MonsterObject.Spawn:689–704`只查静态ValidPoint，不查动态空格。当前产品的不可达/永久堵路边界仍须单独验收，不能把旅行的“相邻空格”筛选冒称源召唤本义。
7. 创建时`ActionTime=创建时+1000`不是最终可行动截止：`Spawn:698–701`调用`Spawned`，`MonsterObject.cs:734–736`又设为实际落地+2000ms；`Monsters/BoneFamiliar.cs:15–19`调用该基类。不可只读前者就让新宠提前攻击。

### 模板等级、训练等级与成长

- 当前唯一Excel`game_monsters.id=2`与`game_pet_templates.id=1`为开发变异骷髅模板，来源`资料参考/myf/Mud2/DB/ApexM2_Monster.csv:381`：**战斗模板Level15**、HP60、DC10–20、AC/MAC10、移动/攻击1000ms；不能把15当PetLevel。
- 模板现`game_pet_templates.summon_skill_id=null`、follow/lifetime仍null，`game_pet_growth`0行。`base_attributes`与怪物模板还有重复旧值；下一次应明确只以`monster_template_id`关联的同一基础属性为数值源，不让profile、pet表与monster表分别维护同一HP/DC。
- Crystal的`MonsterObject.cs:609–611`分开保存`PetLevel/PetExperience/MaxPetLevel`。初训等级等于技能rank只是新建初值，**不等于以后训练永远等于rank**。
- `MonsterObject.cs:811–830`先RefreshBase，再按PetLevel加HP20、AC/MAC两端各2、DC两端各1；移动与攻击减量却按**MaxPetLevel**分别130/70ms，最终均不低于400ms。按本作当前MYF基础表组合这一Crystal公式仍属跨来源开发候选，不是原MYF服务端的已认证宠物成长。
- `PetExp:3502–3515`：已达Max不继续；骷髅/神兽等指定模板所得经验×3；阈值`(PetLevel+1)×20000`；达到后扣当前阈值、等级+1、RefreshAll。源码是一次`if`，并非循环跨越多级；不得把单次大经验改成while而不说明差异。所查复制源没有名为NeedPetExp的配置字段，不能声称已有完整逐级NeedPetExp表；该阈值公式可在唯一成长表显式落为候选。
- `RefreshAll`不调用SetHP；`PetExp`升级也不SetHP，所以升级增加MaxHP但不免费回满当前HP。新Spawn会SetHP(MaxHP)，这是付费新召与训练升级的不同边界。
- 训练不是“必须宠物最后一击”：`PlayerObject.cs:794–899`先计算主人实际所得经验，再对同图、在DataRange内、存活的宠物调用PetExp(amount)。玩家自己击杀也能训练合格活宠。`MonsterObject.Die:982–984`→主人WinExp→GainExp→PetExp构成来源链；不能仅在`fact.Caster==Pet`时给训练。

### 死亡、尸体、再召与Recall

- `MonsterObject.Die:966–1005`置HP0/Dead，原DeadDelay后清尸；不作为野怪发宠物死亡奖励，随后Master=null并清Poison。`HumanObject.Process:317–320`把死宠从主人Pets列表移除；`MonsterObject.Process:1205–1215`按尸体期限从Map移除并Despawn。原DeadDelay按AI：64为0、81/82为int.Max、252为5秒、其余180秒（584–599）；MYF Race不能直接当Crystal AI决定尸体期限。
- 本地未拿到Crystal实际SkeletonName→MonsterInfo实例/AI配置，故不能认证本项目应使用哪个尸体期限。`MonsterObject.GetMonster:65–67`仅表明AI23为BoneFamiliar，`BoneFamiliar.cs:10–19`给出向左下初方向和Summoned标记；不能把MYF Race100直接映射成AI23。
- 死亡后再次施法应走新召路径，消费新符、重置该新宠训练初值，并产生新的实体Life；不是把原死宠HP改满。尸体视觉与活宠1槽的有界清理必须明确。无论是否复用handle2，都要防旧Life攻击、旧恢复/状态和旧训练回调命中新宠；宠物暂时nil后不能把Life再次从0开始而接受旧引用。
- `MonsterObject.PetRecall:1334–1374`不回满血、不更新训练；先按主人到期当前图NoPets判定是否冻结留原图，否则清Target/PMode，**只有宠与主人不在同图时才Teleport到主人Back，失败退当前脚点**。当前复制源同图Recall不会自动把远宠拉到主人旁边，不能凭常识补另一条“同图传送”规则。
- Back是主人当前朝向的反向1格，Front是当前朝向正向1格，定义在`MapObject.cs:193–200`；不是从宠物相对主人位置计算。`MapObject.Despawn:358–375`广播Remove，移除Envir节点，清本对象ActionList，使下属宠物Die，最后Node=null。`MonsterObject.Despawn:3519–3522`先清SlaveList再走该基类。旧宠尸体Despawn后不得让延迟攻击/Recall继续生效，也不得借新宠复用其动作队列。
- NoPets只在召唤受理时检查，500ms新召没有再次NoPets检查；之后跟随/Recall才读主人当前图NoPets（1338、1681–1698）。本作现只配置NoTeleport，没有NoPets合同；下一版每图须显式维护，而不能从Town名字或NoTeleport推导。

### DataRange补充证据与仍缺失的字段

源训练与Recall距离判断直接引用`Globals.DataRange`（PlayerObject898、MonsterObject1713），`Functions.InRange:71`是双轴绝对差都不大于range的方形范围。初查已有副本没有Globals.cs，历史上级路径亦不存在，故没有猜默认。root随后提供两个Windows静态源码目录，按白名单复核后已补证：**CrystalLatest的`Shared/Globals.cs:37`为DataRange=16**。

`D:/123DownLoad/CrystalLatest`的HumanObject、PlayerObject、MonsterObject、MapObject、Map、Envir、MagicInfo、BuffInfo、BoneFamiliar共9个文件与原项目副本逐字SHA全部相同；Settings SHA也与历史三职业来源索引登记值相同。新Globals以独立字节副本保存到`资料参考/crystal-supplement/Shared/Globals.cs`，st_nlink=1，未覆盖原crystal；Settings仅摘录默认名称到`crystal-supplement/source-slices/Settings.pet-defaults.json`，未复制整个文件或读取运营配置。来源、摘要、前后冻结及字段状态见`资料索引/职业宠物补充来源契约.json`。

Settings默认SkeletonName=BoneFamiliar、ShinsuName=Shinsu（Latest162–163），但可被Game配置覆盖，不能由此认证原服实际模板。`MonsterObject.GetMonster:65–67`的AI23→BoneFamiliar与default DeadDelay180000可以被本作选择为显式候选；这不是把MYF Race100等同于AI23。下一合同仍须明确本作采用的模板/行为组合、尸体期限与数量边界，不读取来源账号、PetSave或部署配置来填值。

CrystalLocal是不同版本，不能混入同一快照：虽然PetExp/DeadDelay/RefreshAll/Spawn片段相同，其召唤方法缺NoPets检查、PetRecall始终尝试传送（包括同图），与Latest/现有副本的“NoPets且仅异图”存在实质差异。比较与差异片段单独存于`.work/next-p0-20261029/windows-source-comparison.json`及`source-slices/windows-local-related-methods.diff.txt`。本报告继续以已匹配的Latest/项目副本链为来源，不混用Local较短Recall逻辑。

### 本作World与永久提交接口的必要改造

- 当前`server/internal/game/runtime.go:683–684`按道士职业无条件注入原固定宠；`combat.go:419–446`用一个`r.Pet`和固定fixture.Pet统计/速度；`fighter:155–156`也总取同一模板。还没有召唤资格、训练字段、模板动态派生或正式死亡再召闭环。不能保留免费注入再称技能可选。
- `world_context.go`现仅保留一个当前居民Pet；原Map延迟新召可能先落旧World、再按Recall转到主人新World，需明确适配。已有两个固定World、执行身份与观察scene分离可复用，但不能把所有召唤动作都偷换为21的角色到期语义。
- 护符扣费复用现有`persistence/casting_supply.go`与`game/casting_material.go`的原实例/版本、永久receipt、未知结果原op重试。新召仅在收到确定材料结果后受理一次临时动作；不得重复回调生成第二宠，也不从永久回执冷恢复临时宠物。
- 成长是临时世界事实，不能借此保存宠物HP、当前位置、PetLevel/经验为故障恢复快照。健康切图/重连保留，冷重建按现有故障边界重新召唤。Crystal`PlayerObject.cs:1231–1247`保存/恢复宠物的分支不是本作授权，不应移植。
- 主人永久经验经唯一owner Settle异步提交；训练不能在unknown采用out Role，也不能在晚回调重新查当前`r.Pet`给新宠补旧经验。建议在击杀/获经验事实时冻结合格宠ref（World/handle/Life或独立代次）和数额，由唯一事实队列在确定提交后只推进一次临时训练。SourceSeq不另开宠物并行游标，不推进EconomicSeq；隔离/旧Auth/旧Life回调不得改新宠。来源实际GainExp按当时主宠关系筛选，未来跨图地图法术或换宠组合须独立验，不能简单把成长强绑“原killWorld里的宠”或“最后击杀者”。

## 隐身与施毒：不能只加图标/数值

### 隐身18与群隐19

- 自隐持续`SC采样+(rank+1)×5`秒（HumanObject4496、6139）；群隐`floor(SC采样/2)+(rank+1)×2`秒，固定3×3方形，边界裁剪，只处理cell.Valid且友方Player/Monster（Map.cs997–1038）。群隐不是14/15的7×7，也不按特效尺寸定命中。
- `BuffInfo.cs:20`为Hiding ResetDuration/Properties.None；`MapObject.cs:662–665,737–760`设置Hidden并解除不具备CoolEye或等级不足怪物对该目标的已有锁定。`MonsterObject.cs:674`按模板CoolEye概率采一次，后续索敌在1943–1944等处检查隐藏与等级；当前本作近邻索敌没有这些输入。
- Walk/Run移除Hiding见HumanObject2460–2462、2540–2544。攻击、施法、受击相关分支2869–2885、3418–3422、7138–7141只移MoonLight/DarkBody，不能臆造“隐身受到任何攻击就解除”或“所有攻击技能都解除Hiding”。需原动作对拍确定本作门禁。
- 群隐受理方法只检查护符但没有ConsumeItem（4503–4510），与自隐/14/15不同。应先核来源是否缺陷并选明确开发适配，再决定扣费，不能通过通用Status handler默默扣1。
- 玩家、宠物已有可接收状态框架，但Hidden、怪物感知/已锁定目标、远近过滤、移动打断、死亡及重连保留和状态显示都要共同实现。当前状态计时器不会自动产生这些AI语义。

### 施毒6

- GetPoison在装备中选择Amulet的Shape1/2原实例（HumanObject6870–6889），不是当前仅支持187的Shape0护符槽。MYF灰粉181/183/185为Shape1、黄粉182/184/186为Shape2，原数据`Stditems.csv:183–188`；DuraMax1000/2000/3000不能直接当Count张数。
- HumanObject4365–4377先验可攻击目标，取毒粉1，SC经magic.GetDamage形成value，扣粉后500ms；到期6048再次验target同图、Node及可攻击。Shape1为绿毒，Shape2为红毒；不是按中文颜色猜最终结算类型。
- 两者Duration=`2×value+7×(rank+1)`，TickSpeed2000；绿毒每跳Value=`floor(value/15)+rank+1+Next(PoisonAttack)`（6052–6069）。**Duration在Monster.ProcessPoison中是tick计数，不是可直接乘1000的秒数**；Time先加1、达Duration先移除再伤害，初TickTime默认0，首次tick边界需对拍，不能写成固定总时长默认。
- `MonsterObject.cs:1453–1557`绿毒参与伤害信用/归属、打断自然恢复，红毒改变ArmourRate为0.5；不是统一“红毒最终伤害+20%”。HumanObject的红毒分支与怪物不同，首轮仅打合法野怪也不能把人类路径套入。
- 同类型绿毒弱Value不能覆盖强Value；非绿毒较短Duration不能覆盖较长剩余，见2782–2813。owner.Node失效移除、死亡清Poison、怪物PoisonDamage可被专有AI改写。未来跨World已付费绿毒须持续原目标/Life并进入现有唯一owner kill/奖励队列，不能仅作为一个增加属性的Buff。
- 抗性/恢复、PoisonAttack、毒粉切换策略、槽与数量单位均未配置；Human.ApplyPoison7380–7418还依赖PoisonResistWeight/PvpCanResistPoison等当前未复制Settings值。P0不开放自由PK，不能借施毒加入PK/褐名规则。

## 素材复用与缺口

客户端owner本轮只读核对，未构建/导出新素材：

- 当前manifest仅有`pet_skeleton`完整standing/walking/attack/struck/die，来自Mon4 raw2520起；没有6/13/17/18/19/30对应新增effect条目。现有原像素可复用不等于新技能表现已验收。
- `build_development_assets.py:23–28`现MONSTER_ACTIONS没有appear。Mon4 raw2860–2869（2520+340）本机帧头为10个非空大帧，只能作为出场候选，尚无视觉/动作映射证明；2870–2879是4×1占位。Magic1510–1519也全部4×1占位，不能取“召唤cast后顺延十帧”冒充出场地效。
- Crystal客户端`资料参考/crystal/Client/MirObjects/PlayerObject.cs:1681–1683`施毒起手Magic600–609；1724–1726骷髅Magic1500–1509；1756–1758隐身Magic1520–1529；1864–1866神兽Magic2:0–9。本机primary_180现解码器读取这些帧头均10/10非空，当前manifest均未导入；未作逐帧视觉/动作时点验收。
- 群隐来源PlayerObject3009–3017：16向Magic1160+方向×10的3帧弹道，14/15已存在48帧可复用；落点Magic1540–1549/800ms，本机10/10非空但未入manifest。
- 神兽MYF表386/387为Appr170/171；经典分包推断候选Mon18，但实际Mon18.wil为旧`\x19WEMADE`签名，当前inspect_wil.probe明确UnsupportedFormat，fallback WZL/WZX亦未支持。**包存在不能写成神兽已可导出**；双形态、appear/hide和火焰攻击仍需独立来源映射与解码。
- 来源新召与健康重新观察有不同标记：`BoneFamiliar.cs:15–25`先base.Spawned再置Summoned=true；`MapObject.Spawned:339–354`在353首次BroadcastInfo，因而首次Extra=false，后续GetInfo Extra=true；Crystal Client MonsterObject222–232仅在!Extra时排Appear。不能在每次重连/切图重建Actor时无条件重播新召出场。Crystal独立.lib的BoneFamiliar枚举78不是MYF Mon4 raw2520或appear偏移的证明，所需动作应有单独来源合同。
- 同一handle更换宠物模板的客户端缺口：`GameWorldView.ts:186–215`只在首次handle选择profile，Life变化只沿原profile重建animation；仅添加神兽pet_profiles会继续画旧骷髅。新模板/新Life切换应释放并重建正确profile，旧异步回调不得回填新宠。
- 固定PetPanel目前只有Help占位（AppRoot.ts:287），不能声称已有宠物管理闭环；但本阶段可沿保存技能面板提供正常召唤入口，不必先发明宠物养成UI。任何新固定UI仍需保存scene/prefab，不能由脚本生成布局。

## 需明确的决定与实施验收建议

1. **真实用户待决**：护符包Count单位与报价。root已向用户提出“187每份1000张/500金币，明确本作Count适配开发候选”或继续关闭；截至本报告采集时实际fixture仍enabled=false/max_stack0。不能把MYF DuraMax1000直接解释成该决定已经作出。用户答复后先落唯一Excel与类型合同，再开放材料购买/装备/消费。
2. **来源证据不足**：完整目标1.80技能清单、缺失旧Gxx映射；SkeletonName/ShinsuName到实际模板与AI的配置，具体尸体期限、必要Settings值；MYF与Crystal学习等级/耗材单位差异。可以由本任务显式选择开发候选推进，必须逐项记录，不能写用户审定了尚未讨论的数值。
3. **产品适配需审定**：两宠源改一宠时不同类型替换/拒绝与尸体槽；受理召唤后主人跨World的准确落地/回归策略、NoPets地图规则；群隐未消费护符的源码分支是否采纳。已有一宠上限本身已经明确，无需重问是否多宠；不得以缺口为由默认免费重召或转移死亡宠训练。
4. **推荐验收序列**：正常注册道士→自然或明确隔离到19级→购原书25并学习17→正常购买并装备原护符实例→不足/旧版本/旧Auth拒绝→恰扣1张与MP→500ms一次生成→八方向/前格阻挡/动态占格/新图NoPets时点→主人与宠物分别击杀，训练数值同源且仅一次、升级不回血→活宠重施仅原Recall→宠物死亡旧Life失效→新符再召、训练回新初值→健康重连/两World保留→提交响应丢弃后原op重试不重扣不重召→停服冷建不恢复临时宠物。真实UnknownTransaction故障注入仍须另标，不能把丢弃响应重试冒称网络故障已测。
5. **配置落点建议**：只在唯一Excel现有`game_pet_templates/game_pet_growth/game_skills/game_effects/game_items/game_shop_prices/game_skill_upgrades`与开发profile白名单维护。新增typed summon参数、训练经验/增长规则及NoPets政策；profile只选择ID与实验预算，不内嵌宠物属性或技能数值副本。生命/训练/动作容器属于健康内存，不写角色当前位置/HP/临时World。

本轮只读材料足以设计骷髅召唤候选与明确的缺口清单，尚不足以认证神兽完整资源/AI或三职业完整1.80。下一阶段可验收目标是召唤耗材、宠物训练、死亡再召及健康内存生命周期的完整闭环。

## 本次读取输入摘要

- config/development/first-loop.json：0edbe0d5e7bb8f5ac71592469fbced1b37ac896e5c572380f76f53f5631456d6
- outputs/development-prep-20261026/玩法配置.xlsx：157788372fe9db2a8dc800f23ddea90295e8f80acca5a22ce5863e852da7ebfb
- 资料索引/三职业技能来源映射.json：1d571396ea627a0c491c25e8904d3afb1ff44f6bdad29ebb57db5acf182ca6c3
- 资料参考/myf/Mud2/DB/ApexM2_Magic.csv：356a53a109a9a222900b9e910d1bc304fbbcf54b4679731c17d5ae86c48d9030
- 资料参考/myf/Mud2/DB/ApexM2_stditems.csv：0b9674a23753fd19b614b9dc22d100d0c4c799f77a5a2e22dc5a17e424f283a7
- 资料参考/myf/Mud2/DB/ApexM2_Monster.csv：d97075ce90dd0a86cf75b9d1d16959c1b3478b2503e20624d880ff8ccaa05a39
- 资料参考/crystal/Server/MirObjects/HumanObject.cs：da406ed3f9e2cbc79d9759871410cfaf08b9b17a1e507631e68f2679c105958c
- 资料参考/crystal/Server/MirObjects/MonsterObject.cs：2cc4e673c589ad57452b4682c3bcb155b4b6b3027b1d3ee52a18a5ae1b7df98e
- 资料参考/crystal/Server/MirEnvir/Map.cs：4df569012db86428b82b3c4994a1b0e982b1da7b5ed0b58dcf1930020ed4d5d1
