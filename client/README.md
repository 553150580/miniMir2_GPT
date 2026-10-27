# 客户端

本目录是已建立的Creator3.8.8工程，保存客户端脚本、运行依赖和可编辑资产。Boot/Login/RoleSelect/Game四个scene及Entity/ListItem两个prefab已落盘。上阶段首开发闭环及移动修复通过本机IAB浏览器回归，`development_loop_ready=true`；本阶段主界面已实现，最终验收见[主界面阶段验收](../docs/主界面阶段验收.json)，构建记录见[构建检查](../docs/客户端构建检查.json)。完整聊天、正式发布、手机真机和全P0尚未完成。

开发前先读 [客户端界面约定](AGENTS.md) 和 [场景管理与加载器](docs/场景管理与加载器.md)。固定布局、EditBox默认组件树、PopupView分层返回、触屏输入与导航状态保持均按约定逐项验收。

主界面采用用户2026-10-26上传的[布局参考图](../UI参考图/用户确认/主界面UI布局参考.png)，分区、固定布局和适配规则见[主界面布局基准](docs/主界面布局基准.md)。本阶段保存顶部128、底部344的HUD；同宽720下由1280加高至1600时，仅地图视口从808增至1128，控件、文字与图标不随之拉高。

本阶段制作30张1.80来源原像素UI图，接入职业头像、四钱包详情、小地图与真实坐标、药品数量、血蓝与经验、常用功能菜单。GameHUD负责数据绑定，GameMessages负责当前会话的真实系统记录；消息支持折叠两条、展开搜索和复用List，内置EditBox用于搜索，尚无聊天发送或世界聊天服务。药品消费、完整技能、BOSS和任务等未开放入口说明当前状态。上述为本阶段实现，浏览器和多尺寸验证以阶段验收记录为准。

| 路径 | 职责 |
| --- | --- |
| `assets/scripts/generated/protobuf/` | 从`../shared/proto/`生成的静态PB JavaScript与TypeScript声明 |
| `assets/scripts/generated/protocol_ids.ts` | 从共享协议清单生成的客户端消息号与错误码 |
| `assets/scripts/generated/config_types.ts` | 配置导出生成的客户端类型，由公共配置工具同步 |
| `assets/scripts/core/scene/` | 场景管理加载器基础脚本；对接真实Creator场景和加载视图 |
| `assets/scripts/app/GameHUD.ts`、`GameMessages.ts` | 绑定已保存的HUD与系统消息；纯数据规则分别由HudModel、SystemMessageLog承担 |
| `assets/scenes/`、`assets/resources/prefabs/` | 编辑器可编辑的四个场景与Entity/ListItem预制体 |
| `assets/resources/development/` | 1593张来源候选帧、16×16地图、动作manifest和公开物品/职业映射 |
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

在根目录执行`make client-scene-test`、`make client-test`、`make client-build`，分别检查场景加载器、UI与应用逻辑，以及真实Web Mobile构建。本阶段自动检查已通过：87项JS（16场景、71项UI/应用）、`make test`、`make check`及真实cc声明严格检查；Web Mobile实际构建退出36，源码稳定且6份布局哈希不变。官方声明自身缺失由`skipLibCheck`隔离，不表示关闭项目脚本检查。浏览器交互和四种尺寸复验已通过，最终见阶段验收记录；上阶段62项JS仅为历史数量。

`make dev-start`启动同源客户端`http://127.0.0.1:18080`和本地开发服务，`make dev-status`/`make dev-stop`查看和停止。上阶段浏览器已实证登录/创建道士、入场与真实资源、自动战斗、导航意图保留、两层弹窗滚动、装备回收、复活及免费托管，详见[推进记录](../docs/正式开发推进记录.md)。

上阶段离线重登实证为托管31秒、2次击杀、经验30，免费托管余额21600→21569。正式`runtime_ready`和`production_ready`仍为false，27张必需空表及492项正式门禁不变。药品消费和完整职业技能仍未开放，资源/数值均为development候选，50绑定金币回收仅为开发夹具测试奖励。

上阶段尺寸历史记录采用顶部118、底部268，已验证加高仅增加地图视口、加宽增加地图可见范围，以及无新快照时镜头重夹。本阶段改为128/344，须以新的阶段验收为准；历史结果不替代当前布局或手机真机验证。

针对用户反馈的实体瞬移、闪烁和序列帧问题，入场取票前先预载当前职业所需动作全集：战士/法师各776张，道士1000张，不重复载入另一性别或同种怪物的多份纹理。实体Sprite使用`trim=false`并保留原PNG尺寸和WIL偏移。客户端仅对显示位置做浮点插值，镜头跟随显示位置，walking相位跟随步进度，独立one-shot时钟控制攻击、受击和死亡；权威格坐标及源MoveMS/AttackMS数值不由视图改写。

表现层用`playbackUntil`保护已开始的攻击/受击，后续显示移动在单次动画结束后衔接；HP0释放旧门槛，完成当前已知落脚步后播放完整die并停在末帧，不延迟业务HP或奖励。协议新增的`move_interval_ms=11`和`attack_sequence=12`仅供观察与表现。服务器已对齐移动未完不能攻击，并在致死时立即发HP0基线，真实`-race`共11条测试（含子用例）通过，报告源码与当前文件匹配。

上阶段表现修复用新角色完成浏览器回归：入场预载1000帧，三类实体有109/172/124个连续中间格样本，初始5秒换帧118次；后续窗口帧间隔mean20.27ms、max39.3ms，新warn/error为0。它证明当时本机IAB样本中的插值与换帧已运行，不作为真机性能或60FPS承诺。完整阶段边界与回归证据见[推进记录](../docs/正式开发推进记录.md)。
