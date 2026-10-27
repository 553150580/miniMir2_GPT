# 共享协议实现状态

本目录是双端共用协议源。当前49个PB数据消息、40个消息号、29个错误码及79个Long字段。已实现本地账号、票据/基线、网格输入、永久资产回执、手动药品、owner学习、技能获取升级与8项候选技能；完整职业、双市场、正式兼容与发布仍未完成。

`EntityState.mp/mp_max`继续使用9/10字段号；`uint32 move_interval_ms=11`和`uint32 attack_sequence=12`是纯观察数据，支撑显示步进时长与攻击播放触发，不改变客户端输入权威。首组技能阶段增加观察字段13 `cast_sequence`和14 `cast_skill_id`，独立施法身份用于动作播放。64位字段继续使用Long/BigInt。

药品阶段新增40009 `UseItemRequest`：字段1为原物品实例bytes，2为物品定义ID，3为uint64场景代次，4为`OperationContext`；复用40005 `OperationResult`确认永久消耗，不另建客户端回血权威。新增3101当前状态不能用药、3102使用间隔未到，仍为draft开发编号。客户端按库存顺序选原实例，服务端校验归属、数量、原场景、状态与共享节流；owner扣1提交后只向合法原场景加入分跳效果，同一操作或迟到回执不能重复消费、重复加药效或作用到另一个世界。

首组技能阶段新增40010 `LearnSkillRequest`（原书实例、技能ID、OperationContext），复用40005永久操作回执；库存新增`LearnedSkill`列表，记录已学0级并以存在性区分未学。30003 `CastIntent`沿用并由技能ID表达首批技能意图；新增30006 `CastAccepted`携带场景代次、施法ID、起止格、方向、延迟和输入序号，仅描述已受理的临时施法，重连基线不重播旧特效。3201–3206分别表示未学、状态、冷却、资源、目标及输入过期，均为draft开发编号。移动与施法共用输入水位，服务端从权威目标位置判断射程、阻挡与命中。

获取与升级阶段新增40011/40012 `SkillAssetPreviewRequest/SkillAssetPreview`，涵盖商品、逐阶升级与合法书分解；预览明确具体输入实例、版本、数量、来源/品质/绑定、钱包分摊、输出和升级前后数值。40013 `SkillAssetCommitRequest`只携带预览ID及OperationContext，复用40005永久回执；服务端重建原计划并比对，禁止提交时换料或换币种。升级与分解必须显式列材料，空材料拒绝；3301–3305为预览过期、材料、升级、书来源与购买条件错误，均为draft开发编号。

本阶段30003 CastIntent使用target_grid表达区域落点，新增30007 AttackAccepted和30008 CombatImpact：前者只描述受理时的普通攻击/延迟，后者携带到期事件ID、目标生命代次、格、实际量及结果；客户端不据受理事件提前扣血或结算。施法受理、命中和既有基线各有独立身份。健康断线不清已释放事件，旧生命或场景不能承接旧定向命中。 target_grid允许地图内阻挡中心，射程仍受限；逐目标有效格过滤不变，不新增区域CanFly要求。

技能报价预览用独立有界代次槽，I/O不借用永久Pending；当前连接/Auth/Binding/scene/代次失效后不向新连接投递旧报价，也不释放后来代槽。并发资产变更使原revision失效时，提交按原计划明确拒绝。

`ItemView`新增的来源、来源ID及品质仅展示服务端永久记录，不授权客户端写来源；NPC、初始赠书和legacy不得拆页，合规掉落书输出逐项保留绑定。永久`operations.created_at`已从TTL迁成普通索引，原操作回执先于预览查找/过期检查返回；只有每角色一个可替换的`skill_quotes.expires_at`预览及原有票据保留TTL。迁移不恢复已被旧TTL删除的历史。

当前`make test`完整退出0：153项JS（16场景、137项UI/应用）、23项Python配置回归、Go测试及258个PB64位边界通过；协议40个消息号、29个错误码、49个PB数据消息、79个Long字段。Creator3.8.8 Web Mobile最终实构116.84秒、exit36成功，全部脚本与7份scene/prefab哈希当前一致，7份布局构建前后未被覆盖。 当前真实Mongo/WS全包race为96条pass、0跳过，源码稳定；法师与道士限定范围本机浏览器已通过，新增warn/error为0；不扩大为完整技能、P0、正式或真机验收。本阶段报告为`../../docs/范围技能阶段验收.json`，各项结论按限定范围分别记录。 d1207bc获取升级阶段64条Mongo race及139.33秒构建为历史限定证据；421ce1c的118项JS/54条race/144.12秒与未归因Auto观察继续保留。

