# 客户端

本目录是Creator3.8.8工程，保存客户端脚本、依赖、Boot/Login/RoleSelect/Game四个scene及Entity/ListItem/Effect三个prefab。首循环、移动修复、HUD、药品及首组技能分别有历史浏览器证据。当前已实现技能商店、合规书拆页和首组三技能0–3阶升级的显式原实例预览/提交；自动检查与真实构建通过，限定范围本机浏览器验收已完成，结果见`../docs/技能获取与升级阶段验收.json`。完整职业技能、聊天、发布、真机与全P0尚未完成。

开发前先读 [客户端界面约定](AGENTS.md) 和 [场景管理与加载器](docs/场景管理与加载器.md)。固定布局、EditBox默认组件树、PopupView分层返回、触屏输入与导航状态保持均按约定逐项验收。

主界面采用用户2026-10-26上传的[布局参考图](../UI参考图/用户确认/主界面UI布局参考.png)，分区、固定布局和适配规则见[主界面布局基准](docs/主界面布局基准.md)。上阶段保存顶部128、底部344的HUD；同宽720下由1280加高至1600时，仅地图视口从808增至1128，控件、文字与图标不随之拉高。

上阶段制作30张1.80来源原像素UI图，接入职业头像、四钱包详情、小地图与真实坐标、药品数量、血蓝与经验、常用功能菜单。GameHUD负责数据绑定，GameMessages负责当前会话的真实系统记录；消息支持折叠两条、展开搜索和复用List，内置EditBox用于搜索，尚无聊天发送或世界聊天服务。历史浏览器与多尺寸证据见[主界面阶段验收](../docs/主界面阶段验收.json)。

既有红蓝快捷入口已接入手动用药：资源未满时直接提交，满资源时使用PopupView确认仍会消费；按当前库存顺序选择原药品实例，确认后仍校验原角色、场景、实例和版本。服务端权威扣1后分跳恢复，客户端根据真实库存与HP/MP更新显示。没有自动喝药；首批三项技能以候选范围开放，其他技能、完整职业升级范围、BOSS和任务继续保留未完成状态，具体来源与调度边界见[药品与技能实施边界](../docs/药品与技能实施边界.md)。

首批技能面板与条目模板保存在Game.scene，GameSkills只绑定已有节点；SkillModel负责已学0级、原书实例和目标预检，服务端决定学习结果、耗蓝与命中。火球、治愈的施法动作及效果经预载播放；Effect.prefab仅作为动态效果模板。基本剑术初学准确+0，当前通过对应书本和书页逐阶升级，随机练功仍关闭。首组施法阶段的关键路径与未归因观察保留在历史技能验收。

本轮保留原HUD733对象；技能列表y从-35改为-20、高从470改为360，viewport/mask/content等4处明确调整，为已保存的书店、升级、拆书3按钮留空间。Game.scene相对HEAD从779到938对象是有意编辑；构建layout_preserved仅表示构建前后未被工具覆盖。

获取与升级界面继续绑定Game.scene保存的面板和模板。GameSkillAssets展示服务端报价、具体原材料实例/数量/版本、真实币种分摊与前后属性；SkillAssetsModel只组织意图，服务端40013回执与库存决定最终扣费及rank。NPC/赠书/legacy不能拆页，合规掉落书的输出继承各自绑定。当前限定范围浏览器已核对学习、购书、拆页、升级、列表与取消保留及重登持久，结果见本阶段报告；过期与并发等自动用例仍按各自证据记录。

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
| `assets/resources/development/` | 2017张来源候选帧、16×16地图、动作/效果manifest和公开物品/职业/技能映射 |
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

在根目录执行`make client-scene-test`、`make client-test`、`make client-build`分别检查场景加载器、UI/应用逻辑和真实Web Mobile构建。当前自动检查通过：134项JS（16场景、118项UI/应用）、20项Python配置回归、Go测试与227个PB64位边界案例，协议47个PB数据消息及69个Long字段；真实cc声明严格检查通过。首版Creator构建175.31秒、退出36、源码稳定且7份资产构建前后未被覆盖；List修复后的最终make test退出0，Creator139.33秒、exit36、源码稳定，7份资产构建前后未被覆盖。首组技能421ce1c的118项JS/54条Mongo race/144.12秒构建，以及药品99项/HUD87项/首循环62项均属历史证据。

`make dev-start`启动同源客户端`http://127.0.0.1:18080`和本地开发服务，`make dev-status`/`make dev-stop`查看和停止。上阶段浏览器已实证登录/创建道士、入场与真实资源、自动战斗、导航意图保留、两层弹窗滚动、装备回收、复活及免费托管，详见[推进记录](../docs/正式开发推进记录.md)。

首循环历史离线重登实证为托管31秒、2次击杀、经验30，免费托管余额21600→21569。正式`runtime_ready`和`production_ready`仍为false，26张必需空表及520项正式门禁仍保留。手动红蓝药使用跨来源development候选，首批三技能仅为开发子集，完整职业技能和升级仍待核验，50绑定金币回收仍仅为开发夹具测试奖励。当前配置为`dev-skill-growth-v0.4-d9101bd94921`、开发规则版本4。基础参数来自Excel“技能基础定义”三项，profile仅选skill_ids，逐级扩展9行，运行技能商店5项与分解投影12条均由唯一配置表导出。

首循环尺寸历史记录采用顶部118、底部268；HUD阶段改为128/344并复验加高与加宽。历史报告分别保留对应源码和构建，不替代当前技能阶段的浏览器结果或手机真机验证。

针对用户反馈的实体瞬移、闪烁和序列帧问题，入场取票前先预载当前职业所需动作全集：首循环历史为战士/法师各776张、道士1000张；技能阶段另预载本职业施法及效果帧，不重复载入另一性别或同种怪物的多份纹理。实体Sprite使用`trim=false`并保留原PNG尺寸和WIL偏移。客户端仅对显示位置做浮点插值，镜头跟随显示位置，walking相位跟随步进度，独立one-shot时钟控制攻击、受击和死亡；权威格坐标及源MoveMS/AttackMS数值不由视图改写。

表现层用`playbackUntil`保护已开始的攻击/受击，后续显示移动在单次动画结束后衔接；HP0释放旧门槛，完成当前已知落脚步后播放完整die并停在末帧，不延迟业务HP或奖励。协议的`move_interval_ms=11`和`attack_sequence=12`仅供观察与表现。移动攻击互锁与致死即时基线属于既有验证，当前真实Mongo+WS全包`-race`报告为64条pass、0跳过、源码稳定，见[服务端报告](../server/test-results/latest.json)。

上阶段表现修复用新角色完成浏览器回归：入场预载1000帧，三类实体有109/172/124个连续中间格样本，初始5秒换帧118次；后续窗口帧间隔mean20.27ms、max39.3ms，新warn/error为0。它证明当时本机IAB样本中的插值与换帧已运行，不作为真机性能或60FPS承诺。完整阶段边界与回归证据见[推进记录](../docs/正式开发推进记录.md)。

首组技能阶段（421ce1c）的历史构建与关键路径见`../docs/技能阶段验收.json`。三职学习扣原书、初学0级、火球与治愈表现/耗蓝已核对；带追踪复测的一次请求对应一次受理，连续两点的第二次被动作锁拒绝。Auto已改用内置TOGGLE事件并通过真实引擎生命周期回归。首次无追踪时的状态变化尚未归因，保留观察，不把未复现写成已根治；当前首组材料升级已实现，完整技能与升级、手机、网络提交未知故障及平衡仍待验收。
