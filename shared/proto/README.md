# 共享协议实现状态

本目录是双端共用协议源。当前54个PB数据消息、44个消息号、29个错误码及89个Long字段，边界案例288个。已实现本地账号、票据/基线、网格输入、永久资产回执、手动药品、owner学习、技能获取升级与16项候选技能；完整职业、双市场、正式兼容与发布仍未完成。

`EntityState.mp/mp_max`继续使用9/10字段号；`uint32 move_interval_ms=11`和`uint32 attack_sequence=12`是纯观察数据，支撑显示步进时长与攻击播放触发，不改变客户端输入权威。首组技能阶段增加观察字段13 `cast_sequence`和14 `cast_skill_id`，独立施法身份用于动作播放。64位字段继续使用Long/BigInt。

药品阶段新增40009 `UseItemRequest`：字段1为原物品实例bytes，2为物品定义ID，3为uint64场景代次，4为`OperationContext`；复用40005 `OperationResult`确认永久消耗，不另建客户端回血权威。新增3101当前状态不能用药、3102使用间隔未到，仍为draft开发编号。客户端按库存顺序选原实例，服务端校验归属、数量、原场景、状态与共享节流；owner扣1提交后只向合法原场景加入分跳效果，同一操作或迟到回执不能重复消费、重复加药效或作用到另一个世界。

首组技能阶段新增40010 `LearnSkillRequest`（原书实例、技能ID、OperationContext），复用40005永久操作回执；库存新增`LearnedSkill`列表，记录已学0级并以存在性区分未学。30003 `CastIntent`沿用并由技能ID表达首批技能意图；新增30006 `CastAccepted`携带场景代次、施法ID、起止格、方向、延迟和输入序号，仅描述已受理的临时施法，重连基线不重播旧特效。3201–3206分别表示未学、状态、冷却、资源、目标及输入过期，均为draft开发编号。移动与施法共用输入水位，服务端从权威目标位置判断射程、阻挡与命中。

获取与升级阶段新增40011/40012 `SkillAssetPreviewRequest/SkillAssetPreview`，涵盖商品、逐阶升级与合法书分解；预览明确具体输入实例、版本、数量、来源/品质/绑定、钱包分摊、输出和升级前后数值。40013 `SkillAssetCommitRequest`只携带预览ID及OperationContext，复用40005永久回执；服务端重建原计划并比对，禁止提交时换料或换币种。升级与分解必须显式列材料，空材料拒绝；3301–3305为预览过期、材料、升级、书来源与购买条件错误，均为draft开发编号。

上一范围技能阶段扩展30003 CastIntent的target_grid区域落点，并新增30007 AttackAccepted和30008 CombatImpact：前者只描述受理时的普通攻击/延迟，后者携带到期事件ID、目标生命代次、格、实际量及结果；客户端不据受理事件提前扣血或结算。施法受理、命中和既有基线各有独立身份。健康断线不清已释放事件，旧生命或场景不能承接旧定向命中。 target_grid允许地图内阻挡中心，射程仍受限；逐目标有效格过滤不变，不新增区域CanFly要求。

本阶段新增30009 `SkillModeIntent`、30010 `SkillModeState`、30011 `AttackIntent`和30012 `CastStep`。模式意图只表达开关/充能，AttackIntent只表达朝向及可选预期技能；两者共用鉴权连接和InputSequence。全量模式含scene、player life、revision、发送时剩余时间，基线与健康重连恢复当前状态；同连接成功复活同时递增life与模式revision，客户端不因旧修订拒收复活后的模式。所有64位身份与修订仍用Long/BigInt，倒计时不能自行授予ready。

`EntityState`新增17/18攻击skill/rank；`AttackAccepted`新增实际skill/rank和caster life，`CastAccepted`新增rank。刀光只匹配本连接收到的受理事件与同次身体帧，基线不补旧刀光。地狱火rank0–3单线4格为用户审定本作例外，非Crystal rank3形状全等；首格500ms，后格以实际处理时钟+100ms推进。30012只在实际处理有效格时发送，空有效格也发，阻挡格不发；客户端按scene/caster life/cast/step防重，健康重入只接未来步，不按源客户端50ms视觉间隔预排业务。

技能报价预览用独立有界代次槽，I/O不借用永久Pending；当前连接/Auth/Binding/scene/代次失效后不向新连接投递旧报价，也不释放后来代槽。并发资产变更使原revision失效时，提交按原计划明确拒绝。

