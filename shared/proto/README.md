# 共享协议实现状态

当前rule20为76个PB消息、58个消息号、31个错误码、176个64位字段、581个Long边界。Inventory附只读ProgressionState，新增40019观察原SourceSeq已提交的经验分账；HostingReport追加原经验、未接收经验与入池经验，原experience改为实际接收量。既有技能资产协议新增kind4同源书页合并语义，无新增字段或消息号，仍由原报价/操作身份提交。

自然成长与待用经验已通过rule20限定开发验收：1122项真实Mongo/WS全包Go race、120项配置Python、444项JS（428 UI/app、16场景；217份项目输入与另9份锁定引擎源码分别冻结）；Creator3.8.8实构126.98秒、exit36，10份保存布局不变。正常新道士原装备、原书、原稻草人奖励自然升至22级并跨过旧21级断点；独立明确前态另验50级有界池、满池金币物品继续、原Source回执、旧余额无损封存与旧高等级保留。40019只显示确认后的原奖励/实际接收/入池/拒收，战报和宠物训练使用实际新经验；升级只恢复原场景原出生的存活角色一次，迁移和健康新票不回血或重播。装备提交校验权威需求等级，Role复用保存的可滚动成长详情。kind4沿40011/12/13真实报价手动合并同源书页，保留来源/绑定/品质及总数，完整输入输出写原typed回执与审计；背包复用上下文按钮，10布局不变。当前预览`dev-level-growth-v0.20-ed0b55518991`；MYF经验目录仍为开发候选，不代表自然1至50周期或平衡通过；现有满包额外停战/托管仍须按4.2对齐，不据此认证完整容量策略。转生挑战/费用、完整装备内容、护符规格、真机、容量及完整P0仍待完成。见[阶段验收](../../docs/自然成长与待用经验阶段验收.json)。

以下记录保留各自历史阶段范围，当前以本节rule20与最新验收为准。

当前rule19为74个PB消息、57个消息号、31个错误码、162个64位字段、528个Long边界。新增40016策略/40017原件锁定两条永久请求及只读40018回收结果；操作沿原op与owner提交，未知不换身份，64位保持Long/BigInt。

自动回收与挂机战报已通过rule19限定开发验收：1022项真实Mongo/WS全包Go race、111项配置Python、425项JS（409 UI/app、16场景；212份项目输入与另9份锁定引擎源码分别冻结）；Creator3.8.8实构121.42秒、exit36，正常新角色触屏6项通过，0错误/1警告。策略默认关闭，设置和明确应用背包分开确认，锁定/穿戴/未确认结果/占用为强保护；在线和托管共用原SourceSeq资产事务，40018只显示已提交事实，健康重入不重播。当前预览`dev-auto-recycle-v0.19-65ab01055f34`；主仍22技能、legacy未训宠、护符关闭及17未开放。木剑50/布衣400仍为开发金币夹具，正式分解、损坏回收、洗炼石与祝福油产出未验收；完整P0、真机与容量继续待验。见[阶段验收](../../docs/自动回收与挂机战报阶段验收.json)。

以下记录保留各自历史阶段范围，当前状态以本节rule19与最新验收为准。

当前rule18共70个PB数据消息、54个消息号、148个64位字段及482个Long边界案例，仍属draft开发合同。新增纯S2C30015 PetBorn；14字段承载精确scene/baseline/state_sequence/handle/Life/definition/map/cast/skill/出生Unix/运动水位/方向/RoleKey/server elapsed。movement_revision=0合法；64位保持Long/BigInt，不上传坐标，不新增经济游标。

PetBorn仅在fresh17真正安装、原current World及成功SyncReady的完整World.Ref/Auth stamp仍匹配、含body完整基线全组发送后发出；非当前或未Ready不积压。旧附着quiescing仍可观察出生，输入门槛不开放；Cancel不重播，新Entry清scope。 客户端消费最多1条pending并保留Life水位；健康基线/Owned/永久回执不补播出生。显示发送失败不回滚宠物或材料，不重投新票。主仍22技能、原商店与初始赠书、legacy免费未训宠及护符关闭；独立17候选required_rule_version=17、runtime_enabled=false，护符包Count单位与售价仍待用户答复。见[阶段验收](../../docs/宠物出生表现与隔离客户端阶段验收.json)。

