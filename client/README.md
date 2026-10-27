# 客户端

本目录是Creator3.8.8工程，保存Boot/Login/RoleSelect/Game四个scene及Entity/ListItem/Effect三个prefab。首循环、移动修复、HUD、药品、首组三职业技能及技能获取升级保留各自限定范围验收。本阶段已实现精神力战法、大火球、雷电术、爆裂火焰、群体治疗术，总计8项开发候选技能，并补动作时钟、延迟命中、逐杀结算与异步报价隔离。自动检查、Creator真实构建及96条真实Mongo/WS race通过；法师与道士的限定范围本机浏览器验收已完成；本阶段开发候选通过，完整职业与正式验收仍保留门禁。完整职业技能、聊天、正式发布、真机与全P0仍未完成。

开发前先读 [客户端界面约定](AGENTS.md) 和 [场景管理与加载器](docs/场景管理与加载器.md)。固定布局、EditBox默认组件树、PopupView分层返回、触屏输入与导航状态保持均按约定逐项验收。

主界面采用用户2026-10-26上传的[布局参考图](../UI参考图/用户确认/主界面UI布局参考.png)，分区、固定布局和适配规则见[主界面布局基准](docs/主界面布局基准.md)。上阶段保存顶部128、底部344的HUD；同宽720下由1280加高至1600时，仅地图视口从808增至1128，控件、文字与图标不随之拉高。

上阶段制作30张1.80来源原像素UI图，接入职业头像、四钱包详情、小地图与真实坐标、药品数量、血蓝与经验、常用功能菜单。GameHUD负责数据绑定，GameMessages负责当前会话的真实系统记录；消息支持折叠两条、展开搜索和复用List，内置EditBox用于搜索，尚无聊天发送或世界聊天服务。历史浏览器与多尺寸证据见[主界面阶段验收](../docs/主界面阶段验收.json)。

既有红蓝快捷入口已接入手动用药：资源未满时直接提交，满资源时使用PopupView确认仍会消费；按当前库存顺序选择原药品实例，确认后仍校验原角色、场景、实例和版本。服务端权威扣1后分跳恢复，客户端根据真实库存与HP/MP更新显示。没有自动喝药；当前8项技能以候选范围开放，其余技能、完整职业升级范围、BOSS和任务继续保留未完成状态，具体来源与调度边界见[药品与技能实施边界](../docs/药品与技能实施边界.md)。

技能面板与条目模板保存在Game.scene，GameSkills只绑定已有节点，SkillModel只做意图与目标预检；服务端决定学习、MP、命中与永久结果。当前8项开发候选包括首组火球/治愈/基本剑术与精神力/大火球/雷电/爆裂/群疗；支持0–3阶材料升级，随机练功关闭。地面范围技能传逻辑格，AttackAccepted与CombatImpact分开驱动受理动作和到期表现；保留原帧偏移与地图裁剪，不以视觉范围决定命中。新增技能法师与道士限定范围浏览器验收已完成，见[实施边界](../docs/范围技能与战斗时序实施边界.md)。 区域瞄准中心只检边界与射程，阻挡中心不整次拒绝；服务端到期逐目标筛有效格。

上一获取升级阶段（d1207bc）保留原HUD733对象；技能列表y从-35改为-20、高从470改为360，viewport/mask/content等4处明确调整，为已保存的书店、升级、拆书3按钮留空间。Game.scene从779到938对象是当时有意编辑；构建layout_preserved仅表示构建前后未被工具覆盖。

获取与升级界面继续绑定已保存的面板和模板，展示服务端报价、精确材料、来源、绑定、币种及前后数值。当前8技能/24升级/10商店/32分解投影；初始只赠首组职业书，旧3书500金币，新5书为600/2000/1600/3000/15000金币候选。40013与实际库存决定扣费和rank，NPC/赠书/legacy禁拆。d1207bc已验证首组限定范围获取升级；本阶段真实Mongo/WS race96条已通过，新增技能法师与道士限定范围浏览器已完成，详情见阶段报告。 本阶段已真实核对精神力0→3的2/3/4书与5/10/20页、准确0/3/5/8、重入状态；群疗浏览器只证明玩家受击中的净HP变化与满血宠结果，受伤宠双目标另由真实集成覆盖。

| 路径 | 职责 |
| --- | --- |
| `assets/scripts/generated/protobuf/` | 从`../shared/proto/`生成的静态PB JavaScript与TypeScript声明 |
| `assets/scripts/generated/protocol_ids.ts` | 从共享协议清单生成的客户端消息号与错误码 |
| `assets/scripts/generated/config_types.ts` | 配置导出生成的客户端类型，由公共配置工具同步 |
| `assets/scripts/core/scene/` | 场景管理加载器基础脚本；对接真实Creator场景和加载视图 |
| `assets/scripts/app/GameHUD.ts`、`GameMessages.ts` | 绑定已保存的HUD与系统消息；纯数据规则分别由HudModel、SystemMessageLog承担 |
| `assets/scripts/app/GameSkills.ts`、`SkillModel.ts` | 绑定已保存技能面板、原实例学习预览及合法目标预检，已学0级不等同未学 |
| `assets/scripts/app/GameSkillAssets.ts`、`SkillAssetsModel.ts` | 绑定已保存的商店、拆页与升级入口，提交服务端预览及确切材料，不预测永久结果 |
| `assets/scripts/app/CastPlayback.ts`、`CombatEffects.ts` | 消费服务端施法受理事件，播放预载的动作与动态效果，不决定业务命中 |
| `assets/scripts/app/PotionModel.ts` | 手动用药预览与原实例校验；不预测扣包或回血，满资源才请求确认 |
| `assets/scenes/`、`assets/resources/prefabs/` | 编辑器可编辑的四个场景与Entity/ListItem/Effect预制体 |
| `assets/resources/development/` | 2181张来源候选帧、16×16地图、动作/效果manifest和公开物品/职业/技能映射 |
| `assets/resources/ui180/` | 30张主界面原像素图、Creator资源meta与来源帧映射 |
| `package.json`、`package-lock.json` | 固定运行依赖`protobufjs@8.8.1`与`long@5.3.2`，不包含CLI |

