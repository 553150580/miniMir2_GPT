# Rule15之后的服务端P0闭环审计与职业宠物实施接口

日期：2026-10-29。只读审计；本轮没有修改生产代码、唯一Excel或既有测试，没有启动测试、开发服务或数据库任务。本文给出下一阶段可实施范围，不是新增运行数值源，也不把建议视为已经验收。

## 1. 结论与当前基线

建议下一条实质业务闭环为：**道士正常购入护符并装槽 → 14/15真实消耗与效果 → 购书25并学习稳定技能17召唤骷髅 → 500ms来源落地 → 真实参战、训练、死亡与再次合法召唤 → 健康重入/城镇往返保留同一宠物现场**。这会替换目前按职业直接赠送固定宠物的开发脚手架，形成新的玩家能力与永久资源消耗闭环。

技能17是召唤骷髅、书25、Crystal技能65；技能16是困魔咒。不得将三个ID命名空间混用。护符包单位与报价仍待用户确认，root提出的1000张/500金币在本审计时不是已审定值；生命周期底座可独立推进，商品和正式施法入口须等唯一Excel明确后启用。

审计输入：HEAD `5ae8a140022de45eee00af741ce39bedb9a5fe78`；`config/development/first-loop.json` SHA-256 `0edbe0d5e7bb8f5ac71592469fbced1b37ac896e5c572380f76f53f5631456d6`，rule_version=15。当前有22项技能、4状态、66升级行、24商店行、88书品质分解行、3职业成长、1怪物模板、2件装备。两个World与单一永久owner已分离，21已是本阶段完成范围，不再沿清单历史行误报为未接。

`docs/开发阶段与验收清单.md:3`记录rule15的795条真实Mongo/WS全包race、0跳过及125输入稳定；这是既有阶段报告事实，本次没有重新执行或扩大其证明范围。T030历史行仍写21技能和瞬移待绑定，应结合文首rule15覆盖说明阅读；T032写待执行也不代表6/12小时日领基础从未实现。

## 2. P0目标与真实缺口

依据优先级：最终策划§4.1、§4.2、§4.8、§4.20、§8.1；`docs/架构适配与规则优先级.md`尤其A28/A35/A37；开发清单S02/S03。正式目标完整保留；MYF33项只是本地候选清单，不能将补齐33项等同完整1.80认证。

| 领域 | 当前生产已有能力与入口 | 下一条实质业务缺口 |
| --- | --- | --- |
| 道士职业宠物 | `game/runtime.go:661 initialize`在Job2无条件生成固定Pet；`combat.go:419`真实近战/跟随/受击；`world_context.go:163 activateWorld`保留原指针 | 无召唤技能17/30、无书25/33获取、无耗符召唤/同宠Recall、无训练字段与成长、无死后合法重召。当前宠物只是开发注入，非职业召唤完成 |
| 材料施法 | `game/casting_material.go:16/62/90`，`persistence/casting_supply.go:18/46/77`已有装卸、版本校验、原子扣除、回执、未知提交与一次临时效果；14/15处理器已有 | 当前`casting_supply.enabled=false`、187.max_stack=0、无护符商品；不能直接打开开关而不补单位/价格/包数量与正常购入路径 |
| 三职业技能 | `game/skill_cast.go:26`22行为；`melee.go`近战模式；`skill_assets.go`/Store原书学习、预览升级与合法拆页；21已接 | 本地33项中仍缺11项；完整目标清单及全部伤害、控制、Buff/特殊伤害亦未认证。下表列具体缺项 |
| 装备 | `persistence/assets.go:110`附近装备事务；`combat.go:126 fighter`基础属性投影；当前木剑218、布衣193 | 10部位完整供给、阶段/需求等级、品质词条实例、MC/SC等完整装备属性、持久/修理、部位强化、装备比较与职业外观。不能把已有两个物品换装称T031完成 |
| 回收 | `persistence/assets.go:148`附近手动选择背包装备，绑定输入回收为绑定金币；书拆页有来源限制 | 无自动回收设置、品质/等级/保护条件、锁定与待选预览保护；无统一装备分解多产物表/持久损坏口径。`Settle:384`明示满包放弃该物并PickupStopped，尚非免费自动回收 |
| 自然养成 | CreateRole等级1，正常战斗EXP、三职业成长、日领6/12小时、永久钱包/背包/技能与战报 | 只有20条升级需求，Settle:369循环最多自然到21级；14/15学习22/25、部分技能35级，缺后续经验表。自然1→19、药修成本、职业装备供给、长期挂机净产出、2×G_ref购时及1→50均未验收 |

