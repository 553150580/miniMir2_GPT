# 共享协议实现状态

本目录是双端共用协议源。四份proto当前生成38个PB数据消息，消息清单包含33个消息号、18个错误码和3个后置保留消息，客户端有55个Long字段。当前已实现本地注册/登录、建角与目录、票据入场、基线ACK/SyncReady、网格输入、首循环资产结果和手动用药；完整职业技能与两个市场协议继续按阶段补齐。

`EntityState.mp/mp_max`继续使用9/10字段号；`uint32 move_interval_ms=11`和`uint32 attack_sequence=12`是纯观察数据，支撑显示步进时长与攻击播放触发，不改变客户端输入权威。64位字段继续使用Long/BigInt。

本阶段新增40009 `UseItemRequest`：字段1为原物品实例bytes，2为物品定义ID，3为uint64场景代次，4为`OperationContext`；复用40005 `OperationResult`确认永久消耗，不另建客户端回血权威。新增3101当前状态不能用药、3102使用间隔未到，仍为draft开发编号。客户端按库存顺序选原实例，服务端校验归属、数量、原场景、状态与共享节流；owner扣1提交后只向合法原场景加入分跳效果，同一操作或迟到回执不能重复消费、重复加药效或作用到另一个世界。

本阶段实现与自动检查已通过：真实Mongo+WS全包`-race`集成39条pass、0跳过且源码稳定，见[服务端报告](../../server/test-results/latest.json)；Creator实际构建160.81秒、退出36，源码稳定且6份布局哈希不变。本机浏览器验收已通过，结果见[药品阶段验收](../../docs/药品阶段验收.json)。首循环与HUD的历史浏览器验收保持有效，但不替代当前结果；正式`runtime_ready`、`production_ready`和手机真机验收仍为false。

当前入场顺序为20002入场响应后先发送完整基线，此时连接Ready仍为false；客户端组装匹配基线后提交ACK，服务端仅在首次匹配ACK时激活并发送SyncReady，重复ACK忽略。首个基线不等待ACK，以免双方互等。客户端有界、乱序和过期基线组装已有测试覆盖。

消息号和错误码唯一源为 `protocol_manifest.json`。架构8.2明确的示例编号保留；新增编号仍标记`draft`，实现不等于正式发布。外层帧校验`binding_epoch`，世界消息另校验`scene_epoch`。当前本地开发传输为HTTP/PB与WS；架构目标中的HTTPS/TCP/WSS及正式证书配置继续按发布阶段验收。

```bash
./tools/dev python3 tools/config/generate_protocol.py --check
make protocol
make test
```

服务端常量在 `server/internal/protocol/`，真实Go PB在 `server/api/generated/minimirv1/`；客户端常量在 `client/assets/scripts/generated/`，真实静态PB JS/TS声明在其 `protobuf/` 子目录。生成器使用已安装并锁定的protoc36.2、protoc-gen-go1.36.12和protobufjs-cli2.7.1。工具及来源证据在 `conf/toolchain/`。

PB语法/引用、Go编译、真实cc声明类型检查、177个Long64位边界案例及bytes序列化检查已执行。Creator工程真实构建证据见[构建检查](../../docs/客户端构建检查.json)。编解码报告只证明线格式，鉴权、分发与资产提交的当前证据单独由服务端集成报告记录；正式协议兼容、完整业务、真机和容量仍待验收。

当前`make test`整体exit0，包含99项JS（16场景、83项UI/应用）、16项配置回归、Go测试和177个PB64位边界案例；真实cc声明严格检查通过。新JS覆盖PotionModel9项、未决操作2项及传输1项。药品恢复参数来自唯一Excel的开发夹具，协议本身不授予自动喝药或完整技能能力。

历史首循环为62项JS、11项配置回归与174个PB边界案例，HUD阶段为87项JS。移动攻击互锁、致死HP0即时基线、显示插值与动作预载，以及旧角色离线31秒、击杀2、经验30和剩余21569秒均保留为对应阶段实证，见[首循环浏览器记录](../../docs/客户端浏览器验收.json)及[HUD记录](../../docs/主界面阶段验收.json)。历史数量与样本不代表当前完整P0、官方1.80、真机或容量验收；27张必需空表及492项正式门禁仍在。

经济操作的 `op_id`、`command_seq`、预期资产版本和报价版本共同用于服务端核验；客户端提交金额、绑定或中奖结果不构成权威。输入消息只表达意图。元宝字段名使用 `yuanbao_units`，显示1元宝=内部10，适配架构中的百分精度示例。

普通个人实例使用同一场景协议，禁止客户端通过填入room/line/角色ID自行创建世界或取得权限。普通地图不启用AOI，完整基线与增量按个人实例观察连接下发。公开线路、野外PK、公会、红名、面对面交易不由本协议准备自动加入首发范围。

