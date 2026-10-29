# game召唤实现只读peer

2026-10-29；对照../source-review/review.md。没有修改Go/TS/PB/测试、没有运行DB或重跑测试。这里的执行证据仅引用server-core/lethal-and-summon.json：记录为35个测试节点、exit0、mongo_enabled=false；本peer不是其执行人，更不把尚未运行的新WS验收写成通过。初次读取时61份game输入与该记录摘要一致；归档时world_context.go与summon_pet_test.go已有作者后续修改，inputs.json如实列出当前差异，不对新字节冒称35节点测试已覆盖。

## 需要root决定的来源边缘

**1. 初次NoPets图的legacy活宠未应用登录冻结（源码推断，反例尚未运行）。** runtime.go:691–698创建初始宠、attachLegacyPetState后没有设置FollowSuppressed；summon_pet.go:146只在异图、原已抑制或距离超16时调用Recall。因此合法rule17 fixture若primary.no_pets=true、bootstrap仍legacy，初始同图近身活宠会保留false抑制，combat.go:438进入普通AI。源PlayerObject登录1321附近调用ApplyMapEntryRules，1515–1535会冻结已有未死非豁免宠。必要定向反例：NoPets初始图正常initialize道士，首baseline的Owned应表示跟随抑制，普通step不新增宠物走/攻；先不把主NoPets=false的当前流程写成故障。已发root/server，是否让legacy明确豁免或补登录政策需登记。

**2. 同图传送额外触发禁宠冻结（初审发现，随后源码已修，待作者验证）。** world_context.go:206在old==target也设置FollowSuppressed=true；源PlayerObject.Teleport:1486只在mapChanged时执行入图规则。原宠可能由允许的新召fallback出生在当前NoPets图、尚未Frozen；此后同图21落点距宠<=16时，源不额外做入图冻结，也不会因距离自动Recall，当前activateWorld却立即冻结。当前两图NoPets=false、普通旅行收束及施法锁限制了该分支的正常可达性。归档回读已见activateWorld新增mapChanged，并在204/207只在真换图时解冻/冻结，新增TestSummonPetNoPetsSameWorldTransferDoesNotApplyMapEntryFreeze；peer未执行该测试，不能把先前35节点结果覆盖到新字节。

**3. Frozen跨图强制AutoRecall的“本次AI立即return”未保留（细时序候选）。** 源Monster.ProcessAI:1686–1690在Frozen且异图且主人允许时PetRecall后return。当前summon_pet.go:141–149返回void，combat.step:403后仍到:438，旧宠时钟已到、个人图相邻怪且Auto=true时可能同tick攻击/移动。出生2秒锁独立正确，此问题只针对已冻结旧宠，不应混为出生锁。主NoPets=false且健康旅行先搬宠，使该状态不是本轮普通主路线；建议以Frozen异图原birth+主人允许图+到期旧攻钟的纯状态反例判定，或登记共享tick适配。已交server只读判断，未实跑。

**4. 旧共享Order计数器上溢仍可回绕（既有硬化项，非本17独有）。** skill_cast.go:254检查CastID但accept:286直接ScheduleID++；普通攻击/地狱火续格亦如此。若ScheduleID=MaxUint64，新17事件Order会回0并在相同Due下排到旧事件前。必要边界反例可放旧致死攻击Order=MaxUint64，再受理新召：应拒绝而不扣费/占预约，不能回绕后先出生再死。现实运行不可能在本次负载期间耗尽uint64，本项不能写成真实WS压力失败；root可统一硬化或单列既有边界，勿把普通35节点通过扩大为该上溢已测。

## 已核对正确、无需重复泛测的改动

- 方向旧疑点已关闭：summon_pet.go:28/37接合法direction冻结Front，skill_cast.go:158/262/287在受理成功后才转玩家；新BoneFamiliar自身方向由pet_training.go:64–70取DownLeft=5。对应TestSummonPetCapturesAcceptedDirectionAndRejectNeverTurns保留旧4/请求2条件，源码与精确来源一致。
- 新生/Recall分离正确：受理NoPets在prepare；Fresh在原World队列，Due原Front静态TeleportWalkable优先、失败才主人当前脚点且不增加到期NoPets检查；Recall凭同handle/Life/执行epoch重取当前resident，跨图Back→脚点，同图不移动。TickResidentPetFollow不被出生ActionReadyAt挡住；relocatePet/beginMovement不重设原移动/攻击/ActionReady时钟。旧回调、换Life/换body不得转成新Recall的前置在旧回执与plan gate之间保留。
- Owner.Die与NoDoubleSnapshot修正合理：summon_pet.go:151–165直接置死/移除死亡状态/归档原对象，不再递归走Attacked。外层combat.go:253–268随后一次DeathNotice+完整baseline；archivePetDeath不发snapshot，Owned此时nil，当前图能含原尸体、异图尸体留其World。observation_test.go:80起仍严格断言2帧并增强尸体/Owned=nil，没有放宽旧断言来掩盖重复推送。
- 共享事件批没有观察到17新增丢前后缀：timeline.go:173–226先摘批排序、Inflight计数，逐项消费/遇Pending重放未消费后缀，Summon走专用dispatch，不依赖主人还在原World可受击居民。首中末17的共享批反例、unknown保原预约、旧成功/unknown不清新预约、Despawn清异图Recall均已在现测试源码中明确覆盖；peer不重复运行。ScheduleID上溢另见上项，不混淆普通排序通过。
- Owned元数据足以支持当前客户端：ownedPetPB:169–192仅取唯一已安装活宠及其真实World，含HP/MaxHP、训练、冻结状态与服务端名称匹配派生的已学RecallIDs；不含尸体/预生、不传落点、不把异图宠塞entities。runtime.baseline:790先构建完整Owned，:796–815同一同步worker从同一原宠取当前图实体及训练；没有中间回调写入。splitBaseline:21–33克隆整套公共元数据，每块真实wire预算含Owned，非按实体个数估包。客户端可在完整同scope/stateSeq后选择普通Recall并按handle/Life/训练对拍；不需要新增客户端猜名或坐标字段。

## 范围

当前主profile17仍22技能、legacy未训免费宠、护符和17入口关闭；两图NoPets均false。正常购书/购符、主pack数值、自然成长、17真实GUI及完整1.80认证不由此peer解除。上述边缘已提前发送root/server；后续若源码修正或明确登记适配，应以新的源码/测试记录补充，不改写本次读值为修后已运行结果。

## 后续修复只读回读（独立保留上文发现历史）

server-core/peer-fixes-after.json为作者执行的48节点race、0fail/skip、16.779秒、mongo_enabled=false。peer没有重跑；当前62份game字节与该记录逐项相等，fixes-readback.json保存摘要。四项发现现已闭合到源码：

- initialize:699及真正跨图world_context:205–206统一调用applyPetMapEntryPolicy；同图不调用，Fresh fallback也不调用，保留源例外。
- tickResidentPetFollow:163返回skipPetAI；combat:403/438只跳本只宠物走攻，不跳怪物AI，保留源Frozen异图Recall return。
- prepare/accept/material-reserve都预检ScheduleID/CastID；多目标攻击按needed整体检查剩余Order，最后可用Order只使用一次，不回绕。
- 已付费Hellfire不能续排时明确隔离并保留其他共享事件，不造Order0；未借该边缘修改其它普通队列时序。

来源方向、Owner.Die合并快照、Owned当前/异图元数据前述核对继续成立。当前没有遗留于本次所读game字节的peer阻断；真实WS/全包/Creator/浏览器各自证据由root另记，不用这份只读回读替代。
