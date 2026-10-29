# 完整17所需尸体与训练生命周期：服务端实施接口设计

日期2026-10-29。只读设计；未修改生产、配置、协议或测试，未运行旧全包/专项。承接已冻结的PetWorld/birth底座。护符单位、购符、17开放仍待后续；本报告不把生命周期结构当作完整召唤业务。

**root已确认的本轮合同**：rule16真实启用所有宠物死亡→原World尸体→绝对TTL清理；保留免费bootstrap并显式`legacy_untrained`，仍用原Fixture.Pet属性且不训练。新正规factory可安装有独立训练定义的宠物供未来17与隔离测试。共享预算字段正式名`retained_pet_body_capacity_per_owner=128`，包含活动宠、尸体和pending新生预约；不新增1800ms birth guard，不将101视为通用数量上界。服务端实施按实际wire字节拆块，客户端同步移除leave单块假设。以下建议若与这段不同，以本段及root随后准确typed接口为准；收到可编译类型后才开始game实现。

## 1. 选定结构

保留唯一owner、Player和活动Pet。**宠物死亡时将原`*entity`转交原World尸体容器，不复制entity、不改写原handle/Life/World，不把尸体伪装成新宠或掉落。** `room.Pet`只指活动宠物；新出生分配动态handle和单调Life。原bootstrap handle2保持兼容，下一只宠物可使用3，不再要求所有birth复用2。

建议新字段与类型（待root确认命名）：

```go
type petCorpse struct {
    Body *entity             // 正是死亡的原对象，包含原PetRuntime/状态/动作
    ExpiresAt time.Time      // 实际死亡处理时刻+该模板显式TTL，绝不改成now重算
}
// worldState: PetCorpses []*petCorpse
// room: PetHandleSequence uint32；PendingPetBirth *petBirthReservation
// room保留既有PetLifeSequence、PetWorld；两World共享同一尸体/出生预算。

type petRuntime struct {
    SummonSkillID, RankAtBirth uint32
    TrainingLevel, MaxTrainingLevel uint32
    Experience uint64
    Base, Current devconfig.Fighter
    // 只读PetDefinition/Growth快照或指针；由root typed配置提供，无代码数值默认。
}
// entity: PetRuntime *petRuntime
```

模板`Fighter.Level`仍是怪物模板等级，不是训练等级。尸体保留同一个PetRuntime；原属性与状态观察不会因新宠出生被覆盖。Current是训练后的无Buff统计快照，`fighter`再叠该对象现有Statuses。HP/MP不因训练刷新回满。

## 2. 身份与出生接口

```go
func (r *room) nextPetHandle() (uint32, bool)
func (r *room) petBodies(world *worldState) []*entity
func (r *room) findPetBirth(ref actorRef) *entity
func (w *worker) canBeginPetBirth(r *room, now time.Time) bool
func (w *worker) reservePetBirth(r *room, token petBirthToken, now time.Time) bool
func (w *worker) cancelPetBirth(r *room, token petBirthToken) bool
func (w *worker) installPet(r *room, world *worldState, draft *entity) bool
func (w *worker) archivePetDeath(r *room, world *worldState,
    expected actorRef, now time.Time) bool
func (w *worker) expirePetCorpses(r *room, now time.Time) bool
```

- `nextPetHandle`只向前推进，不回收旧handle。跳过0、Player1、敌方10..17、现有Loot1000；同时检查两World实际登记的Monsters/Loot/Corpses/活动Pet，避免测试或后续内容的其他固定handle冲突。uint32耗尽明确拒绝，不回绕。新owner可重置，由ExecutionEpoch/scene隔离旧引用；健康重入/换图不重置。
- 当前`installPet`固定handle2门禁改为owner分配新handle。未出生draft由服务器构造，不接受客户端指定handle。可将draft.Handle=0作为正式输入；当前测试中2只是未安装draft的兼容写法，不得形成正式“强制复用2”的旁路。成功安装同时分配handle/Life；失败不部分改写实体、计数或名额。
- bootstrap2是唯一明确保留的已有身份；尸体2仍是2，新宠是3或更大。旧攻击/状态/目标始终用World+handle+Life，不能命中新宠。尸体在TTL内仍可被原ref解析，但HP0不再次受伤/阻挡/复活。
- `findPetBirth`给训练/信用使用：按同owner执行身份、handle、Life找到原活动Pet或未到期尸体，可跟随同一birth健康换图。捕获的World是原资格证据，不强迫当前居民World仍相同。攻击目标解析仍要求实际居民World，不能用信用查询授予异图受击资格。
- 单调Life和handle各自检查溢出，不用handle代替Life，也不把原尸体身份重写成临时显示ID。原对象转交完成前不能清slot；失败不能丢原尸体。

