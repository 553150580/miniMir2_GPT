# World核心交接接口（2026-10-29）

核心已冻结交接；完整game race53.952秒通过，最后Town World守卫后的窄race6.676秒通过，不代表21业务开放。root随后独占接入21节点/配置/协议；core agent不再修改server/internal/game共享文件。

## 对象和归属

room嵌入`*worldState`，所以旧同步主动入口的`r.MapID/r.Monsters/r.Casts/...`仍是当前真实World字段，并非副本。`r.PersonalWorld/r.TownWorld`是最多两个固定槽；`r.currentWorld()`返回当前指针；`r.world(ref)`只查本owner两个稳定执行身份，不按map ID/scene回退。`actorRef{World worldRef,Handle,Life}`不再含Scene。

唯一角色仍为原`&r.Player`，宠物仍原`r.Pet`指针。World不另存可受击Player/Pet副本；只存原返回位置和怪物/掉落/导航/事件容器。Role/Auth/Conn/Ready/Pending/Waiting/KillFacts/SourceSeq、Potions/CD/Melee、CastID/ScheduleID等始终只在原room。

`reference(r,e)`给当前World执行引用。`w.resolve(r,ref)`只解析当前居民，供普通输入/推挤。`w.resolveInWorld(r,world,ref)`解析指定原World的目标/角色动作来源；角色迁走后不会还留在旧World。`w.resolveCredit(r,ref)`允许地图HellFire引用这个owner唯一原角色及原Life，但不把它当原World目标成员。无统一HP>0检查；同Life尸体的已受理动作保留，各新cast仍在prepare/accept阶段拒绝死人。

## 原子移交及21到期hook

```go
type worldTransfer struct {
    From worldRef
    Target *worldState
    PlayerLife uint64
    Position devconfig.Point
    PetPosition *devconfig.Point
    OnCommitted func(*worker,*room,time.Time)
    OnRejected func(*worker,*room,time.Time)
}
```

`commitWorldTransfer(r,intent,now)`在事件和批后缀之间调用，核原当前执行World、同PlayerLife、未Pending/隔离，然后`activateWorld`。核心不查HP、不重抽、不扣MP、不写永久绑定，不更改Scene或发送协议。目的World必须在固定槽中，点必须在合法目录边界内；**实际源地形/占位由受信调用方预检**。普通健康travel已在上层查Walkable/动态占格；21由root用TeleportWalkable和明确的源堆叠/宠物规则，不能让核心通用Walkable二次误拒。

移交会保存原World返回点、取消Town导航并归还池、移动原实体一次、增加MovementRevision/CellEntry；不改HPMP/Life/CD/Buff/Potions/Melee。Personal->Town自动设置`SavedPersonal=PersonalWorld`，回Personal清SavedPersonal；正常返回不复制世界。新Town可通过`r.newWorld(mapID)`创建一次并赋`r.TownWorld`，总World序号达到2以后不再分配。初始化已赋PersonalWorld。

核心生产节点是`timeline.go:resolveDueEvent`，返回`*worldTransfer`。root接21时应在普通resolver之前分流：

```go
if e.Cast != nil && e.Cast.Skill.Behavior == "teleport" {
    return w.resolveTeleport21(r,e.World,*e.Cast,now)
}
```

**21是特殊的角色Action，不适用普通火球/近战的caster仍在原图门禁。** 到期用`resolveCredit`核唯一角色/Life，不因同Life HP=0取消；NoTeleport应读`r.currentWorld()`，可信Binding读当前Role。`e.World`是最初接受的事件队列，不能冒充到期当前地图。意图From取本次到期实际ActiveWorld。若同图，Target仍原指针，执行身份不变。

OnCommitted由root新21模块完成：成功TemporalFlux、必要scene/baseline/ACK协议、落区永久candidate，以及技能特定导航策略。跨图的新Scene应先确定，再捕获candidate的观察Scene。目标资源失败/ACK超时不回滚已经成功的服务器移交。新attachment未ACK时，核心step继续旧已释放事件/原期限，关闭新增AI/移动。OnRejected只负责已接受尝试的失败观察；不能再次受理/扣费。

prepare路径新增独立`preparedCast.Scene`，所有正式prepare已填。root21准备也必须填原附着Scene；材料回执仍核它及World/Life。`safeZoneBindingFact.scene`独立保存观察身份，不能从actorRef找Scene。原事件`pendingCast.Scene/pendingAttack.Scene`只控制旧CombatImpact/CastStep观察，不是执行许可。

## 本批事件和容量

