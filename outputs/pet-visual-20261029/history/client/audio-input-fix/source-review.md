# 正常UI触摸与音频解锁：实际引擎分支

真实GUI a2e19b4968b142078cd7c78f3ab7829d已观察到40014、扣Count/MP、Cast、Born、完整10帧与GPU0/4/9，但gestureUnlocked始终false。该失败不是资源缺帧或后端未出生；原WorldAudio只监听全局input，正常Button触摸没有抵达它。本次未启动浏览器/DB/Creator，以下为源码与纯真实类链范围。

本机锁定Creator 3.8.8源码根：`D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos/`。五份实际源码摘要在after.json和validation-4/report.json的external_engine三份冻结中。

- `input/input.ts:340–351`按dispatcher优先级排序，任一dispatchEvent返回false即break；GlobalInputEventDispatcher:66–68才调用全局input事件表。
- `2d/event/pointer-event-dispatcher.ts:49,125–178`的UI dispatcher会认领TOUCH_START；后续已认领TOUCH_END设置dispatchToNextEventDispatcher=false并返回。不是“Button没触摸”或仅全局回调顺序问题。
- `ui/button.ts:888–901`目标Button在CLICK处理后设propagationStopped=true，普通祖先冒泡回调也不能可靠收到结束事件。
- `scene-graph/node-event-processor.ts:257–291`在目标阶段前先执行祖先capturingTarget；`:671–686`把命中的结束事件作为bubbles=true派发。
- `scene-graph/node.ts:1240–1271`公开Node.on/off支持第四参useCapture，取消订阅必须匹配原type/callback/target/capture。

最小生产修正：GameWorldView从实际保存viewport.parent取得Safe，明确注入WorldAudio；WorldAudio监听该Node TOUCH_END捕获阶段，正常按钮之前完成静音引擎解锁，不改事件传播或业务。保留全局input为未被UI认领触摸兜底。current()拒绝旧附着触摸；dispose对原root以同回调/context/capture退订，再stop/clip=null/destroy组件，素材由WorldAssets最后归还。没有DOM监听、QA调用unlock、禁autoplay参数或新固定节点。

纯反例执行真实安装引擎的NodeEventProcessor类（没有重写其捕获/目标/冒泡算法），回调表、池、节点和AudioSource为替身。目标按钮回调按实际Button语义停止冒泡，UI认领后不调用全局input。修前正常目标点击已执行而解锁仍false；修后捕获先解锁、目标仍正常执行；全局兜底不重启，旧scope不解锁，原root capture能准确退订。原Life异步/clip归还、两WAV完整剩余时长也继续通过。after.txt共16项0失败/0跳过，before原失败保留。

实际可听输出、浏览器AudioSource.playing及硬件听感不能由这组替身证明；须由root基于当前TS重新实构并走正常UI验证。新增只读audit.gestureSource用于区分saved_ui_capture与global_input，不允许工具设置。