## 3. 容量、TTL与死亡原子性

容量是**一个owner两个World合计128个retained bodies/预约**，不是每World各128。字段名`retained_pet_body_capacity_per_owner`；TTL按root显式typed字段读取。骷髅候选180000ms来自Crystal默认DeadDelay分支，不泛化所有AI。

仅“尸体满时拒新召唤”还不够：已经存活的宠物总可能死亡，死亡不能被容量拒绝。建议维护以下准入不变量：

```text
尸体数量 + 活动Pet(0/1) + 尚未生效的新生预约(0/1) <= CorpseCapacityPerOwner
```

活动Pet预留自己的未来尸体名额；转尸体时只是名额转换。128尸体时拒新生，不删除最老尸体，不拒绝合法死亡。存在Pending新生预约时，不能在其500ms到期前受理另一fresh计划；不能仅依赖1800ms公共CD，因为Pending可能延迟旧到期事件处理。

TTL180000/minSpell1800的101具估计可说明128候选的动机，**不能替代准入不变量或容量验收**。异步暂停、实际出生/死亡时点、其他未来宠物模板可能改变数量。满额应在fresh永久扣符前拒绝，不能先扣符再发现无名额。

死亡集成点：`combat.go:221 applyDamageInWorld`中HP由正变0后，先执行既有死亡状态规则，再对当前Pet调用archive。archive按原world/ref校验，在同一owner回合将原指针加入`world.PetCorpses`、固定ExpiresAt、归还Pet导航任务、清`room.Pet/PetWorld`并保留birth/handle水位；之后致死快照能同时显示新旧事实。没有clone、重命名或额外资产流水。

旧rule15 fixture通过nil/disabled保持旧模型；rule16真实开启运行规则，原免费开发宠也在死亡后转World尸体，尸体留原图、不随主人旅行。bootstrap显式legacy_untrained：仍用原Fixture.Pet统计，但不获得训练。新Def factory才具备独立成长输入。这是本轮显式行为变化，不能把原手造HP0宠物的旧兼容当作来源尸体业务。

`expirePetCorpses`只移除实际到期原对象及其源对象拥有的未执行动作，不清整个World事件队列。旧目标事件可以到期解析失败，不能转向新宠。释放registry后原ref不可解析。两个World都执行有界尸体清理，不唤醒旧图怪物AI、不刷新原怪物/掉落截止。Pending沿A28暂停处理但不延长ExpiresAt，恢复后按actualNow一次清理；不承诺TTL墙钟硬实时。

Source证据：`MonsterObject.cs:966–1005`在Die时HP0/Dead、DeadTime、广播，之后Master=null、清Poison；不会清Buffs/ActionList。`HumanObject.cs:317–320`从主人Pets移死宠。`MonsterObject.cs:1205–1215`到期移地图、清Master、Despawn；`MapObject.cs:358–375`广播Remove、清ActionList/Node。训练/状态必须仍指原对象，不能因room.Pet清空丢掉它。

## 4. 居民查询与战斗目标

`worldPet`继续只返回活动宠。新增petBodies/枚举器将活动宠和World尸体纳入baseline与原ref解析，确保同一对象恰出现一次；原图尸体不出现在新图baseline。

- `resolveInWorld`可解析原尸体；旧攻击对其HP0终止。若原已释放动作的caster仍需存在，可找到原对象，不会错误使用新宠。到期退役再令该ref失效。
- `baseline`包含当前World尸体及其原kind2、handle、Life、位置、死亡动作、原definition和适用状态。无需新增“尸体kind”或改原handle；客户端按独立handle已可显示多个同profile尸体。
- 普通free/neighbor/Town动态占位只看活阻挡者；尸体不占活动Pet导航槽、不参与AI/索敌，不奖励EXP或掉落。
- `cellObjects`按原入格顺序保留尸体以维持源扫描列表，但HP0不能成为Blocking/新伤害目标。
- World-owned尸体不再具有owner友方资格：`MonsterObject.Die:1000 Master=null`，`IsFriendlyTarget:2541–2544`无Master直接false。因此14/15的AllowDeadTargets不自动包含旧宠尸体；玩家尸体原允许分支保持。治疗/自动Buff和宠物状态概览默认只看活动Pet，不能把全部kind2当作当前宠。
- 尸体未到期的Buff按原对象推进，恢复HP队列仍死亡暂停；新宠不继承尸体Buff/治疗/动作/训练。尸体和活动Pet必须只tick一次。