以下保留各自历史阶段范围；当前运行状态以上述rule18与最新限定验收为准。

rule18新增draft S2C30015 `PetBorn`：只描述17真正安装成功且当时当前World/原附件已成功发送SyncReady的新生。完整匹配身体的baseline全组先发送，再以scene/baseline/state_sequence、handle/Life、definition、map、原cast/skill、角色身份和运动水位配对。movement_revision=0合法。born_at_unix_ms记录真实Spawn处理时刻，elapsed_ms由服务端单调时钟计算，客户端不比较Unix时钟；溢出丢观察事件。Recall、legacy、失败、非当前图或未Ready新生均不积压，不由健康基线/Owned/永久回执补播。显示发送失败不回滚宠物/耗材，也不转投新连接。本条是协议实施契约，非正式验收或入口开放声明。

当前rule17共69个PB数据消息、53个消息号、31个错误码、140个64位字段及458个Long边界案例，无新消息号，仍属draft开发合同。SceneBaselineChunk.owned_pet=10新增OwnedPetState：原活宠handle/Life/definition、实际MapID、HP/上限、训练、recall_skill_ids及follow_suppressed；无落点、预生或尸体，不把异图实体加入entities。

全部分块及退出取消ACK须完全一致并完整组装；同图时交叉核对原实体，64位保持Long。客户端只读召回技能集合选择原CastIntent/材料RPC，服务端重新判断；观察过期则正式拒绝模式变化、不扣符、不伪造经济成功。永久回执仍只记原消耗事实，不能从历史回执排新Recall/新生。主仍22技能/材料关闭，协议能力不授予17或购符，见[阶段验收](../../docs/召唤与跨图宠物规则阶段验收.json)。以下保留历史版本。

当前rule16为68个PB数据消息、53个消息号、31个错误码、136个64位字段及444个Long边界案例，仍为draft开发合同，未新增消息号。`EntityState.pet_training=23`为独立训练等级/上限与uint64经验/门槛，`is_pet_corpse=24`识别无主尸体；模板Level不等于训练等级。主legacy宠不带虚构训练状态；训练EXP可超门槛且单次只升一级，客户端不while重算。

训练和尸体观察均属于同一完整scene/baseline/state_sequence/handle/Life。基线在真实64KiB帧限制内整组预编码分块，单体过大/超过合同则整组拒绝，不删尸体凑包；显式退出等待原request/binding/state完整分块取消确认。64位继续Long/BigInt，不新增经济水位、客户端造宠入口或17技能授权。见[阶段验收](../../docs/宠物尸体与训练引擎阶段验收.json)。以下保留各自历史版本范围。

当前rule15协议67个PB数据消息、53个消息号、31个错误码、134个64位字段及438个Long边界案例，仍属draft开发合同。新增纯S2C 30014 `TeleportResult`，含RoleKey、原/新scene、cast、Life、实际地图/落点/运动水位、outcome及transition_id；客户端不上传传送目的点。

`EnterLineResult.server_transition_id`与跨图成功结果严格配对，正常健康票为空且不补播历史。`EntityStatusState.teleport_mana_penalty_percent`为权威传送耗蓝加成，同图仍用30013；64位保持Long/BigInt。绑定沿20011观察已提交永久事实，原经济command/Pending合同保持。跨图资源/首视口、ACK、SyncReady逐项门禁，见[瞬移阶段验收](../../docs/瞬息移动与世界执行身份阶段验收.json)。

以下rule14及更早协议段落保留历史范围，当前以rule15和唯一protocol_manifest为准。

当前rule14协议共66个PB数据消息、52个消息号、31个错误码、128个64位字段及420个Long边界案例，均为draft开发合同。新增20011 `BindingState`及`RoleBindingPoint`只观察已提交永久绑定：RoleKey、scene、role_revision、原绑定receipt的source_seq、可空中心、cause、原安全区ID和trusted。cause1静默恢复，cause2仅已提交新事实；64位保持Long/BigInt，不新增设置绑定坐标的C2S入口。