本地候选缺项（以配置22项与`资料索引/三职业技能来源映射.json`差集实读）：

| 职业 | 尚未接入技能（稳定ID／书ID） | 可复用基础／新增领域 |
| --- | --- | --- |
| 道士 | 施毒6／23；灵魂火符13／24；困魔16／31；骷髅17／25；隐身18／26；集体隐身19／27；心灵启示28／30；神兽30／33 | 符粉实例与消耗、SC法术、毒跳与归属、目标隐身/索敌、控制免疫、显血、召唤训练/形态；不能以一般Status布尔字段替代各自来源规则 |
| 法师 | 诱惑20／9；火墙22／15；圣言32／19 | 诱惑可复用单宠生命周期但还需资格/概率/寿命/叛变；火墙可复用map-owned事件与原World奖励，但需格区持续登记/重叠/容量；圣言需亡灵资格与概率/等级边界 |
| 战士 | 本地6项候选已有处理器 | 这不证明目标版本技能全集/幸运/所有攻击模式及完整装备管线已认证 |

建议17完成后先接13灵魂火符与6施毒，使道士职业输出和耗材形成可玩的组合，再依领域补其余技能；30神兽可复用17底座，但形态/攻击行为及源模板必须单独核，不能仅换高属性。装备自然成长另形成下一闭环：一段经审定等级区间、三职业各有可感知MC/SC/DC装备供给、正常掉落换装、免费自动回收保护与绑定继承，再用实际1/4/8怪推进收支。不要把这些全部并入17阶段导致范围失焦。

## 3. 下一阶段可证明的范围

root已明确本阶段允许隔离前置等级资格：17使用19级、14/15使用25级样本；只在未Enter前对独立新库设置必要等级/余额，不预置技能书、已学技能、护符、绑定或临时宠物。之后从正常购入/原书学习/装槽/施法入口走完整流程。这与此前21的资格前态一致，能证明技能和资产闭环，不能据此宣称从1级自然升到25或完整经济平衡。

建议阶段验收按玩家可观察结果定义：

1. 新道士正常入场没有免费骷髅；未学17不得召唤。正常买书25并消耗原书学习，已学永久保留。
2. 护符商品预览展示包数量、金币优先扣绑/不足扣非绑、实际原实例/槽100；购买、装槽、14/15及17耗符均有确切版本/数量。14/15效果和正常重复、材料不足有真实表现。
3. 17首次召唤只扣一次符/MP，500ms来源事件生效；同名存活宠Recall不耗符且不重建/回血；死后不免费自动复活，再次合法施法才产生新birth。
4. 原宠受击、真实近战击杀、主人获得合格EXP时训练、训练等级/上限/经验独立可观测；主人最后一击也能训练，非法/重复/迟到回调不能训练新宠。
5. 城镇/个人往返、新票健康重入保留宠物对象、birth、HP/MP、训练和状态；ACK前旧合法事件可完成而新AI关；冷故障重建不从回执或已学技能恢复临时宠物。
6. Pending未知、队列拥满、断线、新旧Auth、旧材料回执只对账同一job；永久扣除不重做、临时效果不重造、旧奖励不被新宠领走。容量仍使用owner已有有界事件/事实预算。

限定结论应为“护符与17职业宠物子闭环通过”，不写完整道士、完整三职业/1.80、自然成长、自动回收、正式NPC公共AOI、真机或容量完成。

## 4. 来源中决定生命周期的行为

本节均来自项目只读Crystal副本，不读取来源账号、数据库或部署配置。

