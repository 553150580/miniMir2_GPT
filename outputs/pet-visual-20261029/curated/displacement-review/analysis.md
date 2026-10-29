# 位移技能旧回归失败与测试前态修复

本轮完整回归的原始失败是 `TestDisplacementSkillsLearningUpgradeCastingAndReconnectMongoWS` 在旧 `displacement_skills_test.go:154` 将怪物10的当前HP与初始化1000作比较。根代理已归档原始整包报告；本目录 `displacement_skills_test.before.go.txt` 保留原测试字节，SHA256为 `f92bbf93e8efd12c008300e9eae7fd93b782a3faaee766e55fb118ca3703355e`。

旧失败日志只有断言文字，没有具体HP或攻击/命中记录，**不能追溯断言此次实际伤害已经证明来自自动挥刀**。只读代码说明了一条合法、可达的时序：

1. `runtime.initialize` 正式设置 `r.Auto=true`。本测试把角色放在(0,0)、怪10放在(1,0)，正好可近战。
2. `join` 等SyncReady和重复ACK屏障返回后，用例才发送 `AutoBattleIntent(false)`。两者之间worker tick可以受理普通攻击。
3. `combat.step→attack→melee.attackFacing` 将普通攻击加入带原Life/World的到期队列。来源配置 `player_attack_impact_ms=300`；关闭Auto只禁止后续AI，不撤销此前受理攻击。
4. `timeline.resolveAttackInWorld` 对命中、闪避或目标失效发出 `CombatImpact`，之后发送最新权威状态。原用例没有观察这些事件，也没有取施法前的同请求基线，只把初始化1000当作稍后抗拒施放前态。
5. 抗拒(`displacement_skills.go:repulsion`)只调用`sourcePushed`，没有调用`applyDamage`。8为法师Job1，免费宠物只属于Job2，所以该角色不存在宠物攻击这一解释。

只修改了 `server/internal/integration/displacement_skills_test.go`，未改生产、数值、限时或共用harness：

- 在join原有可选observer中记录有目标的玩家`AttackAccepted`及对应`CombatImpact`，容量受现有事件预算限制；空挥不等待不存在的Impact。
- 8角色明确保留正常Auto，直到真实普通挥刀造成正伤害后才正常关闭Auto。该受伤前态通过正常WS而不是私有HP/坐标修改产生，运行日志将记录真实accepted、terminal及amount。
- 收到Auto关闭确认后，在原80秒客户端deadline内等此前受理攻击逐一终结；不新增sleep或时限。
- 使用已有`MoveIntent.cancel_navigation`同Request完整基线取当前owner权威前态。只读核实该分支只取消导航并调用baseline，不改Auto/攻击队列，不结算战斗，因此不会掩盖伤害因果。
- 每阶抗拒前冻结怪10当前Life/HP，施法后严格要求同Life且HP完全相等；整个连接还拒绝任何Skill8的正伤害`CombatImpact`。新增日志打印各阶before/after，仍核实际MP扣耗、位移、学习升级与健康重入。

本补丁只完成gofmt（exit0）；按根代理要求，Creator/GUI并发窗口中没有启动Go、Mongo或Creator。后续定向真实Mongo/race和整包验收由根代理串行执行。新测试目前不能标为运行通过。