## 5. 确认killEXP后的训练接口

```go
type petTrainingCredit struct {
    Pet actorRef
    KillID string
    Qualified bool
    // 资格在原获经验事实时冻结；原World/当时位置关系保留在事实中。
    SourceSeq int64        // 队首正式admit时绑定同一q
    Applied bool
}
func (w *worker) capturePetTraining(r *room, killID string) *petTrainingCredit
func (w *worker) preparePetTraining(credit *petTrainingCredit,
    q persistence.Settlement) bool
func (w *worker) awardPetTraining(r *room, credit *petTrainingCredit,
    confirmedExperience int64) bool
func (w *worker) refreshPetStats(pet *entity) bool
```

在`timeline.go:335 enqueueKillFactInWorld`捕获原活动Pet：与主人当前World相同、在显式DataRange16内且Pet活着。**不要求Pet是最后一击，也不要求Pet在被杀怪的原World。** 把credit并入原killFact，沿现有16槽事实队列，不新增永久队列或SourceSeq。

`flushKillFacts`正式排号时将credit绑定原q.SourceSeq与最终q.Experience并预先核安全整数/配置范围。`finishSettlement:530`确认原owner/Auth/generation/opID/指纹、原job成功后，原finished closure只对仍匹配的队首KillID调用award一次，再移除队首。unknown不加经验，仍保留原q/credit；成功重放不再加。不能从当前Role.Experience差反推（可能已升等归零），不能用怪物原始EXP绕过实际确认的角色EXP。

award找到原birth；同birth健康移图仍可识别，原Pet已变成保留尸体时可更新该原对象的训练记录但绝不复活/回血；原对象已真正Despawn/故障销毁则不恢复临时训练。即使当前已安装新宠，也不把旧credit转送给它。原owner被围栏拒绝时不触新owner。

source为同步GainExp，本作等待永久EXP成功后训练会改变同批后缀看到成长属性的时点，必须登记“确认EXP后训练”的适配，覆盖同批两击、死亡/新生、unknown和迟到回调；不能宣称两种时序完全等价。

成长只使用typed/Excel明确输入：初始level=召唤rank、Max=4+rank（17）、经验倍率3、阈值系数20000、HP/AC/MAC/DC加成、按Max训练级减移动/攻击间隔、400ms下限。达到Max不再加经验；一次调用只升一级，超额保留；刷新不SetHP。所有计算先核溢出，不散落game默认。

PB已由client负责人按root授权生成：EntityState字段23嵌套`PetTrainingState{training_level,max_training_level:uint32; experience,next_level_experience:uint64}`，字段24为`is_pet_corpse`。Go字段为PetTraining/IsPetCorpse，训练字段为TrainingLevel/MaxTrainingLevel/Experience/NextLevelExperience。Max时next=0、保留超额。使用现有scene/baseline/stateSequence/handle/Life隔离，不需新独立推送/经济command；经验用Long/BigInt，不要求exp<next，不由客户端while升级。原尸体保持kind2、HP0并明确corpse flag，不再按kind2笼统授予友方资格；尸体训练观察仍是它自身记录。

## 6. 基线、消息队列与运行预算判断

已实读边界：

| 层 | 当前约束 | 判断 |
| --- | --- | --- |
| 客户端BaselineAssembler | 每块及完整基线总计≤2048实体、≤64块；handle跨块唯一 | owner128状态体+角色/8怪/掉落约138实体可被结构接受；动态handle无固定2门禁 |
| server/runtime.go:776 baseline | 当前固定ChunkCount=1 | 不能仅看实体数宣称128完整尸体一定装进一帧 |
| wire/frame.go:9、client/GameConnection | 双端单帧≤65536字节 | Corpse保留状态、完整属性与新训练观察会增加实际PB大小；需要按候选完整状态实编码验证 |
| transport/ws.go:28–43,75 | 发送队列128条、累计256KiB；超预算关闭连接 | 128是消息条数而非尸体数；慢连接仍受字节背压，不因此承诺容量 |
| combat事件/kill队列 | owner共享128事件、16事实；Pending单个 | corpse不是到期战斗事件，不应长期占128 event slots；TTL用独立有界corpse容器/清理。训练嵌原kill，不新增永久任务 |

