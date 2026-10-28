# 群疗集成失败只读诊断

生产代码、真实测试源和fixture均未修改；对照首轮归档79项SHA，当前差异为空。两轮仅通过.work中的Go -overlay副本增加观察，第二轮另外显式等待2500ms作受控因果实验，不计入原用例通过。

首轮全包失败仅有Go日志，没有逐帧，因此不能逆向断言首轮一定由宠物死亡造成。其“准备群疗310/30”取自700ms睡眠前最后已读快照，不是施法时的新观察。

纯logging诊断轮正常：Tao04:18:03.493收到HP310/pet30；学习done03.907；CA29在04.610，500ms延迟后05.189/05.191两目标各11入队；05.492收到玩家299、宠物35，实际恢复通过。

受控慢窗口轮确认：原准备快照仍310/30；额外等待42.985→45.486；CA前已读宠物0/60、仍(8,7)，三个攻击者attackSequence从1变2。45.495收到CA，46.001仅玩家Impact29金额11。诊断副本以EXPECTED_FAILURE_OBSERVED立即失败，证明此条件会让原双目标等待永不满足。时间为日志读取时间；不伪称精确还原首轮每一服务端tick。

来源与当前代码均按生效时HP>0筛群疗目标；死宠不发该目标Healing Impact是正确规则。源怪周期2500ms，本例三怪每次各打10，宠物原60经首波剩30，下一波即死。学习RPC在全包竞争中耗时约2.78s，单项约0.48s，再加未读取观察的700ms及技能500ms，旧前态跨过危险窗口。

本次改动没有改变群疗target/amount/queue/tick；手动流程Auto=false，AutoEnabled与未被调用的Pushed不影响。CA由同worker同步先发送并入FIFO，Impact只能后续到期发生，未发现本次Impact被castAccepted先行吞掉的可能。

建议只修隔离用例：先在低威胁前态完成购书学习，再启动受伤波；从既有等待+500ms延迟+恢复tick观察窗和怪物最大伤害/攻击周期计算宠物生存余量。发起/生效观察明确校验HP、life、范围，按本次cast_id统计两个友方，并在前态失效时立即诊断。不要修改生产过滤、群疗数值、唯一Excel或靠加timeout覆盖。

完整证据：run1.events.jsonl、run1-summary.json、run2/events.jsonl、run2/experiment.json、两个instrumentation.diff、original-sha.json及diagnosis.json。
