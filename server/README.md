# 首个可玩闭环服务端

`cmd/nbserver` 是同一 Go 程序，支持 `account`、`game`、`gm`、`supervisor`。当前装配仅接受 development、127.0.0.1 监听与本任务专用 MongoDB 副本集；未提供生产 TLS/发行配置时拒绝生产启动。数值来自 `config/development/first-loop.json` 的有来源候选，`runtime_ready=false`，不宣称已经还原目标 1.80 或通过完整平衡验收。

从项目根目录运行：

```bash
./tools/dev python3 tools/environment/mongodb_dev.py start
./tools/dev python3 tools/server/dev.py start
./tools/dev python3 tools/server/dev.py status
./tools/dev python3 tools/server/dev.py stop
```

控制器先构建 `server/bin/nbserver`，每次生成独立 run-id、日志与 PID 元数据，只有 PID 命令行及 run-id 全部匹配才会停止进程；未知端口占用不接管。状态和日志在被 Git 忽略的 `server/.dev/`。没有自动开机。MongoDB 由独立环境工具管理，停止游戏不会停止 MongoDB。节点内部鉴权密钥保存在用户私有目录，不写进仓库或日志。

## 节点与协议

| 节点 | 地址 | 接口 |
|---|---|---|
| account | 127.0.0.1:18100 | POST `/rpc`，`/internal/register-game`，GET `/healthz` |
| game | 127.0.0.1:18101 | WS `/ws`，GET `/api/role?role_id=<hex>`、`/healthz` |
| gm | 127.0.0.1:18102 | 只读 GET `/api/status`（内部密钥）与 `/healthz` |

`/rpc` 使用共享 PB：10009→10010 注册、10001→10002 登录、10003→10004 角色目录、10005→10006 建角、10007→10008 入服票据；除注册登录外均携带 `Authorization: Bearer <account_token>`。保留 `/api/register`、`/api/login`、`/api/roles`、`/api/roles/create`、`/api/ticket` 供本机调试，客户端正式联调使用 PB。口令按 Argon2id 存储，并发认证预算为4。

帧为 `version:u8=1 | flags:u8 | msg_id:u16BE | request_seq:uvarint32 | binding_epoch:uvarint64 | [error_code:uvarint32] | protobuf`。binding_epoch 每帧都编码，未绑定时为0；flags.bit0 表示错误；每帧最大64KiB。一个 WebSocket 二进制消息就是一个帧。

每次新 WS 先经 account 取随机短期凭证（仅保存 SHA256，45秒有效、单次 nonce），发送20001。服务端20002给出新的 binding_epoch、scene_epoch、baseline_id 与经济下一序号。服务端紧接20002发送5完整首基线，Ready仍为false并拒绝动作和经济请求；客户端组装完整基线、资源与场景真实ready后才发送4 ACK。首次匹配scene/baseline的ACK激活场景并同步背包/钱包/托管状态与战报（可附当前快照），最后6 SyncReady才开放操作。重复ACK不重复激活、发送战报或SyncReady；开发ACK等待预算保持10秒。同一连接的重复入场不允许；健康重连复用原房间临时世界，并补发原死亡ID。

当前用5完整快照重复推送代替 delta；后续重复快照不需要 ACK。实体 kind 为1玩家、2宠物、3怪物、4已经原子自动拾取的短暂掉落实体。action 为0待机、1移动、2攻击、3受击、4死亡；direction 为0上、1右上、2右、3右下、4下、5左下、6左、7左上。地图坐标向右为+x、向下为+y。HP/MP、动作、位置、宠物、怪物及冷却只在内存。EntityState追加纯观察字段move_interval_ms（当前玩家600ms、宠物1000ms、稻草人1500ms）与attack_sequence；攻击序号只在实际受理攻击时递增，独立于可能被同tick受击覆盖的action，冷却或移动未结束的拒绝不递增。致死伤害立即推送HP0/Action4基线，永久奖励仍等待资产事务提交。

移动结束前不得发动攻击已按本地Crystal候选来源对齐：`资料参考/crystal/Server/MirObjects/HumanObject.cs:138-142,2494` 通过ActionTime约束走步与攻击；`MonsterObject.cs:650-665,2128-2131` 在走步时将AttackTime至少推到MoveTime，BoneFamiliar沿用基类。当前统一在NextMove之前拒绝攻击，等于步结束时可按原攻击冷却受理；玩家/宠物/怪物均有边界测试。本次未更改配置移动/攻击间隔，只对齐该候选时序，不宣称目标官方1.80已完成验收。