当前入场顺序为20002入场响应后先发送完整基线，此时连接Ready仍为false；客户端组装匹配基线后提交ACK，服务端仅在首次匹配ACK时激活并发送SyncReady，重复ACK忽略。首个基线不等待ACK，以免双方互等。客户端有界、乱序和过期基线组装已有测试覆盖。

消息号和错误码唯一源为 `protocol_manifest.json`。架构8.2明确的示例编号保留；新增编号仍标记`draft`，实现不等于正式发布。外层帧校验`binding_epoch`，世界消息另校验`scene_epoch`。当前本地开发传输为HTTP/PB与WS；架构目标中的HTTPS/TCP/WSS及正式证书配置继续按发布阶段验收。

```bash
./tools/dev python3 tools/config/generate_protocol.py --check
make protocol
make test
```

服务端常量在 `server/internal/protocol/`，真实Go PB在 `server/api/generated/minimirv1/`；客户端常量在 `client/assets/scripts/generated/`，真实静态PB JS/TS声明在其 `protobuf/` 子目录。生成器使用已安装并锁定的protoc36.2、protoc-gen-go1.36.12和protobufjs-cli2.7.1。工具及来源证据在 `conf/toolchain/`。

PB语法/引用、Go编译、真实cc声明严格检查、258个Long64位边界及bytes序列化检查已执行。线格式验证不等于鉴权、永久资产提交或完整技能验收；本阶段真实Mongo/WS race96条已通过，限定范围本机浏览器已通过，完整玩法/真机仍待独立验收。

当前make test完整退出0，153项JS、23项Python及258个PB边界通过。当前快照`dev-combat-area-v0.5-ddfb248d01b3`、rule_version5；98表841记录（candidate343/confirmed393/historical36/unverified69），26张必需空表及565项正式阻断。新增候选不表示正式配置成熟，`runtime_ready=false`、`production_ready=false`。 基础8项、24升级、10商品与32分解投影各来自唯一配置表，profile只选skill_ids；协议不授予自动喝药、未实现技能或正式玩法能力。

历史首循环为62项JS、11项配置回归与174个PB边界案例，HUD阶段为87项JS；药品阶段为99项JS、16项配置回归、177个PB边界和39条真实Mongo+WS全包race测试。移动攻击互锁、致死HP0即时基线、显示插值与动作预载，以及旧角色离线31秒、击杀2、经验30和剩余21569秒均保留为对应阶段实证，见[首循环浏览器记录](../../docs/客户端浏览器验收.json)及[HUD记录](../../docs/主界面阶段验收.json)。历史数量与样本不代表当前完整P0、官方1.80、真机或容量验收；26张必需空表及565项正式门禁仍在，新增候选未认证为成熟配置。

经济操作的 `op_id`、`command_seq`、预期资产版本和报价版本共同用于服务端核验；客户端提交金额、绑定或中奖结果不构成权威。输入消息只表达意图。元宝字段名使用 `yuanbao_units`，显示1元宝=内部10，适配架构中的百分精度示例。

普通个人实例使用同一场景协议，禁止客户端通过填入room/line/角色ID自行创建世界或取得权限。普通地图不启用AOI，完整基线与增量按个人实例观察连接下发。公开线路、野外PK、公会、红名、面对面交易不由本协议准备自动加入首发范围。


首组技能阶段（421ce1c）的历史构建与关键路径见`../../docs/技能阶段验收.json`。三职学习扣原书、初学0级、火球与治愈表现/耗蓝已核对；带追踪复测的一次请求对应一次受理，连续两点的第二次被动作锁拒绝。Auto已改用内置TOGGLE事件并通过真实引擎生命周期回归。首次无追踪时的状态变化尚未归因，保留观察，不把未复现写成已根治；当前首组材料升级已实现，完整技能与升级范围、手机、真实网络提交未知故障及平衡仍未验收。