在本目录执行：

```sh
npm ci --ignore-scripts --no-audit --no-fund
npm run protobuf:generate
npm run protobuf:verify
```

`protobufjs-cli@2.7.1`安装在用户级工具目录`~/.local/share/minimir-toolchain/node-tools/`，由项目工具调用。`MINIMIR_NODE_TOOLS`可显式指定该目录。客户端使用自己的`node_modules`解析运行依赖，验证过程不借用外部CLI的`NODE_PATH`。

64位协议字段使用`Long`，从十进制字符串或精确高低32位构造；禁止经不安全的JavaScript Number转换。编解码边界检查说明见生成目录内README，安装与验证证据见`../conf/toolchain/node-tools.lock.json`和`client-pb-verification.json`。

从Creator3.8.8打开本目录可编辑已保存的场景和预制体。固定UI层级不由运行时脚本重新创建，生成工具不得覆盖布局；`AGENTS.md`始终是本目录的当前约定。

在根目录执行`make client-scene-test`、`make client-test`、`make client-build`分别检查加载器、UI/应用和真实构建。当前`make test`完整退出0：153项JS（16场景、137项UI/应用）、23项Python配置回归、Go测试及258个PB64位边界通过；协议40个消息号、29个错误码、49个PB数据消息、79个Long字段。Creator3.8.8 Web Mobile最终实构116.84秒、exit36成功，全部脚本与7份scene/prefab哈希当前一致，7份布局构建前后未被覆盖。 当前真实Mongo/WS全包race为96条pass、0跳过，源码稳定；法师与道士限定范围本机浏览器已通过，新增warn/error为0；不扩大为完整技能、P0、正式或真机验收。本阶段报告为`../docs/范围技能阶段验收.json`，各项结论按限定范围分别记录。 获取升级d1207bc的134项JS/20项Python/64条Mongo race/139.33秒，以及首组421ce1c的118/54/144.12均保留历史范围。

`make dev-start`启动同源客户端`http://127.0.0.1:18080`和本地开发服务，`make dev-status`/`make dev-stop`查看和停止。上阶段浏览器已实证登录/创建道士、入场与真实资源、自动战斗、导航意图保留、两层弹窗滚动、装备回收、复活及免费托管，详见[推进记录](../docs/正式开发推进记录.md)。

首循环历史离线重登实证为托管31秒、2次击杀、经验30，余额21600→21569。当前快照`dev-combat-area-v0.5-ddfb248d01b3`、rule_version5；98表841记录（candidate343/confirmed393/historical36/unverified69），26张必需空表及565项正式阻断。新增候选不表示正式配置成熟，`runtime_ready=false`、`production_ready=false`。 当前8技能基础、24升级、10商品及32分解投影从唯一Excel导出；profile只选skill_ids。50绑定金币回收与免费复活仍是开发夹具，完整职业、自然渠道、网络未知提交故障与真机不据此通过。

首循环尺寸历史记录采用顶部118、底部268；HUD阶段改为128/344并复验加高与加宽。历史报告分别保留对应源码和构建，不替代当前技能阶段的浏览器结果或手机真机验证。

针对用户反馈的实体瞬移、闪烁和序列帧问题，入场取票前先预载当前职业所需动作全集：首循环历史为战士/法师各776张、道士1000张；技能阶段另预载本职业施法及效果帧，不重复载入另一性别或同种怪物的多份纹理。实体Sprite使用`trim=false`并保留原PNG尺寸和WIL偏移。客户端仅对显示位置做浮点插值，镜头跟随显示位置，walking相位跟随步进度，独立one-shot时钟控制攻击、受击和死亡；权威格坐标及源MoveMS/AttackMS数值不由视图改写。

表现层用`playbackUntil`保护已开始的攻击/受击，后续显示移动在单次动画结束后衔接；HP0释放旧门槛，完成当前已知落脚步后播放完整die并停在末帧，不延迟业务HP或奖励。协议的`move_interval_ms=11`和`attack_sequence=12`仅供观察与表现。移动攻击互锁与致死即时基线属于既有验证，64条Mongo+WS全包race为d1207bc历史结果；本阶段到期时刻与托管边界补测后真实Mongo/WS race96条通过，不沿用64条作为当前数量。

上阶段表现修复用新角色完成浏览器回归：入场预载1000帧，三类实体有109/172/124个连续中间格样本，初始5秒换帧118次；后续窗口帧间隔mean20.27ms、max39.3ms，新warn/error为0。它证明当时本机IAB样本中的插值与换帧已运行，不作为真机性能或60FPS承诺。完整阶段边界与回归证据见[推进记录](../docs/正式开发推进记录.md)。

首组技能阶段（421ce1c）的历史构建与关键路径见`../docs/技能阶段验收.json`。三职学习扣原书、初学0级、火球与治愈表现/耗蓝已核对；带追踪复测的一次请求对应一次受理，连续两点的第二次被动作锁拒绝。Auto已改用内置TOGGLE事件并通过真实引擎生命周期回归。首次无追踪时的状态变化尚未归因，保留观察，不把未复现写成已根治；当前首组材料升级已实现，完整技能与升级、手机、网络提交未知故障及平衡仍待验收。