| 来源位置 | 行为事实 | 对本作实现的约束 |
| --- | --- | --- |
| `HumanObject.cs:4399–4438` | 骷髅先NoPets；同名存活且有Node的宠物排500ms Recall并return；否则源最多2个怪宠，取Amulet1，PetLevel=rank、Max=4+rank、ActionTime=now+1000，向当前Map排500ms事件并冻结Front | 单宠是产品明确例外；Recall与fresh必须分流，不能固定每次耗1符；不能初始化赠宠替代 |
| `HumanObject.cs:4448–4487` | 神兽同样Recall优先；fresh耗5符，PetLevel=rank、Max=1+rank×2 | 30可复用底座但不是17别名，也不能将Max与当前训练级混为一值 |
| `HumanObject.cs:3424–3452` | 通用Magic已经推进法术时钟/动作并扣MP，才进入具体召唤函数 | 同宠Recall不耗符不等于不耗MP/CD；本作永久扣除先完成再临时生效的校验次序仍依现有A28适配 |
| `Map.cs:916–933` | 到期取预建monster与冻结front；Master.Dead直接return；原Map.ValidPoint(front)则Spawn原Map/front，否则Spawn主人当前Map/当前坐标，最后加入主人的Pets | 500ms是Map-owned事件；不能统一“用主人当前图邻格”，也不能套21尸体继续执行特例 |
| `Map.cs:637–643`、`MonsterObject.cs:689–704` | ValidPoint仅尺寸+cell.Valid；Spawn再次验证、RefreshAll并满HP | 不是`free/neighbor`：未检查动态占格、斜角或DoorOpen；应使用明确来源静态Walk适配，不得复用关闭门BFS判点 |
| `HumanObject.cs:317–321`、`MonsterObject.cs:965–1005` | 死宠从主人Pets移除；尸体作为地图对象另有生命周期 | 死亡不能免费重召；新生不能复用旧birth；本作单宠实体预算下尸体/替换如何显示须明确适配，不引入无界尸体表 |
| `MonsterObject.cs:811–830` | PetLevel加HP/AC/MAC/DC；骷髅/神兽的速度又受MaxPetLevel影响，有400ms底线 | 怪物模板Level、PetLevel、MaxPetLevel是三个概念；不能用静态f.Pet或只把训练级写进Fighter.Level |
| `MonsterObject.cs:982–984`、`PlayerObject.cs:847–899` | 怪物死亡给EXPOwner.WinExp；玩家GainExp后给同主人当前图、DataRange内且存活的宠物PetExp(amount) | 不要求宠为最后一击。资格要捕获当时原宠，不可在异步完成后重新找当前宠 |
| `MonsterObject.cs:3502–3517` | 骷髅/神兽经验×3；阈值(PetLevel+1)×20000；一次调用只升一级，然后RefreshAll | 独立训练表/配置、合法整数与溢出边界；不顺手改成while连升或技能阶=训练级 |
| `MonsterObject.cs:1334–1376,1671–1728` | PetRecall有NoPets门禁；异图或超DataRange的跟随可Recall到主人Back/主人脚点 | 临时不同图是可表达状态；跟随/Recall应有正式居民身份，不应把新图坐标当旧图坐标进行寻路 |

来源Settings中宠物名字和Globals.DataRange等尚需按白名单独立核来源并由唯一Excel维护，本审计不填猜值。Map.NoPets是否/如何投影也需明确；不能从Town没怪或安全区推导允许召唤/伤害免疫。

后续来源补充：root/来源审计者已找到CrystalLatest，四核心及Settings摘要匹配已复制来源，Globals.cs:37的DataRange=16将以supplement契约和独立副本登记。这是可追溯候选来源，仍由唯一Excel投影；本报告不增加代码默认。BoneFamiliar/AI23到MYF381保持显式跨来源candidate。

## 5. 独立于护符包单位的生命周期底座API建议

以下是**待root确认接口**，不是现有API或已授权生产补丁。建议增加`game/pet_lifecycle.go`、`pet_training.go`、`summon17.go`，先将生命周期与配置、资源、永久扣除解耦。