`ItemView`新增的来源、来源ID及品质仅展示服务端永久记录，不授权客户端写来源；NPC、初始赠书和legacy不得拆页，合规掉落书输出逐项保留绑定。永久`operations.created_at`已从TTL迁成普通索引，原操作回执先于预览查找/过期检查返回；只有每角色一个可替换的`skill_quotes.expires_at`预览及原有票据保留TTL。迁移不恢复已被旧TTL删除的历史。

当前阶段最终自动、复活修复后Mongo全包race与浏览器验收正在收尾，结论以`../../docs/战士攻击模式与方向技能阶段验收.json`为准，不沿用历史测试数量。Creator3.8.8 Web Mobile已完成314.29秒真实构建，exit36、源码稳定，7份scene/prefab在构建前后保持。上一范围技能阶段153项JS/23项Python/258个PB边界、96条race与116.84秒构建属于`范围技能阶段验收.json`的历史限定证据；d1207bc的64条race/139.33秒及421ce1c的118项JS/54条race/144.12秒仍保留历史身份。

当前入场顺序为20002入场响应后先发送完整基线，此时连接Ready仍为false；客户端组装匹配基线后提交ACK，服务端仅在首次匹配ACK时激活并发送SyncReady，重复ACK忽略。首个基线不等待ACK，以免双方互等。客户端有界、乱序和过期基线组装已有测试覆盖。

消息号和错误码唯一源为 `protocol_manifest.json`。架构8.2明确的示例编号保留；新增编号仍标记`draft`，实现不等于正式发布。外层帧校验`binding_epoch`，世界消息另校验`scene_epoch`。当前本地开发传输为HTTP/PB与WS；架构目标中的HTTPS/TCP/WSS及正式证书配置继续按发布阶段验收。

```bash
./tools/dev python3 tools/config/generate_protocol.py --check
make protocol
make test
```

服务端常量在 `server/internal/protocol/`，真实Go PB在 `server/api/generated/minimirv1/`；客户端常量在 `client/assets/scripts/generated/`，真实静态PB JS/TS声明在其 `protobuf/` 子目录。生成器使用已安装并锁定的protoc36.2、protoc-gen-go1.36.12和protobufjs-cli2.7.1。工具及来源证据在 `conf/toolchain/`。

PB生成与静态边界统计分别记录在`conf/toolchain/`，当前包含288个Long边界案例及bytes序列化检查。线格式正确不等于鉴权、永久资产、完整技能或真机验收；本阶段最终结果单列于阶段报告。

当前快照`dev-combat-modes-v0.6a-015eb33fec33`、rule_version6；98表897记录（candidate399/confirmed393/historical36/unverified69），26张必需空表及637项正式阻断。16技能、48升级、18商品与64分解投影来自唯一Excel，profile只选skill_ids，初始只赠首组3本职业书；`runtime_ready=false`、`production_ready=false`。协议不授予自动喝药、未实现技能或正式玩法能力。

历史首循环为62项JS、11项配置回归与174个PB边界案例，HUD阶段为87项JS；药品阶段为99项JS、16项配置回归、177个PB边界和39条真实Mongo+WS全包race测试。移动攻击互锁、致死HP0即时基线、显示插值与动作预载，以及旧角色离线31秒、击杀2、经验30和剩余21569秒均保留为对应阶段实证，见[首循环浏览器记录](../../docs/客户端浏览器验收.json)及[HUD记录](../../docs/主界面阶段验收.json)。历史数量与样本不代表当前完整P0、官方1.80、真机或容量验收；当前26张必需空表及637项正式门禁仍在，新增候选未认证为成熟配置。

经济操作的 `op_id`、`command_seq`、预期资产版本和报价版本共同用于服务端核验；客户端提交金额、绑定或中奖结果不构成权威。输入消息只表达意图。元宝字段名使用 `yuanbao_units`，显示1元宝=内部10，适配架构中的百分精度示例。

普通个人实例使用同一场景协议，禁止客户端通过填入room/line/角色ID自行创建世界或取得权限。普通地图不启用AOI，完整基线与增量按个人实例观察连接下发。公开线路、野外PK、公会、红名、面对面交易不由本协议准备自动加入首发范围。


首组技能阶段（421ce1c）的历史构建与关键路径见`../../docs/技能阶段验收.json`。三职学习扣原书、初学0级、火球与治愈表现/耗蓝已核对；带追踪复测的一次请求对应一次受理，连续两点的第二次被动作锁拒绝。Auto已改用内置TOGGLE事件并通过真实引擎生命周期回归。首次无追踪时的状态变化尚未归因，保留观察，不把未复现写成已根治；当前首组材料升级已实现，完整技能与升级范围、手机、真实网络提交未知故障及平衡仍未验收。
