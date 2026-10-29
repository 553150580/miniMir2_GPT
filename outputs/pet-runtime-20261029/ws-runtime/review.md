# 宠物生命周期真实WS用例准备

新增文件仅 `server/internal/integration/pet_runtime_test.go`，已冻结。最终真实MongoWS单项race通过，0skip，case14.85s、package20.338s，包装命令27.114s/exit0；全部Go与两份运行配置before/after摘要相同。原始命令、输出和摘要见`real-mongo-ws-result.json`、`real-mongo-ws.txt`与对应before/after。

早期包装编译命令与原始输出见 `compile-result.json` / `compile.txt`：`./tools/dev.ps1 go test ./internal/integration -run ^$ -count=1 -timeout 90s -v`，exit0，明确no tests to run，不能代替最终真实执行。没有重复配置Python或whole Go。

用例正常注册账号与两个道士角色，不预置角色经济、不向room注入body。局部cfg将宠物HP设1、攻击0，并同步匹配typed Definition.Base；怪物固定伤害20、命中255，前格保证优先打宠，动作/移动长间隔只阻止后续转杀主人；尸体TTL设10000ms。正式两书、fixture与主180秒来源参数不修改；预算128不变；17及护符始终关闭。

第一角色验证真实初始未训练活宠、怪物首击归档、原handle/Life与HP0/IsPetCorpse、没有第二活宠；1.3秒后正常新票重入原scene，保持尸体原位置与原截止，到期移除。第二角色在原尸体尚未到期时完整停止全部节点后冷建，验证新scene没有恢复旧corpse，仅按显式legacy免费bootstrap产生新宠。冷建join完成（包括初基线/ACK）必须严格早于“第二角色最初Enter开始+TTL”，这个时点早于任何实际死亡，所以不是在旧尸体已自然到期后才断言不恢复。legacy初始Life0是既有合法语义，本测试比较同birth，未改生产以制造非零Life。

每段检查Role经验、经济/来源水位、钱包、物品与报告不变；revision与SessionEpoch按真实成功Enter次数各恰+1（首次一次、健康/冷重入各第二次），角色BSON无临时宠物/World字段。首轮真实FAIL是测试错误地把正常Enter的revision+1算作宠物永久变更；`auth.go:246–250`明确该接管规则，已修测试计数。该轮原始源码/日志/摘要独立存于`first-real-enter-baseline/`，不是产品反例；3秒TTL与Life非零假设的早期草案从未作真实产品失败，见`draft-compile-only/`。

NoMaster友方排除由game专项覆盖。本WS不把14/15材料入口拒绝当作友方筛选证明，也不为此打开护符。当前不覆盖训练自然收益或17正常耗符召唤。