```go
// 唯一实体仍归room，不将Pet拷贝进每个World。
// 推荐room字段：PetLifeSequence uint64；PetWorld worldRef。
// entity增加Pet *petRuntime（或同义字段，不能与room.Pet混淆）。
type petRuntime struct {
    SkillID, RankAtBirth uint32
    PetLevel, MaxPetLevel uint32
    PetExperience uint64
    Base, Current devconfig.Fighter // 模板快照与训练后的无Buff快照
}

func (r *room) petIn(world *worldState) *entity
func (r *room) petReference() actorRef
func (w *worker) nextPetLife(r *room) (uint64, bool)
func (w *worker) installPet(r *room, world *worldState, pet *entity) bool
func (w *worker) removePet(r *room, expected actorRef) bool
func (w *worker) relocatePet(r *room, expected actorRef, to *worldState,
    point devconfig.Point, now time.Time) bool
func (w *worker) refreshPetStats(r *room, pet *entity) bool

// accepted召唤意图不保存永久role；固定归属的到期事件使用已有owner预算。
type pendingSummon struct {
    ID, Order uint64
    Origin worldRef
    Master actorRef
    Front devconfig.Point
    Due time.Time
    Pet *entity // 尚未resident的预建唯一新生对象，或纯不可变birth计划
    Recall actorRef // 同宠Recall路径引用既有birth，不能再造对象
}
func (w *worker) resolveSummon(r *room, event pendingSummon, now time.Time)

// 训练资格只在原获准EXP事实处捕获；confirmed回调使用同一冻结事实。
type petTrainingFact struct {
    Pet actorRef
    KillID string
    Experience int64
    // owner execution身份与当时资格可以一并冻结；去重随原killFact生命周期。
}
func (w *worker) capturePetTraining(r *room, killID string,
    confirmedExperience int64) *petTrainingFact
func (w *worker) awardPetTraining(r *room, fact *petTrainingFact) bool
```

### 5.1 birth、尸体与统计快照

- `PetLifeSequence`属于room生命周期，不从当前`r.Pet.Life+1`计算，也不在`r.Pet=nil`时归零。每次真正新生（含死后重召、获准替换）分配一次；handle保持2。溢出明确拒绝，不能回绕。健康重入/Recall/跨图不分配新birth；冷owner有新ExecutionEpoch，旧ref自然无效。
- 正式初始化应无宠；旧fixture兼容须显式区分历史规则，不可保留当前正式Job2无条件赠宠旁路。已学17只是永久能力，不是冷重建自动召唤许可。
- 同名存活Recall保留HP/MP、训练、状态与birth。死宠不能Recall；新召唤按来源新建初始训练，不继承旧尸体训练/HP。未来17/30替换活宠的策略属于单宠产品例外，需审定，不能默认覆盖/免费升级。
- 模板`Fighter.Level`保留来源怪物等级，用于目标等级/状态计算；训练`PetLevel/MaxPetLevel/PetExperience`独立存。`fighter`和Pet AI的攻击/移动间隔必须从该实体Current快照读取，再叠Statuses；不再回读全局f.Pet。成长刷新不随意回满当前HP，首次Spawn满HP另按源分支执行。
- 移除/替换前取消对应Town导航task、归还workspace/route并推进占位revision。旧命中/治疗/状态/Recall/训练回调都按原World+handle+Life解析，不能只比较handle2。若保留旧尸体画面，应有独立、有界且无战斗权限的表示；本阶段可以明确采用新birth替换单槽旧尸体的显示适配，不能藏第二只可参战宠。

### 5.2 必须表达PetWorld居民身份

**如果承诺完整Map-owned来源落地合同，则应增加PetWorld；仅利用当前旅行等事件排空，不足以删除来源异图分支。** 目前17道士不能施21且普通travel等待events清空，常规入口通常不会在500ms内换图，但这只能说明常规可达性窄，不能使原Map/front与主人当前Map脚点成为同一个概念。

`PetWorld`只引用已有Personal/Town之一，不增加World数量、owner、永久Pending或第二只宠物。到期时：原World存在且front静态合法就原World落地；否则核主人当前World和脚点静态合法；Master同Life且活着才继续。原World不应因inactive被当作非法，原图落地不发送新scene实体/特效。Source.Spawn失败时不得注册一个幽灵resident，也不能退款/改落相邻空格来掩盖失败。

随后源PetRecall/跟随规则可将同一对象转到主人图；先保留源落地结果，再在符合时钟的宠物AI中Recall，不能把两个阶段折叠成直接当前图Spawn。若该阶段尚未实现跨图PetRecall，就不能声称完整17来源生命周期。

所有居民查询需统一，而非给单个resolver补if：