`advanceEvents`把两World的Due事件合成一个按Due/全owner Order稳定排序的批。每项有`dueEvent.World`，没有临时swap或复制room。resolver返回transfer后，核心在下一项之前提交，再以同now继续剩余项；不会等未来Due排空，也不会在批中21后静默丢后缀。

两World队列+局部批未消费项由`r.eventCount()`合计。摘入批不归还容量；正在消费的父slot可以交给HellFire子格。Pending/隔离在中途出现，原后缀退回各自World原Due/Order/权限/Amount，Inflight归零。旧`advanceCasts/advanceAttacks`旁路已删除，测试也使用唯一生产调度。

`resolveCastInWorld/resolveAttackInWorld/areaTargetsInWorld/lineTargetsInWorld/skillTargetInWorld/applyDamageInWorld`显式传World。地图HellFire在旧World检查地形/怪物、用原信用和固定Amount，下一格仍实际now+LineStepMS。没有新World的伪造怪物/幽灵Player。原Life复活后旧信用失效。

## 旧World被动进度

无地图动作的Dormant个人World保持原对象/期限，在返回首baseline之前不无条件复活/清掉落。仍有已释放HellFire的inactive World只推进必要怪物状态和原截止，不运行新AI/Auto/Hosting、不第二次推进Player/Pet的恢复或药效。

ACK观察与执行分离：旧地图事件不会因为Ready=false或Scene变化被取消，但事件发包需原World和观察Scene匹配。`observesWorld`/`impactInWorld`明确阻止旧图句柄/坐标被包装到新Town Scene。当前scene基线只含当前World真实实体。

## 落区candidate与旧kill顺序

仍用一个永久Pending和一个owner KillFacts队列；新增的未排号`BindingCandidate`仅有一个槽。每次实际事实捕获取得单调`FactOrder`，旧World kill带OriginWorld/Order/DropPoint。

root在成功21落区且需改绑定时构造：

```go
fact := &safeZoneBindingFact{
    request: persistence.SafeZoneBindingRequest{
        ExpectedRuleVersion: ..., ExpectedConfigHash: ...,
        MapID: ..., Position: actualPoint, Action: "teleport", EventID: frozenEventID,
        OriginBindingSourceSeq: originalProofSelectedAtDue,
    },
    center: domain.RoleBinding{MapID:...,X:...,Y:...}, zoneID:...,
}
if !w.queueBindingCandidate(r,fact) { /* 不能丢成功动作：移交前应预留/核此唯一槽 */ }
```

queue只冻结动作/Auth/scene/Life/Order，SourceSeq和ExpectedRevision明确为0；OriginBindingSourceSeq不改，保留到期原证明。`flushKillFacts`比较kill头与candidate.order，先来事实先admit，只有admit才取最新Role.SourceSeq+1/Revision并计算不可变摘要/原job。旧kill不会迫使200ms移交等待，但它的SourceSeq仍先于后发生的落区绑定，21后缀kill又在绑定之后。candidate与kill共享现有KillQueueCapacity总量，不是额外无限队列。

`queueBindingCandidate`本身不校验来源合法动作/地形/是否需重复绑定；这些由root21成功后置和Store正式API负责。它也不自动flush，root可在完成本批必要操作后调用`flushKillFacts`；若马上产生Pending，剩余已释放事件会原样退回。未知提交沿原q/授权/OriginBindingSourceSeq重试。

## 异步永久结果和原World掉落

`settleInWorld(r,originWorld,dropPoint,q,finished...)`与`finishSettlement`捕获原room/Auth/slot代次/操作ID/摘要/OriginWorld。旧callback不能清新slot或新Auth/新room；unknown不采用out Role、不换job。成功Role/Inventory更新同owner；Loot仅写原World和实际提交时刻起算的原500ms期限，不读当前Town的Player.Pos、不因返回刷新。PickupStopped会停止两个World的Auto，避免返回原图后越过满包门禁。

## 当前验证口径

World专项已覆盖唯一实体/尸体已受理移交、新cast拒死人、批首中末/同Due跨World顺序、Pending后缀、旧地形HellFire/原Life、inactive共享容量、原World Loot/unknown/旧slot、Dormant与Reactive截止、ACK前无新动作、candidate和kill顺序。首轮完整race发现3处手造preparedCast缺独立Scene的pure fixture，已补fixture，不放宽生产门禁。完整game race53.952秒通过；随后仅补Town World nil/错MapID守卫，World/MapTravel/ACK窄race6.676秒通过。未跑最终21实际WS或宣称21已开放，root接入后的最终全包另验。
