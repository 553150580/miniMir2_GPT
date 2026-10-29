# 保存 Safe 与音频销毁顺序复核

本记录只覆盖本机锁定 Creator 3.8.8 源码、生产类轻量替身及纯测试；新 TS 尚未重新实构或执行浏览器换角。失败 GUI run422a443fc2e84cd4a240ed9e2a19b13d 保留为原历史证据，不由本测试替代。

- `engine/cocos/scene-graph/node.ts:1493–1544`：先置 Destroying，发公开 NODE_DESTROYED，再销毁事件处理器，递归立即销毁子节点，最后立即销毁本节点组件。因此外层 AppRoot.onDestroy 可晚于 Safe、Effects 和 AudioSource。
- `engine/cocos/scene-graph/node-event-processor.ts:182–195`：destroy 主动清空捕获与冒泡回调表。WorldAudio 在 Safe 的 NODE_DESTROYED 中同步退休；该回调内不再 off Safe，后续事件表由引擎清空，不靠吞异常。
- `engine/cocos/scene-graph/node.ts:1270–1273`：off 无条件访问 `_eventProcessor`，已被对象清引用的节点不可调用。
- `engine/cocos/core/data/object.ts:651–657`：strict isValid 只含 Destroyed 与 ToDestroy，不含 Destroying。实际退休事件另记 inputRootRetired，不能用 strict 等同正在销毁。
- `engine/cocos/core/data/object.ts:300–309,354–391`：destroy 排队并置 ToDestroy；立即销毁调用预销毁、清引用、置 Destroyed。排队组件仍持 clip，必须先 stop/clip=null；实际已毁组件不再 stop 或 destroy。
- `engine/cocos/scene-graph/component.ts:405–413` 与 `engine/cocos/audio/audio-source.ts:269–271`：组件销毁调用 onDestroy；AudioSource 自行 stop 并清 clip。子树先毁后晚到的 WorldAudio.dispose 只释放自身全局监听与令牌，不重复触碰已毁声道。

生产修复只改 WorldAudio：保存根公开销毁事件提前释放、全局 input 始终先按原 callback/context 退订、普通主动 dispose 按原 root/context/capture 退订两个根监听、源声道按存活与排队状态分别处理。旧异步令牌不能启动排队或已毁声道。同一 Safe 上新实例的监听/声道独立，不被旧实例再次 dispose 清掉。

`world_audio_destroy.test.cjs` 从锁定引擎抽取并执行 Node._onPreDestroyBase/off/emit/_removeComponent、Component._onPreDestroy、CCObject.destroy/_destroyImmediate/_destruct/compileDestruct/isValid、AudioSource.onDestroy，使用真实 NodeEventProcessor。仅节点字段、回调表、调度队列、声卡和 node activator 是轻量替身；对象清引用执行引擎 compileDestruct 的非 JIT 分支，不以手写 destroyed 布尔替代顺序。WorldAssets 的持有/归还使用真实生产实现，decRef 的瞬间断言所有仍活声道已清 clip。此处没有真实 WS、GPU、浏览器声音或硬件听感结论。

初版反例保留 `before.txt/before.json`：4项中3失败，其中准确复现 GUI 的 null.off 调用链。此后测试加强为真实对象清引用与排队根销毁共5项；最终联同既有释放/播放/表现回归29项通过，见 `after.txt/after.json`。所有此前失败均原字节保留。

完整冻结见 `../validation-5/report.json`：389 UI/app、models、strict、PB检查及协议清单检查全部通过；204份仓库输入及8份引擎外部证据分别 before=after=current。10份保存布局在仓库快照内，未修改 Go、PB、Excel、资源或布局。
