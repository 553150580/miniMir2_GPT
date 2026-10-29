# PetBorn 服务端只读二审

结论：本次已读实现中未发现实质阻断；未修改Go，未运行Go/DB/Creator。此结论是源码审查，不替代Root统一真实WS/race结果。

- `summon_pet.go:89–125`：唯一生产发布点在非Recall分支实际 `installReservedPet` 成功之后；同一Pending token、原Caster Life/执行身份、原World与动态出生位置先核实。失败/Recall提前返回；`newPetBirthFact`又拒legacy/非训练新生。原Front合法时留在原World，fallback采用当前World，事实精确引用实际安装的World/body；没有以CastAccepted或500ms推断成功。
- `pet_born.go:23–52`、`runtime.go:414–434`：附件凭据须在原Conn/完整Auth/角色/scene/baseline、Ready且绑定匹配时捕获，SyncReady实际成功入队后才保存；发送失败关闭原Conn，身份在发送时改变不保存。新票/断开/初始化/隔离路径会清理或使旧凭据失效，旧指针不能升级成新附件。
- `pet_born.go:70–81,93–111`：局部事实保留原Actor World/handle/Life、原对象指针、definition、position、direction、movementRevision；发布前须仍是当前World的唯一活宠、仍由同一PetBornReady持有。跨图、换birth、死亡/尸体、位移、方向变化均失败关闭显示；不迁移或改写业务身体。
- `runtime.go:814–846`、`baseline.go:17–61`、`pet_born.go:113–140`：先构造完整同水位合法分块，含真实居民、尸体及Owned，检查实体/分块/实际wire上限；随后逐块向捕获的原Conn入队，每块前及事件前复核附件/身体及Sequence。全组入队成功后才发送配对30015。这里证明的是完整组先成功入发送队列，客户端是否实际接收/应用由客户端模型和真实WS另外核验。
- `pet_born.go:94–99,124–140`：先消费一次局部fact，基线或事实发送失败关闭原Conn并结束，不进Waiting/经济重试队列、不保存到Role/Owned/World快照，故新票或AutoRecall不会补发旧事实。没有永久资产回滚或二次材料扣耗接口。`resolveSummonPet`的defer只归还对应新生reservation，安装成功已消费token，不删除已生效身体。
- `pet_born.go:84–90,102–107,132–138`：出生时点来自实际resolve调用的本批now，而非旧due；elapsed使用Go单调差并拒负数/uint32溢出，协议保留uint64身份和Unix时间。没有客户端本机时钟推算的依赖。
- `pet_born_test.go` 已有针对实际安装、原附件、断开/新票、跨World、死亡/新Life、位移/Sequence变更、基线第2块失败、事件发送失败、elapsed及局部重复调用的源码反例。多分块压力样本手工扩大130身体预算明确只测wire分块，不能据此宣称运行128容量验收。
- `integration/first_loop_test.go:468–518,572–581`：新增observer在正常统一readFrame完成状态投影后执行；join中的两段直接读也显式触发，避免资产/Join/旅行until吞掉30015。`summon_pet_test.go:327–404`仅保留当前一组，最多64块/2048实体；先检查完整同scope基线含精确出生身体，再核30015的角色/绑定/map/cast/watermark，不会靠最终宠物状态倒推事件。正常WS用例期末要求事件总数仍1，因此Recall/健康票/冷建补发会显式失败。

审查没有重新执行上述测试，也不将结构守卫描述为真实网络丢包、未知事务故障注入、完整客户端/GPU或容量结果。所读输入摘要见 `inputs.json`。