绑定成功先发送同一Role的完整Inventory，再发BindingState。客户端经济操作继续以Inventory.revision和原command构造，绑定不改Pending或经济水位。来源实时安全区观察仍用baseline.safe_zone_id，与绑定证明来源区分开。20007–20010沿rule13同owner旅行合同保持；21仍关闭。见[绑定阶段验收](../../docs/安全区永久绑定阶段验收.json)。

以下rule9等旧协议描述为历史范围，当前以上述rule14及唯一protocol_manifest为准。

普通移动与动作时钟修复保持当前协议及rule9配置，211项JS/359个PB边界和289条真实Mongo/WS race另行通过；未新增位移消息或开放位移技能。见[本轮验收](../../docs/普通移动与动作时钟阶段验收.json)。

本目录是双端共用协议源。rule9阶段已验收基线包含58个PB数据消息、46个消息号、31个错误码及108个Long字段，边界案例359个；另有3个后置保留消息。已接本地账号、票据/基线、网格输入、永久资产回执、手动药品、owner学习、技能获取升级与19项候选技能及3项防御状态。护符运行入口仍disabled，14/15正向材料路径只作隔离测试；完整职业、双市场、正式兼容与发布仍未完成。

`EntityState.mp/mp_max`继续使用9/10字段号；`uint32 move_interval_ms=11`和`uint32 attack_sequence=12`是纯观察数据，支撑显示步进时长与攻击播放触发，不改变客户端输入权威。首组技能阶段增加观察字段13 `cast_sequence`和14 `cast_skill_id`，独立施法身份用于动作播放。64位字段继续使用Long/BigInt。

药品阶段新增40009 `UseItemRequest`：字段1为原物品实例bytes，2为物品定义ID，3为uint64场景代次，4为`OperationContext`；复用40005 `OperationResult`确认永久消耗，不另建客户端回血权威。新增3101当前状态不能用药、3102使用间隔未到，仍为draft开发编号。客户端按库存顺序选原实例，服务端校验归属、数量、原场景、状态与共享节流；owner扣1提交后只向合法原场景加入分跳效果，同一操作或迟到回执不能重复消费、重复加药效或作用到另一个世界。

首组技能阶段新增40010 `LearnSkillRequest`（原书实例、技能ID、OperationContext），复用40005永久操作回执；库存新增`LearnedSkill`列表，记录已学0级并以存在性区分未学。30003 `CastIntent`沿用并由技能ID表达首批技能意图；新增30006 `CastAccepted`携带场景代次、施法ID、起止格、方向、延迟和输入序号，仅描述已受理的临时施法，重连基线不重播旧特效。3201–3206分别表示未学、状态、冷却、资源、目标及输入过期，均为draft开发编号。移动与施法共用输入水位，服务端从权威目标位置判断射程、阻挡与命中。

获取与升级阶段新增40011/40012 `SkillAssetPreviewRequest/SkillAssetPreview`，涵盖商品、逐阶升级与合法书分解；预览明确具体输入实例、版本、数量、来源/品质/绑定、钱包分摊、输出和升级前后数值。40013 `SkillAssetCommitRequest`只携带预览ID及OperationContext，复用40005永久回执；服务端重建原计划并比对，禁止提交时换料或换币种。升级与分解必须显式列材料，空材料拒绝；3301–3305为预览过期、材料、升级、书来源与购买条件错误，均为draft开发编号。

上一范围技能阶段扩展30003 CastIntent的target_grid区域落点，并新增30007 AttackAccepted和30008 CombatImpact：前者只描述受理时的普通攻击/延迟，后者携带到期事件ID、目标生命代次、格、实际量及结果；客户端不据受理事件提前扣血或结算。施法受理、命中和既有基线各有独立身份。健康断线不清已释放事件，旧生命或场景不能承接旧定向命中。 target_grid允许地图内阻挡中心，射程仍受限；逐目标有效格过滤不变，不新增区域CanFly要求。

阶段B历史新增30009 `SkillModeIntent`、30010 `SkillModeState`、30011 `AttackIntent`和30012 `CastStep`。模式意图只表达开关/充能，AttackIntent只表达朝向及可选预期技能；两者共用鉴权连接和InputSequence。全量模式含scene、player life、revision、发送时剩余时间，基线与健康重连恢复当前状态；同连接成功复活同时递增life与模式revision，客户端不因旧修订拒收复活后的模式。所有64位身份与修订仍用Long/BigInt，倒计时不能自行授予ready。

