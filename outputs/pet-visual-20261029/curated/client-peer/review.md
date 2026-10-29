# PetBorn客户端独立只读复核

范围：`PetBornModel.ts`、`WorldAudio.ts`、`GameSession.ts`、`AppRoot.ts`、`GameWorldView.ts`、`EntityAnimation.ts`及资源所有者接线。未改生产TS、未启动Go/Mongo/Creator。另按根代理新任务修正了独立位移WS用例，详见相邻displacement-review，不混入本客户端复核结论。

## 已真实复现并修复的一项遗漏

手动旅行进入quiescing后，`GameSession`将输入ready设false，但服务端仍保留原World观察/SyncReady stamp、继续已付费事件。原30015门禁只接受ready，AppRoot也只在ready消费，所以在收束期间成功出生会永久漏掉首Appear；若随后旅行取消，原场景仍在却不会重发出生。其它Cast/Impact已明确允许原World在quiescing继续投影。

实际轻量探针读取真实Session/Travel/PetBorn/完整Assembler，仅连接为内存替身。修前控制组ready出生正常，quiescing完整含body基线已应用但pending=0，其它旧Cast回调=1；Cancel恢复同attachment ready后无播放。精确修前源码SHA与工具输出转存在 `quiescing-birth-before.json`（注明首次实际工具输出转存及重复保存时已经读到修后代码，不伪造第二次修前运行）。

客户端代理最小修改Session接收和AppRoot消费为 `ready || travel.phase==='quiescing'`，保留原角色、规则版本、完整attachment及模型全部身份校验；未改变输入门槛。独立反向探针 `.\tools\dev.ps1 node .work/pet-visual-20261029/client-peer/quiescing-birth-probe.cjs` 实际exit0：pending=1，收束期可消费，Cancel后不重播。输出及修后SHA在 `quiescing-birth-after.json`。该探针不声称真实WS、GPU或可听声音验收。

## 其余当前范围未见实质阻断

- `PetBornModel`保留最多一条pending及同attachment单调Life消费水位；64位身份经过string/BigInt，不转Number。角色、connection/binding、scene/baseline/map、body handle/Life/Definition/MovementRevision/方向和正HP均精确匹配。完整Assembler尚未到达事件stateSequence时只等待，不从Owned或首次baseline猜出生。资源失效只丢pending而不重置消费水位；换真实attachment才重置。
- AppRoot先apply完整最新基线，再消费出生。GameWorldView不从事件创建body，也不能将新Life套到旧visual；同Life新移动修订会同步cancelAppearance，死亡/尸体/位移优先。表现时钟仅使用服务端elapsed加本地单调接收后时长，未将客户端Date与服务器Unix相减。
- WorldAssets预载并持有AudioClip，两个声道使用保存Effects容器上的AudioSource；token、原World/visual/Life/位移及disposed条件共同约束微任务。资源失效在引用归零前同步取消并stop、clip=null；旧回调不清理新声道。音频完整剩余WAV可超过图像窗口，开始资格仍受600/1000ms真实配置窗口限制。
- 另只读核Creator3.8.8真实引擎 `cocos/audio/audio-source.ts`：公开clip=null会重置播放器并将_lastSetClip清零，迟到load比较原clip后销毁旧player；组件onDestroy也stop+clip=null。因此这里没有退回不可撤销playOneShot。该引擎阅读只证明代码生命周期机制，不证明用户设备解锁、实际可听音频或延迟容量。

独立运行 `.\tools\dev.ps1 node --test client/tests/app/pet_born_runtime.test.cjs`，实际exit0、6项PASS、0skip（工具wall约1.87秒，Node报告约509ms）。这些用例确实调用真实GameWorldView/WorldAssets/动画/位移，但Cocos Node/AudioSource是轻量替身；不能把测试标题中的“真实AudioSource”当作引擎实际音频播放证明。真实GPU帧、触屏解锁及音频状态由根代理串行Creator+浏览器工具验收。

## 对照来源Extra

`BoneFamiliar.Spawned`先调用base广播，随后才设置Summoned=true；`GetInfo.Extra=Summoned`。因此原首次广播Extra=false触发客户端MonsterObject的Appear，后来Recall/健康观察Extra=true不触发。当前仅实际fresh install成功、原观察World且已SyncReady时发PetBorn，不为Recall/健康/未观察World补播，符合该区分。500ms法术到期、2000ms宠物行动锁均未被当作出场动画；Appear10帧×100ms来自独立来源资产合同。