- `world_context.go:125 resolveInWorld`：Player仅当前World；Pet仅PetWorld；Monsters/Loot原World。`resolveCredit:142`可跨World认同一birth信用，不能赋予可受击居民身份。
- `runtime.go:772 baseline`、`skills.go:93 skillTargetInWorld`、`skill_cast.go:58 areaTargetsInWorld/399 autoSkill`、`combat_status.go:198 buffTargets`、`displacement_skills.go:32`：只把resident Pet暴露给对应图的友敌目标和客户端。
- `combat.go:42 free/58 neighbor/419 Pet AI/440 monster target`、`town_navigation.go:194 updateTownOccupancy`：使用该World居民，绝不将另一图Pet坐标阻挡当前图同坐标。Pet若暂在inactive World，不允许调用默认room当前图的move/nearest；应显式World的受控Pet处理/Recall。
- `combat_status.go:105 tickStatuses`与`skills.go:139 tickHealing`：唯一Pet对象每轮最多推进一次；不同居民图不能造成重复tick或把它漏掉。只开放必要旧世界事件/宠物跟随，不因此让所有休眠个人怪每tick复活、破坏既有首baseline原截止语义。
- `world_context.go:163 activateWorld`、`map_travel.go:368 finishMapTravel`、`teleport21.go:152`：当前代码把任何nonnull Pet都随主人迁移、包括尸体，改造时需由业务明确选择保留/随行同一resident；保存的HasPet/PetPosition只是位置提示，不可将旧birth复活。现有健康迁移承诺保持，来源召唤落地不是通用player-transfer副作用。

### 5.3 永久扣符与一次临时召唤

现有`CastingReceipt`只保存永久数量和技能，不保存可恢复战斗现场，方向正确。复用`submitMaterialCast`与原q未知重试，不新建按room长协程，不新增并行SourceSeq。manual使用EconomicSeq，auto仍用SourceSeq；与kill/binding串行，只有admit时冻结最新永久水位。

现`materialCast:16`在`prepareCast`前检查材料，`prepareCast:120`无Summon标志且默认抽MC伤害RNG，`acceptPreparedCast:255`固定入普通pendingCast，三处都要新增来源分支。先由服务端当前状态分类Recall/fresh：Recall不要求有符；fresh才用固定原实例/版本原子扣除。不能为使协议方便而额外耗符。可令Recall走普通无永久材料CastIntent，fresh走CastWithMaterial；若选择统一请求，必须相应定义可选材料身份及回执语义，不能伪造一次材料消耗。

fresh plan冻结原worldRef、Master actorRef、Front、技能/模板/rank与唯一事件身份。仅已确认成功回执安排一次500ms事件，MP/CD/接受帧只一次。临时效果水位或event token不得因新连接重置，也不能因重复receipt返回最新Role而重排事件。DB未知保留原plan/q/job，未确认前不创建可参战宠；回执丢失后新owner只恢复永久扣符，不补临时宠。

owner尚健康但断线并不取消已经受理动作；旧连接只能失去response资格。健康重入仍同一World/PlayerLife，可同步当前Pet，不创建第二份。真正旧owner/Auth/ExecutionEpoch失效不能写新房间。材料提交后Master死亡、到期点不合法等源失败不猜退款。17明确`Master.Dead`禁止落地，不能复用21已接受后尸体继续的特例。

事件应计入`room.eventCount()`，摘取至EventBatch的后缀仍占共享128预算；新生与Recall都不得开启独立无界列表。用原World执行参数，不复制room去跑闭包。当前`advanceEventsUsing:165`可复用排序/后缀放回机制，`progressMapTravel:283`事件排空条件须包含新增召唤事件，不得在等待500ms时被误判为已清空。

### 5.4 训练与已确认角色EXP的结合

按root要求建议采用：**在合法获经验事实发生点冻结原宠资格 → 同一kill的永久EXP事务成功 → 对冻结原birth一次award**。挂在`applyDamageInWorld:215/enqueueKillFactInWorld:330`捕获，`killFact`携带可选训练fact；`flushKillFacts:348`按原owner队首排号；`finishSettlement:524`确认原job成功后，在原finished closure移除队首前award。使用该job已确认的`q.Experience`，不从当前Role.Experience差反推（可能升等归零），也不按原始怪EXP另造与角色奖励不一致的数。