`EntityState`新增17/18攻击skill/rank；`AttackAccepted`新增实际skill/rank和caster life，`CastAccepted`新增rank。刀光只匹配本连接收到的受理事件与同次身体帧，基线不补旧刀光。地狱火rank0–3单线4格为用户审定本作例外，非Crystal rank3形状全等；首格500ms，后格以实际处理时钟+100ms推进。30012只在实际处理有效格时发送，空有效格也发，阻挡格不发；客户端按scene/caster life/cast/step防重，健康重入只接未来步，不按源客户端50ms视觉间隔预排业务。

rule9新增`EntityState.statuses=19`与`combat_attributes=20`，承载当前生命的完整状态列表和实际战斗属性。`EntityStatusState`的施加者life及各属性64位值保持Long/BigInt；剩余时间为0时状态仍可存在，客户端只按后续权威空列表或列表差异清除，不凭倒计时撤盾或重算伤害。健康重入恢复当前常驻状态，不补播旧起手；状态和HP/MP/CD不写入永久角色快照。

rule9新增40014 `CastWithMaterialRequest`和40015 `SetCastingSupplyRequest`，复用40005永久回执。材料施法携带原scene/life、输入序号、技能、目标、原材料实例与版本及OperationContext；独立槽100装卸同样核对原实例/版本。owner先原子扣除Count与版本并记录回执/审计，再在合法原现场一次应用临时施法；历史回执先于当前材料门禁核对，不能重抽随机计划或重复支付。手动使用经济序号，自动使用独立来源流水；迟到回调不释放新代槽，也不把旧request投给新连接。3207/3208是材料未开放或不足、实例版本变化的draft开发错误码。

运行配置`casting_supply.enabled=false`，护符包单位和报价未确认，未创建护符NPC商品；14/15装卸与施放入口均关闭。独立随机测试实例的正数单位只证明协议和事务候选路径，不表示玩家购符施法闭环已开放。真实Mongo提交未知故障未注入，Unknown/Pending同job回调竞争只作单元回归。抗拒、野蛮及瞬移仍在来源审计与政策待定阶段，本协议统计不代表已实现这些位移技能。

技能报价预览用独立有界代次槽，I/O不借用永久Pending；当前连接/Auth/Binding/scene/代次失效后不向新连接投递旧报价，也不释放后来代槽。并发资产变更使原revision失效时，提交按原计划明确拒绝。

`ItemView`新增的来源、来源ID及品质仅展示服务端永久记录，不授权客户端写来源；NPC、初始赠书和legacy不得拆页，合规掉落书输出逐项保留绑定。永久`operations.created_at`已从TTL迁成普通索引，原操作回执先于预览查找/过期检查返回；只有每角色一个可替换的`skill_quotes.expires_at`预览及原有票据保留TTL。迁移不恢复已被旧TTL删除的历史。

当前rule9限定证据为207项JS（16场景、191项UI/应用）、33项Python、359个PB边界与276条真实Mongo/WS全包race测试及子测试（0跳过）；独立.NET的586向量只证明已提取公式子集。Creator3.8.8 Web Mobile最终实构187.1秒、exit36，源码稳定，10份scene/prefab构建前后保持；首次新增Badge导入规范化触发的摘要保护失败另行保留。本机浏览器15张有效截图、188条输入/6条表现记录、新增warn/error为0，31购书学习与状态、14/15入口关闭、响应布局和健康重入已观察。详细范围见[防御技能阶段验收](../../docs/战斗状态与防御技能阶段验收.json)；不据此认证完整P0、目标1.80、手机真机、容量或正式发布。

统计勘误：v0.9归档`outputs/development-combat-status-20261028/client-tests.log`的207已含16项场景（第183行起），原223是重复相加。正确合计207=16场景+191项UI/应用；原日志不改，本轮普通移动时钟修复另行记录。

阶段B的175项JS/25项Python/288个PB边界、172条race与314.29秒构建是[战士模式与方向技能阶段](../../docs/战士攻击模式与方向技能阶段验收.json)的历史限定证据。此前范围技能阶段153项JS/23项Python/258个PB边界、96条race与116.84秒构建，以及d1207bc的64条race/139.33秒、421ce1c的118项JS/54条race/144.12秒，均保留历史身份。

