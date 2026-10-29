# 召唤17 game 核心交付

## 最后补修后的冻结检查点

root授权同图NoPets、ScheduleID耗尽、Frozen恢复分支及初始NoPets四项补修后，game共16文件重新冻结。最终专项为`peer-fixes-after.json`：48节点、0失败、0跳过、race开启，16.779秒；完整命令/原始事件/输入摘要均同名前缀保存。before/after/current在本报告更新时一致，具体输入数和16文件hash见report.json。root最终全Go/Mongo/WS仍另行验收，未擅自重跑完整game。

实际修前失败和修后结果保留：

- `same-world-nopets-before.json`及`same-world-nopets-before-fixed-profile.json`均实际失败；修后`same-world-nopets-after.json`36节点通过。Fresh可合法fallback到NoPets当前图且不冻结，同图Teleport不套入图政策；测试后半段使用固定两图政策，回允许图后再真正跨入NoPets才冻结。此前短暂编辑误将NoPets设置放到另一测试，已复核还原，最终组合覆盖全部旧专项。
- `schedule-before-valid-profile.json`实际失败；`schedule-after.json`46节点通过。新cast及攻击在分配前检查耗尽，多目标先整体保留所需次序余量，prepared受理与fresh预留再次检查；不能先扣资源或分出0序号。已付费HellFire后继不可分配时明确隔离，原同批后缀保留，不发0 Order。最后一个合法MaxUint64次序只使用一次。最早`schedule-before.json`误把半月25写成24的前态错误另保留，不将其当合法半月反例。
- `frozen-init-before.json`两个case均实际失败：Frozen恢复后当tick多做一次宠物攻击（宠/怪序号均1）；合法初图NoPets的legacy宠owner观察未抑制且实际行走。修后只有来源Frozen异图且允许分支返回skipPetAI，怪物仍攻击一次；初始化与真正跨图复用一次入图政策，不在每tick按NoPets盲设，以保留Fresh fallback和同图例外。两项同最新48节点组合一起实际通过。

以下段落保留第一次冻结及其发现过程；其中35节点/14文件是历史检查点，不是最后字节。

game生产与测试已冻结。最终定向race为`lethal-and-summon.json`：35个含子例节点、0失败、0跳过，14.762秒，104份输入before/after/当前一致。当前fixture SHA为`684268f3d4bd2f035a52681f8f0531b3f85afc19f7ef9fa826ed66a92522fa5d`。完整game探索轮`game-first.json`为失败，不冒充最终整包通过；最终整包及真实Mongo/WS由root统一运行。本agent未运行Mongo、未停主服务。

上述“当前一致”指报告首次保存时的检查点。其后root继续补devconfig旧版本空RecallName键门禁及17升级预览；依赖变更列表见report.json，game14份冻结hash仍一致，最终业务以root后续统一全包的before/after/current为准，不拿本专项覆盖后来修改的依赖字节。

## 实际行为

- 召唤计划复用原preparedCast、pendingCast和全owner共享容量。Fresh先锁唯一身体预留，再走原材料job；unknown及队列满保留原token/job，成功回执只放行一次临时事件，失败/失效清自己的预留。历史回执仍先于当前输入/scene查询，无plan时不能生成新宠或转成Recall。
- Fresh普通CastIntent返回需要材料且不占槽；Fresh材料RPC遇到同名活宠形成明确失败，交原Store回执，不扣耗也不伪造经济成功。正常Recall使用普通CastIntent，无护符仍可受理，保留MP/动作/CD。
- 请求dir合法且真正受理后才Turn；捕获按请求dir计算的原图Front。500ms到期先原World静态Walk Front，失效才主人当前图脚点；不追加闭门/动态空格或到期NoPets门禁。不合法点释放原预留，不造ghost。新正规骷髅方向为来源DownLeft枚举5，首次普通走攻锁从实际Spawn起算原2000ms。
- Recall锁原宠handle/Life及owner执行代次，500ms内换World不丢。到期只对异图做Back→脚点静态Teleport；同图不搬。NoPets抑制并保留原居民；恢复允许图后自动Recall。自动Recall不受新生2000ms走攻锁，且不改原动作/移动/攻击截止。共享DataRange使用Definition.SourceDataRangeCells，普通跟随2格不替代该来源范围。
- NoPets旅行/21移交只移动主人，保留原宠World；普通允许图继续移交同一原指针。主人真实死亡直接Die并归档异图或当前图活宠，不能由旧Recall/旧birth复活。尸体Despawn同时清掉其它World队列中属于该birth的Recall。
- 每块基线重复相同OwnedPet metadata，只有实际活宠，不含尸体/预留，不将异图宠物加入当前实体列表。RecallSkillIds只从选中技能与原宠冻结RecallName派生；允许不同模板同名，大小写敏感，不按显示名或模板ID代替来源同名。

## 实测和失败记录

`final-narrow.json`为修死亡推送前34节点PASS；最终`lethal-and-summon.json`增加旧死亡观察用例，35节点PASS。正常handler、真实disabled候选完整Fixture.Validate、静态Walk闭门与占格、原图/fallback、主人死亡、旧birth、原共享批首中末、wire多块、unknown/队列满/旧回调都已覆盖；纯模型回调不是数据库故障注入证据。

保留以下真实失败，不覆盖历史：

1. `first-narrow.txt`：新测试怪物漏Monster元数据，baseline panic；修合法测试前态，非生产缺陷。
2. `third-narrow.txt`：新wire测试误用不存在的PB Kind字段，编译失败；改用真实数值PB压力前态。
3. `fourth-narrow.txt`：本批普通攻击正确抽取命中RNG，但测试把“17不抽伤害RNG”的oracle误用于普通攻击；只给已有攻击设置确定性命中，17准备仍由fatal oracle约束。
4. `real-candidate-narrow.txt`：真实候选空accuracy数组与旧ValidateCombat逐阶数组要求冲突，root已修召唤专用严格分支；没有给17伪造伤害/准确参数。
5. `real-candidate-fixed.json`：测试自行打开未定Count单位的材料入口，被严格配置验证正确拒绝；改成保持材料关闭的完整候选验证，不猜pack数量。
6. `game-first.json`：完整game探索轮441节点PASS、唯一TestLethalStatePushesBeforeBroadcastWindow失败，145.038秒；输入期间Root完成最终配置/类型更正，因此source_stable=false。首版主人死亡递归Attacked宠物造成baseline→DeathNotice→baseline，已改直接Die并合并角色终态；原严格两帧断言保留并补原尸体/Owned=nil检查。

## 修改边界与未完成验收

仅修改game共14文件，清单及当前hash见report.json。既有测试只改observation_test.go，强化rule17主人死亡同帧宠物归档断言；没有降低observedWorker版本，没有把常规测试降成16/15，没有放宽生产配置验证。

主profile仍22技能、legacy免费开发宠不训练、材料关闭；17、正常购符及用户pack规格并未开放。本agent只交game实现和纯race证据。真实学习/装槽/扣耗/成长/重召、健康与冷建、正常UI与Creator、全包race由root另验。完整自然养成、三职业技能、公共城镇、真机和容量不在本次通过范围。