资格是主人获得EXP时原Pet同主人当前World、在经核定DataRange内、存活。**不是最后击杀者等于Pet，也不是宠一定在被杀怪原World。** 原World/事件/权限仍保存在killFact；未来旧图火墙+法师诱惑组合需要按获经验当时主人/宠关系核资格，不能偷加originWorld==PetWorld条件。当前Tao17没有21/地图火，该未来组合不构成本阶段新增范围。

callback仅接受原owner执行身份和原kill成功；找到同一birth才加经验，原宠已被替换/移除就不转送新宠。资格已冻结，不能在回调里再按当前距离判定；同一birth健康迁移仍可认信用，但不得因此在旧图注册幽灵实体。迟到/重复成功回调不重复加，unknown未确认不加，失败不加；不为训练单开SourceSeq或永久pet表。

**需要登记的时序适配**：Crystal在GainExp内同步训练，本作若等待永久事务成功，训练属性晚于同批尚未结算的战斗后缀；现有Pending随后暂停AI可以缩小差异，不能证明同批完全无差异。应明确以“确认RoleEXP后训练”作为本作资产边界适配，并有同批两击/unknown/换宠用例。若root要求逐时序来源等价，则需改为在已确认内存死亡/获经验事实处一次推进临时训练、永久资产仍异步；两种选择不能混写为已同时满足。

## 6. 可安全并行的实施边界

接口确认前均不写生产。建议分两轮，避免多人同时改runtime/combat/World：

| 责任块 | 可独立文件 | 必须串行集成/依赖 |
| --- | --- | --- |
| 来源与配置 | 只读源核验、唯一Excel、devconfig的Pet/Summon定义与严格验证、public投影 | root统一拥有；Map.NoPets/静态ValidPoint、训练距离/速度/阈值、单位和报价明确后再开放，不用代码默认 |
| Pet生命周期与居民核心 | 新`pet_lifecycle.go`、`pet_training.go`及纯测试 | **同一人独占**runtime实体/room/initialize/baseline、world_context、combat、skills、combat_status、Town导航占位和map_travel/teleport21受影响入口；先完成居民helper及签名再接17 |
| 永久材料与获取 | persistence的CastingReceipt/consumeCastingMaterial、shop/quote、strict tests；协议若无需变更可完全复用 | 与game通过不可变prepared plan、成功receipt回调对齐；不在Store创建Pet或同步调用worker |
| 17技能模块 | 新`summon17.go`和新技能测试 | 待核心helper冻结后接prepare/accept/resolveDueEvent/autoSkill；现有skill_cast/timeline入口由核心负责人或root单写，避免文件重叠 |
| 客户端与资产 | 原书/护符槽、17输入、权威Pet训练观察、原动画/同帧生命周期 | 协议Long/life、实际resident baseline与资源身份先锁定；客户端不推测宠物出生/训练或恢复旧尸体 |
| 真实业务验证 | 独立`integration/summon17_test.go`、证据与UI工具 | root冻结Go/fixture后顺序真实Mongo/WS；源码尚变动不跑冻结终验。本次只提出范围，没有启动这些测试 |

首轮生命周期底座可在护符包未确认时实现并用显式模板参数核验：birth分配、PetWorld解析、实体stats、原World500ms落地/死亡拒绝、Recall/迁移、捕获训练fact与成功callback。但它只是完整17所需结构，不应独立宣布召唤业务已完成；必须后续正常购书/购符/耗材/真实参战与重召打通。

## 7. 需要保留的后续P0闭环

- 自然1→19/25及更高等级：经验、掉落、药耗、宠物资源空窗、三职业装备与地图供给、日领和托管同规则，提供真实时长/收支样本；不靠测试库升等冒充自然可达。
- 自动回收：免费角色持久设置、品质/等级/锁定/穿戴/预览保护、实际一次拾取与分解事务、绑定衍生物、满包明确策略；再据此核G_ref，不沿当前两个物品回收价推全经济。
- 完整装备与统一属性：10槽、MC/SC/DC/HP等完整基础属性、需求等级/职业校验、实例品质词条与版本、损坏/修理和强化管线；每段有实际掉落与换装收益，不止加字段。
- 其余11技能与完整版本证据、法师职业宠物、完整状态/控制、双卡购买/到期、仓库和市场等仍在§8.1 P0。17阶段不删这些范围，也不将更多防重测试当作它们的产品实现。