20006支持八方向摇杆与点地寻路，松摇杆不会清除既有点地目标；斜角移动与攻击均检查墙角。30004的monster_count=0保持原值，1..8切换数量，不通过切换重置槽位HP/CD。成功后立即以同一请求seq返回30005 HostingState，携带当前余额、auto状态和永久revision，随后立即推送当前基线，让死亡/暂停状态也能同步怪物数；不重建实体或刷新HP/CD。初始/经济同步的30005仍为seq0。满包拒绝开启时沿原seq返回错误，并以seq0补发真实托管状态；重连后由30005还原自动战斗开关，由基线怪物实体数量还原怪数。30003仅skill_id=0普攻开放。道士job2才有当前开发骷髅，战士不凭空获得宠物。技能耗蓝、完整职业技能与召唤资格仍待专项实现和验证；当前MP来自候选出生值，真实同步但普攻不耗蓝。

40001装备、40002分解、40006免费领时、30002复活均走原16字节 op_id、command_seq、expected_revision、expected_rule_version。每笔经济请求仅40005 OperationResult解锁对应 pending，40003/40004不能解锁。旧op相同载荷重试返回原业务结果和当前确认watermark；同op改变载荷拒绝。提交结果不明时保留原操作身份与原结果，后台有界重试，不生成新op或重抽掉落。40007金币补时尚缺正式Gref价格，明确拒绝且不扣币。

开发主动复活只在fixture明确许可时开放：DeathNotice.allowed_actions=1，ReviveChoice.action=1及原death_id；收到成功40005后恢复临时出生点HP/MP。不收费、不掉经验装备，仅为开发验证；死亡重连不回血，原复活op重放不会再次治疗。

## 已落地的权威与托管边界

永久资产只写 `minimir_dev_game`；集成测试仅用随机 `minimir_dev_game_test_<id>`，测试结束删除自己的库。迁移先检查本项目标记，拒绝认领未标记的既有数据库。单角色聚合保存四钱包、背包/装备、等级经验、领取额度、托管余额与已提交战报，不保存临时战场。

account 批准 game 所有权；15秒 Mongo 服务端时间租约、3秒独立控制池续租，续租失败立即隔离。资产事务使用 snapshot+majority，实际写账号授权guard与固定32组run guard，并校验owner/run_epoch/session_epoch及角色CAS；换代也实际写同组屏障。永久奖励采用顺序来源游标，同一来源仅提交一次；重要经济操作的收据与审计outbox在同一事务内。没有房间长期goroutine；8个固定场景worker、8个固定DB worker、有界队列、连接及房间预算。离线不构造场景和背包同步消息。

在线和离线使用同一网格战斗与逐击杀结算，普通怪独立10秒复活、互斥65/20/10/5掉落及20次装备保底。当前开发赠送两木剑、一布衣和红蓝各一用于测试；手动分解50/400绑定金币借用来源Price，仅是开发资产测试值。没有药品假消耗或未验证技能效果。背包满不擅自按Price自动卖物：记录未拾取物品/绑定与停止原因，停止新增离线托管；清理背包后可重新开启自动战斗。完整自动回收设置后续接入。

每日手动领取6小时；角色卡权益字段支持当天总12小时与只补差额，但本阶段没有付费开卡入口。在线、关闭自动战斗、死亡、时长耗尽、满包停止和维护时不扣离线时间。健康房间离线继续模拟，仅实际托管秒数扣时，DB等待暂停模拟与计费；无固定每秒收益或离线时长乘杀怪数。战报收益逐事件保留绑定。停止原因0为尚无停止事件、1时长耗尽、2死亡、3背包满。真实故障后临时场景不恢复，完成换代屏障后创建新世界；已提交永久事实保留。

## 验证

```bash
./tools/dev go test ./...
# 保存真实结果与源码/fixture摘要到server/test-results/latest.json
./tools/dev python3 tools/server/test.py --mongo --race
# 显式连接本任务副本集，真实HTTP+WS与Mongo资产事务，随机测试库结束清理
MINIMIR_MONGO_E2E=1 ./tools/dev go test ./internal/integration -count=1 -v -timeout=120s
```

集成测试覆盖：注册口令、最多5角色、建角op防重/冲突、单次票据、ACK前完整首基线/门槛与错ACK拒激活/重复ACK只激活一次、SyncReady、点地/松摇杆、原op装备分解、免费6小时、在线/暂停不扣时、实际离线扣时与战报、健康与死亡重连、实际杀怪升级/装备掉落、8怪调节、主动复活；独立存储测试验证有效owner拒抢、32组guard换代、旧owner/旧session写拒绝、奖励游标防重和满包处理。保底测试先把测试角色置于19次未掉装备的合法前态，再通过真实WS战斗验证第20次掉落，不声称已跑20次完整击杀回放。单测验证八方向和墙角阻挡。

仍未验收：完整技能、药耗/修理、卡支付、金币补时、市场/共享仓、多人挑战、目标1.80完整动作伤害、完整成长数值、跨进程真实故障注入及容量压测。开发单节点与本机可玩闭环不代表生产高可用。
