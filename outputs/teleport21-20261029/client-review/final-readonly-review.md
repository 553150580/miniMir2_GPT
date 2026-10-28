# rule15 客户端、协议与GUI证据终审

结论：所审迁移协议、客户端归属和既有GUI证据分层一致，未发现新增客户端阻断。下列结论不替代报价修复后的新实构、最新真实全包及阶段最终归档。

- **测试边界正确。** 原 `final-rule15/` 的328项与修复后 `final-preview-fix/` 的329项均为UI/app模型、真实TS配受控Cocos替身和资源契约；不是Mongo/WS并发证明。67消息/134个64位字段/438个PB边界也明确仅为静态wire验证。World/game及新增Pending受控回调race均保留组件边界；真实Mongo/WS与冷重建另见 `../ws-focused/report.json`，未冒称完整网络UnknownTransaction故障注入。
- **实际GUI证据可追溯。** 成功件是 `../browser-verified/report.json`：9项、0错误、0警告、全部Entry为rule15；预置仅等级19和余额，书13、21、绑定及位置未预置。首轮 `browser-final` 失败及 `../diagnostics/first-login-selection/` 必须保留，不能当成功件。Town回归7项0错误/0警告；通用UI6项保留1条方向警告。
- **协议与真实观察一致。** `world.proto:58`、`TeleportModel.ts:49`、`GameSession.ts:139` 严格隔离role/连接binding/fromScene/Life/cast；独立32hex transition匹配新Entry，未借用手动travelId。真实大scene为12811381193842506751，仍保持精度；上行21无目标坐标，30014上行计数0。跨图结果→Entry→ACK时间为1793222082468→2469→5165，同ID/地图/落点匹配；MP255→245→232。此网络时间只证明先迁移后等资源ACK，不能证明墙钟精确200ms。同图30013无新Entry/ACK，健康票空transition且无到达光重播。
- **保存资产未变。** 已读核两张实际截图（跨图入场、Flux详情），并核10份scene/prefab当前摘要均与原实构一致。相对HEAD，资源变更只有public gameplay；保存prefab、场景、来源地图资源及既有FX帧无tracked差异或新增未跟踪文件。21沿用Magic1590–1599及通用250–269原帧；没有新固定布局或Flux附着美术。
- **World迁移关键门禁保持。** `world_context.go:42/149` 在原World/Life、唯一Player/Pet、修订溢出及永久围栏有效后才提交；`timeline.go:165` 用全owner Due/Order与共享局部批容量，原World目标居民与跨World来源信用分别解析。`teleport21.go:179` 成功提交后再发结果/Entry，server Ready=false；`AppRoot.ts:394/608` 仅当前baseline/首视口/ACK/SyncReady及精确运动水位消费一次到达光。迟到资源只释放原owner，经济Pending和command不因瞬移改写。
- **确认并公开的实际调度限制。** 先前tick kill已进入永久Pending时，`combat.go:286`、`timeline.go:166` 和transfer守卫会延迟21；不能宣称200ms硬实时或完全不等旧kill。本次新增 `teleport21_pending_test.go` 以真实原配置Hellfire/21及kill slot/job复现，两子例race通过（包5.016s、93输入稳定）。200/400ms保持原Due、MP、cast/order、原slot/job，不重扣或抽落点；原回调解锁后的首个650ms模型tick才执行，Flux从实际now开始。该测试是受控组件回调，不是Mongo慢事务或网络unknown注入。Root保留A28安全冻结的决定与原计划§6一致；当前实施边界文档已明确此例外。

归档尚需区分：旧270.92秒构建与9项GUI属于报价label修复前；本次只改SkillAssetsModel两个单位中文并新增展示测试，329项/strict已过，但旧截图不能替代新报价修复实构或预览证明。当前历史 `checks/go.json` 仍是exit1失败件，应引用修复后真正终态的新全包证据。来源锁同步后，仅fixture的_source_evidence.source_locks字节变化；`fixture-source-lock-sync.json` 证明运行字段/public不变，不能把旧185输入报告改写为当前185逐字节全同。

本审查未修改生产源码；唯一新增Go专项由Root另行明确授权，仅运行该narrow race。未启动构建、浏览器或服务。
