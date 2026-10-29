# 隔离GUI库名前缀修复

旧前缀 `minimir_dev_game_test_pet_visual_` 加32位run后为65字节，超过Mongo数据库名63字节限制。`before-probe.log/json` 保存只对字符串执行的真实反例（exit1），未接数据库；两份GUI源码前态独立保留。首轮Creator及marker失败的旧运行报告由Root原样保存，本工单没有改写。

新前缀 `minimir_dev_game_test_pet_viz_` 加32位run为62字节。GUI validator先显式检查<=63字节，再核新前缀和精确32小写hex run。新增纯测读取Root Python helper显式DB_PREFIX比对，拒旧65字节、异常名字、main端口、旧rule及其它工程。没有改业务UI路线。

最终5项纯Node测试全部通过，0失败/0跳过，实际原始日志与命令/退出码见 `after-tests.log/json`；JS、测试及Root Python helper三个输入在测试前后摘要一致。GUI当前SHA `c65f3195d32c37230f965a69ad30ed31670504e9d1ad05ef49fec470a26fdbe0`，测试 `bde274383c526851f3aa79c92a186d3a8ca3b9ac6e1b9b5fca8af42877508cb0`。此前 `checks/gui-tool-tests.*` 及implementation-review中的旧SHA是prefix修正前记录，不能当成当前GUI工具摘要。

没有执行DB、Creator、Go或浏览器。旧Creator包与之后可能变化的客户端源码不能当成当前GUI实构资格；真实新包及运行仍由Root串行完成。
