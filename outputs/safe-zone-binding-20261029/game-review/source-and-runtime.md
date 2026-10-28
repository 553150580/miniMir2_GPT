# 安全区永久绑定来源与game实施事实（2026-10-29）

本阶段限定为正常进入Town后，玩家成功普通Walk落入来源安全区，永久绑定其中心。初始新生池未认证，保持关闭；技能21仍关闭。绑定是独立永久事实，不保存临时站位、HP/MP、CD、Buff、导航或宠物现场。区域观察字段不表示统一伤害免疫。生产game已冻结，最终全包结果以root统一报告为准。

## 来源精确动作语义

以下路径均是项目只读副本`资料参考/crystal/`，不是来源部署或账号数据。

| 来源位置 | 已核行为与失败边界 | 本阶段范围 |
| --- | --- | --- |
| `Server/MirObjects/PlayerObject.cs:1057–1062` | `SetBind`随机取`Envir.StartPoints`，与正常安全区到访绑定是不同入口 | 初始池关闭，不由Town开发入口或当前个人图推断 |
| `PlayerObject.cs:1064–1068` | `SetBindSafeZone`把`BindLocation`设为区域中心，`BindMapIndex`设为当前地图 | 永久map/center来自此语义 |
| `Server/MirEnvir/Map.cs:2386–2394`、`Shared/Functions/Functions.cs:71–74` | 按来源SafeZones顺序首匹配，`abs(dx)<=size && abs(dy)<=size`，含边界Chebyshev | 复用目录SafeZoneAt；不是欧氏圆 |
| `Server/MirDatabase/SafeZoneInfo.cs:6–8` | Location、Size、StartPoint分别存在；SetBindSafeZone没有StartPoint门禁 | 正常到访可先开放，不依赖初始新生池认证 |
| `Server/MirObjects/HumanObject.cs:2404–2508` | Walk的CanMove/CanWalk失败2406–09、非法格2414–17、关门2420–23、占格2444–45、CheckMovement2466均早于成功落格2472；安全区查找2480、绑定2484，随后才处理格上法术2500–08 | 仅成功普通Walk触发；失败Walk、每tick坐标、健康join、spawn都不能伪造动作 |
| `HumanObject.cs:2514–2613` | Run先逐中间格校验2551–84，最后落格2600、安全区查找2605、绑定2609；不按每个路过格绑定。超重分支2516–19会调用Walk且未立即return，应区分真实动作结果 | 当前没有Run入口，延期 |
| `PlayerObject.cs:4264–4308` | Turn要求CanMove4268，设置时钟/方向后CheckMovement4273可早返；通过后4275查安全区、4279–80直接写绑定，即使位置未变化 | 当前没有Turn入口；不能从攻击转向或失败Walk推导 |
| `HumanObject.cs:2649–2724` | Pushed非法下一格2658是方法早return，成功前缀也跳过后处理；占格2670为break，允许后处理；只有result>0的2688分支，格法术2701–08后才查区2712/绑定2716 | 未来必须`Steps>0 && Postamble`，不能普挂relocate；当前延期 |
| `Server/MirObjects/MapObject.cs:780–800`、`HumanObject.cs:6985–7030`、`PlayerObject.cs:1451–1470` | 基础Teleport先验目标地图/点782再实际换图/坐标788–89；Human在base成功6992后才于7020/7024查区绑定。Player有1458–65前置门禁 | 当前健康travel不是本阶段绑定动作；未来实际成功Teleport可另接已认证入口，延期 |
| `PlayerObject.cs:1379–1391`、`1392–1445` | 普通Revive无SetBind；TownRevive使用既有BindMap/Location1396–97直接落格1421–22，1445设置InSafeZone，没有重新SetBind | 复活不伪造新绑定 |

## 冲撞新发现及修复边界

`HumanObject.ShoulderDash:4975–5157`逐次检查下一格；5002的`!ValidPoint(next) || GetSafeZone(next)!=null`在选目标和推人之前终止。它禁止冲进来源区域，并不禁止从区域向外走。被推目标调用Pushed的5079分支没有同一个安全区guard，因此不能把guard并入通用Walkable或Pushed。

源MP消耗在HumanObject3442–3451已经发生，零步冲撞仍支付单次费用/技能时钟；失败分支5128–40不产生额外两秒移动锁；部分成功前缀5144只结一次伤害。本阶段`server/internal/game/displacement_skills.go:117`增加当前room地图的下一格SafeZoneAt检查，保持其余消耗和部分成功语义。`safe_zone_dash_test.go:13`覆盖第一格阻止、前缀后阻止、含边界角、从区域向外走；`:61`覆盖Pushed可进区域但本阶段不绑定。测试中在小图放置的区域仅用于纯内核几何反例，不是合法生产配置；root已在启用绑定时禁止个人图携带SafeZones。

这是本阶段新发现并修复的遗漏；此前不带区域的6400来源位移向量不能替代该guard的验收。

