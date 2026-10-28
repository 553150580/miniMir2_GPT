# 客户端旅行基础只读复核

2026-10-29。范围为MapTravelModel、MapAttachmentCoordinator、MapViewportPreparation、GameSession及消息去重。AppRoot当时仍在补测试，只读取调用合同，不以中途未完成实现报缺陷。随后按root授权扩大到WorldAssets的Sprite消费者释放顺序。所有探针仅写.work，未修改客户端/Go/fixture。

## 已复现的问题

1. **保存的primary Surface/HUD仍绑帧时释放最后资源引用。** Coordinator先释放旧GameWorldView，再挂新view。原GameWorldView.dispose清活跃appearance和prop，却不清保存World/Surface；HUD仅接受sourceMap的失效通知，primary没有生命周期。用真实prepareMapSurface、GameHUD.setMapFrame、GameWorldView.dispose和WorldAssets.dispose，CC节点/refcount轻量stub证明两个保存Sprite在background ref0时仍绑定。`primary-frame-probe.cjs`与`primary-frame-before.json`保留修前证据。
2. **动态特效/状态与先前退役actor只destroy，资源释放时仍留绑定。** CombatEffects.dispose/正常update过期、StatusEffects.dispose/正常update过期均仅destroy。GameWorldView.apply移除actor也是destroy并删索引；同帧的总dispose无法再找到这些节点。`dynamic-frame-probe.cjs`走真实draw/update/apply/dispose，按Creator延迟销毁保留Sprite字段，记录active skill、active status、同帧已退役actor三项仍绑定；活actor.body和prop是已正确解绑的正对照。原始输出`dynamic-frame-before.json`。

此证据是实际生产方法加轻量CC/refcount探针，不冒称已观察到Creator GPU黑屏/资源泄漏画面。同job的新WorldAssets可能同时持有公共帧而掩盖ref0；末次退场或失败路径仍需要完整生命周期保证。root与客户端owner已确认修复，等待当前Creator构建终态后修改并重建。

## 生产API建议

WorldAssets统一同步before-dispose/失效协议，通知保存Surface、HUD与所有动态消费者，全部解绑后才释放sourceMap及owned资源。各系统统一retire(visual)，先禁用节点、清spriteFrame/相关customMaterial、移出父节点，再destroy；普通过期/实体移除和总dispose都使用。不能仅在总dispose扫描当前Map。SourceMapView已有相同清理顺序可复用。

回调必须捕获本消费者及owner代次，不能扫全局当前world。共享Frame对象可被新旧Assets同时使用，Frame相等不足以判断旧回调仍拥有当前保存Sprite。解绑回调失败时的资源保留/重试顺序也应明确，不能先永久标记disposed而让剩余引用无法再清理。

## 已核对且未发现新的重要缺陷

- Model保留connection/binding/role/scene/baseline/life/rule完整身份，uint64使用Long/string/BigInt；提交后旧计划到期不本地回滚，ACK/Ready按完整新附着身份匹配。
- Coordinator异步加载/首视口迟到只释放本任务资源和view；WeakSet防止已退役view被旧回调再释放，旧任务不能ACK新挂载。
- MapViewportPreparation仅取首视口lease，prepare成功不释放，取消/失败统一release；实际view取得自己的lease并ready后调用方归还prime。缓存ready内部有拒绝处理，部分acquire失败不会造成未处理Promise拒绝。
- GameSession当前op终态拒绝可撤回本地推测，但confirmed floor不降；旧scene/旧op回执与Pending不会降低新Entry/SyncReady水位。生产经济入口通过PendingOperations串行化当前角色操作，原op/指纹跨旅行保留。
- SystemMessageLog.entries与seenReports均最多capacity（上限100）；导航提示仅存最后一个身份字符串；没有为每次导航失败无限增加Set。额外消息观察表受当前背包和单帧大小约束。

模型编译及专项：

```powershell
.\tools\dev.ps1 node client/tests/ui/compile_models.mjs
.\tools\dev.ps1 node --test client/tests/app/map_travel.test.cjs client/tests/app/messages.test.cjs
```

TypeScript5.8.2编译通过，23项通过、0失败、0skip，295.8393ms。该结果不替代后续生命周期修复回归、Creator实构或真实UI城镇往返。

## 帧生命周期修后独立重放

客户端owner冻结修复后，按新正式API重放`primary-frame-after-probe.cjs`（真实view.prepare，HUD传WorldAssets，含外部assets.dispose与同Frame新owner后旧通知）和`dynamic-frame-after-probe.cjs`（原draw/update/apply/dispose输入反向断言）。两命令退出0，所有`released_while_bound=[]`。输出保留在对应`*-after.json`，修前探针/JSON不覆盖。

本机Creator3.8.8引擎`cocos/core/data/object.ts:300–309`进一步确认destroy只标记ToDestroy并入队，其文档写延迟到当前帧渲染前。探针检查的是同步资源释放顺序，未假定待销毁对象会实际再次上屏。

## AppRoot冻结后新反例

冻结后读取AppRoot实际链路，发现旅行finally全局清理缺少本请求归属。`stale-travel-finally-probe.cjs`复用现有runtime harness加载真实AppRoot与Session，使用受控时钟推进服务器准备过期，未改生产代码：旧travel1资源仍pending→服务端phase4过期（同连接/同scene）→新travel2完成资源和prime，处confirming→旧1资源迟到。旧1正确释放自己的迟到assets，但其finally调用不带ID的releasePreparedTravel，将新2资源/prime释放。结果`new_prepared_assets_disposed=1`、`new_prepared_slot_cleared=true`，输出`stale-travel-finally-before.json`。

相同无归属清理也出现在reconcileTravel finally；Loading/Input更新也须避免旧任务覆盖新任务。

**可达性限定（进一步读取真实按钮入口后修正）：** 当前AppRoot.button:883–891使用全局uiActionBusy串行等待按钮返回的Promise，私有travelMap仍在等待时第二次真实地图按钮点击会被挡住。本探针直接调用了私有方法两次，证明的是函数防重入归属缺口，不能证明现有触屏入口能触发。现只列为API硬化缺口，未找到绕过button的生产调用链，不据此否定既有Creator构建或宣布触屏业务失败。该限定已及时发root与客户端owner。前两项真实帧释放问题及其修后独立重放不受此限定。
