# 当前Go冻结输入追加只读复核

结论：未发现新增阻断。本次没有运行Go/DB/Creator，原 `inputs.json` / `review.md` 保留为修正前审查，不把旧摘要当当前。

与前次10文件审查摘要逐项比较，只有 `pet_born.go` 和 `pet_born_test.go` 变化，其余8份输入字节仍等于前次记录。当前生产文件SHA为 `8cb80a389104f58248194937a8538ce58364b331edf1bba3cbc5435431ea3147`，测试为 `f5e545f70a8fafc3220bea9dd0a7c57ae277f81ed57cbba8d227d7d1ca4e3aed`。完整对比见 `current-inputs.json`。

`pet_born.go:20,28,31–33` 现在将完整 `worldRef{Epoch,ID}` 纳入成功SyncReady的附件凭据，并在每次current复核时要求当前World精确相同。它补上“内部World已切换，而原scene/baseline/ExecutionEpoch尚相同”的边界：即使新生fallback身体实际装入新World，旧World的SyncReady凭据也不能授权发布给原观察附件。此前的完整Conn/Auth值比较、RoleID/Origin、Ready、原scene/baseline仍全部保留，没有将旧Auth升级为新会话或只比较SessionEpoch。

发布路径仍先消费局部事实，再检查精确Actor World/同一body/Life/movement/position/direction；完整baseline全组成功入队后才发事件。新增World守卫只取消显示资格，不回滚已消费材料、已安装身体或reservation的正常终态，也不为后续新SyncReady重新挂起旧事实。

新增 `TestPetBornSyncReadyWorldIdentityDoesNotFollowUnannouncedWorldChange`（344–367）确实构造原SyncReady锁个人World、内部换Town而不换scene/baseline、原Front静态失效导致实际Town fallback的新生。它要求业务身体保留、原Conn无30015；再让新会话同步Town并发baseline，也不得补发旧事实。这里仅确认反例及断言与当前守卫一致，实际执行结果由Root全包统一记录。
