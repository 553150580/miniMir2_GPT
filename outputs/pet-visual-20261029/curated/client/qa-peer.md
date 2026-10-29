# 独立QA工具只读审查

本轮仅阅读`prepare_pet_visual_qa.py`、`isolated_creator.py`、`serve.mjs`及`verify_pet_visual_browser.cjs`，没有修改工具，也没有启动DB/Creator/浏览器。

确认的合理路径：候选17只合并到随机run的独立fixture；主仍22技能和legacy未训宠。独立工程按同一`public_gameplay()`生成唯一差异public，正常Creator构建，不运行时替换资源。QA端口18110/11/12与18081独立，主默认18100/1/2和18080；健康同时核fixture/rule18/instance。角色资格只对正常创角、从未owner/学习/绑定的道士CAS设19级与明确Count2，书25由正常创建赠予，没有seed技能/临时body。清库要求随机专名及完整精确marker；finally停自有进程/恢复main。浏览器只走保存按钮，协议与公开audit只读；GPU帧在AFTER_DRAW复制，Recall/健康段连续观察避免仅结束截图漏重播。

已即时发给root/资产agent的具体问题：

1. GUI读取`cooldown_ms`，候选17真实字段是`cooldown_base_ms`，会在启动浏览器前失败。资产agent已接收工具修复；这里不把它记作生产故障。
2. `launch_static`在Popen后读取windows_identity/保存meta发生于其内部try清理之前；该阶段抛错时caller尚未收到child，可能将static_stopped记true而留存刚启动进程。已建议扩大登记/清理范围，无法证明所有权时保留失败而非称已停。
3. `isolated_creator.build_project`只重hash原输入键，新增TS/scene/prefab未计入source_stable。已建议重新枚举受控输入集合；Creator新增meta可以明确单独记录，不能把未检查的新增生产脚本/布局归入“字节稳定”。

本审查是当前源码只读结论，后两项由root维护的工具继续收敛；没有运行故障注入反例，也不提前宣称真实GUI已过。完整原声/actual playing与帧混合审计字段已同步资产agent。WAV meta由root真实Creator导入，客户端没有猜写音频importer schema。

## 修复后只读复核

原两项工具边界已在当前源码收敛，未发现新的实质阻断：

- `prepare_pet_visual_qa.py:155–197,286–307`：Popen后立即把child/node写入共享handle，再读取身份/保存meta；finally统一`stop_static(handle)`。仍存活但meta为空或身份不符时抛错并保留现场，`static_stopped`保持原false；只有原child已退出或核实所有权并停止后才返回true。身份登记异常不再丢失子进程句柄，也不停止未知进程。
- `isolated_creator.py:95–109`：构建后重新枚举assets/settings/profiles/.creator/node_modules及三个根配置文件，直接比较完整集合与摘要。新增TS、布局、public或meta均不能被原键遍历漏掉；library/temp/build仍是明确允许的Creator产物目录。
- `test_pet_visual_qa.py`新增的unknown-static和late-source两反例分别检查未知进程不能stop/不能声称已停，以及模拟Creator退出36但新增assets/late.ts仍失败。这是纯替身测试，不能写成真实进程故障注入。本次只读复核没有重新执行这9项测试。
- marker未知分支：creation_attempted在insert之前设置；finally仅当自有QA进程已停止、三QA端口均已释放后读取完整marker，精确相等才drop随机专属库。marker缺失/读取异常/不同purpose/run/project时保留库并记cleanup_errors，不因“尝试过创建”就删除。
- 主/隔离边界：main仍18100/1/2，QA18110/11/12；独立静态包限定18081+其QA代理端口，主包限定18080+主代理端口。启动前不接管已有监听；静态健康核原PID、RunID、真实build_root和代理端口；业务健康核fixture/rule18/本轮instance。主恢复要求QA已停和主端口空闲，恢复后核主rule18/当前fixture。静态未知异常可以留下需人工核查的18081现场，但整个报告失败，不能冒称完全清理或真实GUI成功。

新旧工具结论各自保留。root所报主Creator实构、完整Go任务和测试9项不是本agent执行，本文件不把它们计入个人已跑检查。当前只读来源摘要及TS冻结复核见`qa-peer-readback.json`。