当前入场顺序为20002入场响应后先发送完整基线，此时连接Ready仍为false；客户端组装匹配基线后提交ACK，服务端仅在首次匹配ACK时激活并发送SyncReady，重复ACK忽略。首个基线不等待ACK，以免双方互等。客户端有界、乱序和过期基线组装已有测试覆盖。

消息号和错误码唯一源为 `protocol_manifest.json`。架构8.2明确的示例编号保留；新增编号仍标记`draft`，实现不等于正式发布。外层帧校验`binding_epoch`，世界消息另校验`scene_epoch`。当前本地开发传输为HTTP/PB与WS；架构目标中的HTTPS/TCP/WSS及正式证书配置继续按发布阶段验收。

```bash
./tools/dev python3 tools/config/generate_protocol.py --check
make protocol
make test
```

服务端常量在 `server/internal/protocol/`，真实Go PB在 `server/api/generated/minimirv1/`；客户端常量在 `client/assets/scripts/generated/`，真实静态PB JS/TS声明在其 `protobuf/` 子目录。生成器使用已安装并锁定的protoc36.2、protoc-gen-go1.36.12和protobufjs-cli2.7.1。工具及来源证据在 `conf/toolchain/`。

PB生成与静态边界统计分别记录在`conf/toolchain/`；`client-pb-verification.json`实际记录58条消息、108个64位字段、359个Long边界案例及bytes序列化检查，`go-pb-generation.json`记录对应源和生成文件摘要。线格式正确不等于鉴权、永久资产、完整技能或真机验收；各项业务和构建证据单列于阶段报告。

当前快照`dev-combat-status-v0.9-48873786ee49`、rule_version9；98表922记录（candidate423/confirmed393/historical36/unverified70），24张必需空表及668项正式阻断。19技能、57升级、21商品与76条书品质分解来自唯一Excel，profile只选skill_ids，初始只赠首组对应职业一本书；`runtime_ready=false`、`production_ready=false`。3008张来源PNG中原2914张完整保留，新增94帧不等于14/15本机正向表现验收。协议不授予自动喝药、未实现技能或正式玩法能力。v0.6a的98表897记录、26空表/637阻断及16/48/18/64投影属于阶段B历史。

历史首循环为62项JS、11项配置回归与174个PB边界案例，HUD阶段为87项JS；药品阶段为99项JS、16项配置回归、177个PB边界和39条真实Mongo+WS全包race测试。移动攻击互锁、致死HP0即时基线、显示插值与动作预载，以及旧角色离线31秒、击杀2、经验30和剩余21569秒均保留为对应阶段实证，见[首循环浏览器记录](../../docs/客户端浏览器验收.json)及[HUD记录](../../docs/主界面阶段验收.json)。历史数量与样本不代表当前完整P0、官方1.80、真机或容量验收；当前正式门禁以上文rule9统计为准，新增候选未认证为成熟配置。

经济操作的 `op_id`、`command_seq`、预期资产版本和报价版本共同用于服务端核验；客户端提交金额、绑定或中奖结果不构成权威。输入消息只表达意图。元宝字段名使用 `yuanbao_units`，显示1元宝=内部10，适配架构中的百分精度示例。

普通个人实例使用同一场景协议，禁止客户端通过填入room/line/角色ID自行创建世界或取得权限。普通地图不启用AOI，完整基线与增量按个人实例观察连接下发。公开线路、野外PK、公会、红名、面对面交易不由本协议准备自动加入首发范围。


首组技能阶段（421ce1c）的历史构建与关键路径见`../../docs/技能阶段验收.json`。三职学习扣原书、初学0级、火球与治愈表现/耗蓝已核对；带追踪复测的一次请求对应一次受理，连续两点的第二次被动作锁拒绝。Auto已改用内置TOGGLE事件并通过真实引擎生命周期回归。首次无追踪时的状态变化尚未归因，保留观察，不把未复现写成已根治；当前首组材料升级已实现，完整技能与升级范围、手机、真实网络提交未知故障及平衡仍未验收。
