# 宠物生命周期真实WS用例准备

新增文件仅 `server/internal/integration/pet_runtime_test.go`。当前状态为已编译、等待root通知执行真实MongoWS，不能标成业务通过。

真实包装编译命令与原始输出见 `compile-result.json` / `compile.txt`：`./tools/dev.ps1 go test ./internal/integration -run ^$ -count=1 -timeout 90s -v`，exit0，明确no tests to run。全部当前Go文件及两份运行配置在编译前后摘要相同，见compile-before/after.json。没有重复配置Python测试，没有运行Mongo或whole Go。

用例将正常注册账号与两个道士角色，不预置角色经济、不向room注入body。局部cfg将宠物HP设1、攻击0，并同步匹配typed Definition.Base；怪物固定伤害20、命中255，前格保证优先打宠，动作/移动长间隔只阻止后续转杀主人；尸体TTL设3000ms。正式两书、fixture与主180秒来源参数不修改；预算128不变；17及护符始终关闭。

第一角色验证真实初始未训练活宠、怪物首击归档、原handle/Life与HP0/IsPetCorpse、没有第二活宠；1.3秒后正常新票重入原scene，保持尸体原位置与原截止，到期移除。第二角色在原尸体尚未到期时完整停止全部节点后冷建，验证新scene没有恢复旧corpse，仅按显式legacy免费bootstrap产生新宠。每段检查Role经验、经济/来源水位、revision、钱包与报告不变，角色BSON无临时宠物/World字段。

NoMaster友方排除由game专项覆盖。本WS不把14/15材料入口拒绝当作友方筛选证明，也不为此打开护符。当前不覆盖训练自然收益或17正常耗符召唤。