## 单槽永久任务与帧内顺序

`server/internal/game/safe_zone_binding.go:30`的`prepareSafeZoneWalk`在实际Walk前只预备不可变候选；`movement.go:61`预检后，仍先完成原落格、朝向、MovementRevision、CellEntry和原独立时钟。`:81`随后提交；禁止预检成功但动作失败也写绑定。

候选冻结原Auth、room/连接身份、MapID、成功目标格、来源首匹配区中心/ID、ExpectedRevision、SourceSeq+1、rule/hash、Action=`walk`、32位hex EventID。JSON全请求摘要与Store一致。永久库只持久中心及可信receipt；请求成功目标格不成为角色临时位置快照。只有当前绑定中心相同且BindingSourceSeq>0、已从真实receipt恢复BindingSafeZoneID>0才跳过；legacy同中心但无可信证明仍须第一次验证。

不增加队列：`runtime.go:119`的`PendingBinding`是既有Pending所有者持有的单槽fact。`safe_zone_binding.go:65`在成功Walk后立即占Pending并提交原job；队列满保留同一job，按既有重试节拍重新提交。`CodeOperationPending/CodeServerBusy`也保留原q、SourceSeq、EventID、原Auth和摘要，不采用带err的返回Role，不靠当前坐标重建事实。

`combat.go:280`已有Pending屏障先于状态、已释放事件、新动作；新加入的`:405`屏障位于玩家动作后、宠物及怪物新动作前。因此同tick玩家进区后，宠物不会再走一步/攻击，怪物也不启动新动作。该tick更早已经结算的状态或已释放事件不回滚；未来Due事件、格事件、attack/cast队列仍留在原容器，Pending期间不丢不重建，事务确定后按原scene/handle/life验证后推进。

SourceSeq顺序依赖既有owner串行：`:295/:301`先flush原KillFacts；预备绑定还显式拒绝已有KillFacts，不能抢占旧击杀SourceSeq。Walk落格后无同步DB和owner重入，立即占同一Pending，所以其他经济命令进入原Waiting。绑定成功后`:123`flushKillFacts，再after放行Waiting；以后kill从更新后的SourceSeq+1开始。无新增无界fact队列、无异步偷换当前源序号。

## 回调、重入、unknown与客户端顺序

`safe_zone_binding.go:90`回调所有权同时检查worker当前room指针、未Isolated、原Auth、Pending=true、同一fact指针、原EventID/摘要。旧owner、旧授权、已替换槽回调不释放当前新Pending。永久事实由Store事务保证；新owner的Enter事务读取已提交证明，worker不做同步DB信任判断。

断开原连接不丢已授权原job；回调仍可更新同一永久owner的Role。新连接/新scene/life只允许静默当前状态，不把原动作当当前新事件Toast。硬错误、损坏receipt或fence错误隔离owner而非假成功。Store replay可能返回“当前较新Role+原旧op”，所以核原receipt摘要/点/区/来源后采用当前Role；只有Role.BindingSourceSeq仍等于该请求且中心/区相符并仍在原连接/scene/life才发cause2，否则cause1。

`safe_zone_binding.go:116`成功先发送完整Inventory/Wallet，随后`:121`发送BindingState，且不改EconomicSeq/command水位；永久Revision可随绑定增加，后续经济expectedRevision沿完整Inventory取得。绑定回调没有刷新玩家属性、重建怪物/宠物、恢复HP/MP、清CD/Buff/药效或重置导航。

`runtime.go:639`与`map_travel.go:465`在EnterLine之后、首baseline之前发送cause1；每次attachment或旅行scene一份，非周期广播。source_seq为BindingSourceSeq，legacy0，不是当前全部永久SourceSeq。SafeZoneID来自已核receipt的派生内存字段，不能仅由中心反推（区域可能重叠）。

## 验收与未开放项

纯game八组绑定测试覆盖不可变请求、legacy同中心、trusted幂等、区外/非动作不写、unknown/队列满原job、同tick宠物屏障、已释放事件后续一次结算、Inventory顺序、战斗/经济保持、断线、life/scene/新Auth/新room/新Pending隔离、较新Role回放、坏receipt、kill顺序和计数耗尽；两组冲撞测试覆盖新guard及不同Pushed分支。

真实Mongo/WS本次用例55.60秒、包60.810秒exit0/0skip，日志和命令见本目录report.json。最终helper窄race5.900秒通过，真实初始状态从revision15/source7出发，避免向客户端发布sourceSeq>roleRevision的不可达测试状态。较早完整game52.655秒早于这次纯测试helper修正，明确保留这一证据限制；最终全包以root冻结manifest和报告为准。

Run、Turn、Teleport/Pushed自动绑定、未认证初始新生池、21的200ms跨图、公共Town AOI/NPC、真机与容量均不在本次game声明范围内。没有新增统一安全区伤害免疫规则。