保守静态分析：若每具kind2只可能保留14/15两种状态，完整正整数PB的上界估计仍在64KiB附近；若把当前四种状态都作为一般合法尸体输入、加完整attributes和训练，128具可能超过64KiB。具体合法配置、字段上界及新PB尚未最终锁定，**本轮没有做实编码，不能报告“128容量通过”。** 需要把最大合法corpse状态、uint32/uint64大身份、训练carry、活动实体和其它baseline元数据一起编码检查。

root已选择基线构建器按实际`proto.Size`分块，保持同一Scene/Baseline/StateSequence、跨块唯一handle，metadata一致，每帧加wire头后不超MaxFrame，且整个基线有明确总字节/块预算。不能截掉尸体、改写身份或静默丢后半基线。现有客户端组装器已支持，但`AppRoot.leave():892`硬要求取消响应chunkCount==1，client将同步改成完整组装确认；真实WS helper也应按完整块组装，ACK不能早于全部块。

单条实体超帧或总快照超预算要明确拒绝配置/隔离，不能删旧尸体来腾空间。分块只是wire完整性的必要处理，不宣布128性能容量通过。

对CPU/内存也须保留边界：原活体10个的战斗数量约束仍成立，但可观察尸体可达128。baseline、排序、状态枚举与Creator节点数增加，不能把“结构可表示”写成手机/GPU/owner吞吐验收。容器按实际存在量有界分配；不为每个room提前创建128个entity/状态map。两个World尸体合计预算共享，不按每图翻倍。

## 7. typed配置与代码分工

root与配置作者正在锁定：`PetRuntime.Enabled`、owner共享`RetainedPetBodyCapacityPerOwner`；legacy_untrained尸体TTL以及每PetDefinition的TTL、基础Fighter/召唤身份；成长规则及DataRange；正式17的500ms/NoPets/首次可行动来源输入。准确Go类型由root提供，不自行猜字段；若新增baseline总字节/块预算，也须显式维护。旧fixture nil/disabled保持规则15；rule16开启尸体运行，本阶段不启用购符或17。

建议我独占game实现：

1. 新`pet_corpses.go`：原对象死亡转交、owner名额/新生reservation、handle allocator、TTL清理与body枚举。
2. 新`pet_training.go`：运行态、每实体Fighter快照、资格捕获、原kill确认一次award与PB观察。
3. 改`pet_lifecycle.go/runtime.go/world_context.go`：动态handle安装、单活动Pet/多尸体居民、baseline/解析/原指针旅行。
4. 改`combat.go/timeline.go/skills.go/skill_cast.go/combat_status.go/displacement_skills.go/town_navigation.go/map_travel.go`中真正受影响的死亡、事实、目标、恢复、占格、历史snapshot与事件处理入口，不复制room。
5. 若选择分块，新`baseline.go`收拢构建与严格字节检查；root协调客户端leave/集成helper，不让多个agent同时写game旧文件。
6. 新`pet_corpses_test.go/pet_training_test.go`及更新已有Pet底座断言；先保留身份/旧尸体回收/错给新宠的反例，再定向race和完整game；全包MongoWS由root冻结统一运行。本报告阶段尚未开始这些测试。

root独占devconfig/Excel/schema/导出/proto；Store/材料回执本轮不改；来源agent核模板/Growth与500ms链；client agent锁定训练观察与尸体显示/块组装。接口确认后再实施，避免把未决定字段偷偷放成新默认。

## 8. 必要验收目标

- 旧bootstrap2尸体和新handle3活宠同屏，原尸体指针/handle/Life/World保持，原攻击/状态ref绝不命中新宠；expire原对象才移除。
- 两World分布尸体共享容量；capacity-1具尸体+活宠死亡仍能归档；满128拒fresh但不删最老体；预约阻止第二个延迟新生，unknown保同token。
- TTL原截止不因新生/换图/重连刷新，Pending不延长；inactive尸体可清理且不唤醒怪物/重刷掉落。
- 同birth原角色/宠物跨图和新旧连接；原credit只给原出生，主角最后一击也能训练，异图/超16/死宠不捕获；unknown和重复成功不加第二次。
- 单次EXP仅一级、超额carry、达到Max不再加、训练不回血，模板Level不被训练级覆盖；快照与AI间隔来自各实体当前统计。
- 新训练PB大整数与128体最大合法编码/分块、完整ACK、取消旅行/leave收束；不以普通帧样本替代边界，也不以容量128替代真实手机/负载结果。

参考只读客户端方案：`.work/pet-runtime-20261029/client-design.md`。本报告仅准备实施合同；17、护符购入与完整职业宠物业务继续未完成。
