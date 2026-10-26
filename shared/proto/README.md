# 共享协议实现状态

本目录是双端共用的首个开发闭环协议源，四份proto已生成37个PB数据消息，消息清单包含32个消息号、16个错误码和3个后置保留消息。当前已实现本地注册/登录、建角与目录、票据入场、基线ACK/SyncReady、网格输入和首循环资产结果；完整技能与两个市场协议继续按阶段补齐。

`EntityState.mp/mp_max`继续使用9/10字段号；新增`uint32 move_interval_ms=11`和`uint32 attack_sequence=12`作为纯观察数据，分别支撑显示步进时长与攻击播放触发，不改变客户端输入权威。PB消息数量仍为37、消息号32、Long字段54；64位数据继续使用Long/BigInt，源配置与资源数值未改。注册请求与SyncReady已接入实际处理链。真实Mongo+PB HTTP+WS的`-race`共11条测试（含子用例）通过，报告源码与当前文件匹配，见[服务端报告](../../server/test-results/latest.json)。最新真实Creator构建与本机IAB浏览器回归已通过，本轮`development_loop_ready=true`；正式`runtime_ready`、`production_ready`和手机真机验收仍为false。

当前入场顺序为20002入场响应后先发送完整基线，此时连接Ready仍为false；客户端组装匹配基线后提交ACK，服务端仅在首次匹配ACK时激活并发送SyncReady，重复ACK忽略。首个基线不等待ACK，以免双方互等。客户端有界、乱序和过期基线组装已有测试覆盖。

消息号和错误码唯一源为 `protocol_manifest.json`。架构8.2明确的示例编号保留；新增编号仍标记`draft`，实现不等于正式发布。外层帧校验`binding_epoch`，世界消息另校验`scene_epoch`。当前本地开发传输为HTTP/PB与WS；架构目标中的HTTPS/TCP/WSS及正式证书配置继续按发布阶段验收。

```bash
./tools/dev python3 tools/config/generate_protocol.py --check
make protocol
make test
```

服务端常量在 `server/internal/protocol/`，真实Go PB在 `server/api/generated/minimirv1/`；客户端常量在 `client/assets/scripts/generated/`，真实静态PB JS/TS声明在其 `protobuf/` 子目录。生成器使用已安装并锁定的protoc36.2、protoc-gen-go1.36.12和protobufjs-cli2.7.1。工具及来源证据在 `conf/toolchain/`。

PB语法/引用、Go编译、真实cc声明类型检查、174个Long64位边界案例及bytes序列化检查已执行。Creator工程真实构建证据见[构建检查](../../docs/客户端构建检查.json)。编解码报告只证明线格式，鉴权、分发与资产提交的当前证据单独由服务端集成报告记录；正式协议兼容、完整业务、真机和容量仍待验收。

最新`make test`整体exit0，包含62项JS测试（16场景、9UI、6传输/视口/基线、8未决经济操作、14运动、9动画）、11项配置回归、Go测试及174个PB64位边界案例，真实cc声明严格检查也已通过。服务器已对齐移动未完不能攻击，致死HP0即时基线已通过集成与浏览器验证；客户端使用预载、独立表现时钟和浮点显示插值，不延迟业务HP或奖励。旧角色离线31秒、2次击杀、经验30与剩余21569秒的战报已实证，最新动画修复另以新角色完成入场及动作回归。本轮闭环通过不等于完整P0、官方1.80、完整HUD美术、真机或容量验收，当前仍有27张必需空表和492项正式门禁。

经济操作的 `op_id`、`command_seq`、预期资产版本和报价版本共同用于服务端核验；客户端提交金额、绑定或中奖结果不构成权威。输入消息只表达意图。元宝字段名使用 `yuanbao_units`，显示1元宝=内部10，适配架构中的百分精度示例。

普通个人实例使用同一场景协议，禁止客户端通过填入room/line/角色ID自行创建世界或取得权限。普通地图不启用AOI，完整基线与增量按个人实例观察连接下发。公开线路、野外PK、公会、红名、面对面交易不由本协议准备自动加入首发范围。

