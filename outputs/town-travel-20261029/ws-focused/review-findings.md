# 独立只读复核发现

2026-10-29，真实WS专项通过后复核。未修改Go；探针只写.work，用overlay叠加到真实game包。证据是`review_probe_test.go`、`review-overlay.json`与`review-probes.stdout.txt`。

```powershell
.\tools\dev.ps1 go test -race -overlay ../.work/town-travel-20261029/ws-focused/review-overlay.json ./internal/game -run '^TestReview' -count=1 -v
```

退出0、game4.946s，两个probe均确认实际反例，PASS仅表示已复现，不表示被复核业务正确。

1. **已取消travel_id可被旧重传重新建立并提交。** `map_travel.go:152`只识别最近plan的同ID，`:165`允许最近终态后再prepare，`:208`覆盖唯一记录。scene20先prepare/cancel ID1，再prepare/cancel ID2，迟到ID1 prepare/commit仍可建立新plan并切Town21。网络scene未变，因此from_scene隔离不能挡住这个顺序。需保留可证明的旧请求拒绝边界，例如同scene/同连接的单调旅行请求版本（当前版本终态保留，旧版本拒绝），或有明确容量/满额规则的本scene终态ID集合；不能无界记历史，也不能静默逐出终态后允许旧ID复活。
2. **返个人图未ACK仍启动新AI攻击。** `combat.go:264`在Conn存在且Ready=false时只做超时检查，继续执行；`progressMapTravel`在Committed返回false，之后`:388`原Auto分支继续。Probe以无旧Attacks/Casts的健康往返，返图Ready=false且Travel.Acked=false，下一step玩家AttackSequence0→1。旧恢复/状态与绝对deadline可按真实时钟继续；是否允许“新自动攻击/导航”需按本阶段ACK门禁明确。若资源确认是新场景动作门禁，应在新AI前停住，不通过整体冻结全部状态掩盖期限行为。现有PersonalDeadlines单测会在未ACK时处理过期怪物/掉落，这是另一类事实，不能用它证明新AI已被挡住。

本轮未发现旅行清空unknown原job、改变原op/指纹或跳过Pending收束的证据。prepare/commit重复不延长原期限；Quiescing同时等待PendingRetry/KillFacts/Waiting/Casts/Attacks；原回调收束后才移交；新ticket健康附着取消未提交plan，已提交world保留；Town离线不扣时分支及状态保留已有真实WS与game专项证据。以上结论仅限已读代码与对应测试，不把读取视为故障注入验收。

## 修后独立重放

两处均已由对应owner修复。新增TravelHistory按Conn/Auth.SessionEpoch/scene保留显式有界ID集合（唯一Excel MaxPlansPerScene256），满集合拒新，不淘汰旧tombstone；新AI门禁位于原怪物/掉落期限处理后、玩家/宠物/怪物的新动作之前，保留旧截止事实。

在不修改server输入的前提下，使用`review_fixed_probe_test.go`及`review-fixed-overlay.json`对同一反例序列作反向断言：迟到旧prepare/commit必须Conflict且原scene不变；未ACK返图step不能推进攻击水位/新攻击队列。`-race`两项均通过，退出0，game4.867s，原始输出`review-fixed-race.stdout.txt`。

原真实WS case未重复运行；此结果仅补两条修复的反例验证，完整冻结Mongo/WS/race由root后续统一执行。
