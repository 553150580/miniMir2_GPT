# 传奇2竖屏挂机版开发入口

抗拒火环8与野蛮冲撞27已通过rule11限定开发验收：403条真实Mongo/WS race（0跳过）、242项客户端、41项配置Python、4项Windows主机、377个PB边界和6400组独立来源对拍通过。Creator3.8.8实构80.56秒，10份保存布局不变；两技能正常购书、原书学习及触摸施放5项流程和通用UI6项流程通过。21项技能仍属开发候选，新两项自动关闭，瞬息移动21待绑定点业务；完整P0、真机、容量与目标1.80完整认证未完成。见[本阶段验收](docs/抗拒火环与野蛮冲撞阶段验收.json)。

以下rule10及更早统计保留为历史范围，当前状态以上述rule11验收为准。

Windows工具链与rule10权威位移、导航观察基础已通过限定开发验收：379条真实Mongo/WS race（0跳过）、240项客户端、41项配置Python、4项Windows主机、377个PB边界及3892组来源原语对拍通过。Creator3.8.8实构171.48秒、10份保存布局不变；本机触屏6项流程通过。Windows使用用户明确授权的专属Mongo8.0.34新副本集，业务时限未放宽。19项技能范围保持，8/27/21未开放；完整P0、真机、容量、目标1.80完整认证及发布仍未完成。见[阶段验收](docs/权威位移与导航观察阶段验收.json)与[Windows适配](docs/Windows工具链适配与当前验证.md)。

普通移动与动作时钟修复已完成限定验收：怪物/宝宝按各自资源六帧600毫秒落脚，1000/2000毫秒AI发步节奏保持；普通walk显式推进攻击时钟，不再让移动节流充当攻击共享锁。211项JS、33项Python、359个PB边界与289条真实Mongo/WS race通过，Creator实构193.42秒且10份保存资产未变；本机6张截图、25条输入/2条表现记录，新增warn/error为0。四周4小物、中央开阔及头顶布局保持。见[修复阶段验收](docs/普通移动与动作时钟阶段验收.json)和[实施边界](docs/普通移动与动作时钟修复边界.md)。

上一战斗状态与三项防御技能阶段已完成限定开发验收。快照`dev-combat-status-v0.9-48873786ee49`、rule_version9；19项技能、57条升级、21项商店及76条书品质分解投影来自两本唯一Excel。魔法盾31已在本机GUI正常购书、学习、施放并查看状态详情；护符仍为disabled，幽灵盾14与神圣战甲术15的材料路径仅由隔离测试验证，不表示玩家购符施法闭环已开放。完整P0、目标1.80完整认证、手机真机、容量及正式发布仍未完成。最终范围见[阶段验收](docs/战斗状态与防御技能阶段验收.json)和[浏览器证据](outputs/development-combat-status-20261028/本机浏览器验收.json)，实施约束见[防御技能边界](docs/战斗状态与防御技能实施边界.md)。

v0.8的17×17菱形开阔地图已完成独立限定本机验收，64×32地格、48像素土层、场边4件小物、4阻挡及285可走连通保持；名字Label24、保存缩放0.7，与血条稳定置于各形象头顶。历史证据见[地图阶段验收](docs/17x17菱形地图阶段验收.json)，当时配置为rule9；该rule10统计保留为历史，当前以rule11验收为准。玩法依据《最终策划案.md》，技术参考《架构方案.md》。

## 从这里开始

1. 阅读 [架构适配与规则优先级](docs/架构适配与规则优先级.md)，了解普通个人场景、托管、市场、仓库和故障恢复的适配。
2. 阅读 [资料总览](资料索引/资料总览.md) 与 [资源转换与技能核验](docs/资源转换与技能核验.md)，定位上级目录中已存在的源码和资源。
3. 按 [开发阶段与验收清单](docs/开发阶段与验收清单.md) 推进工程、所有权、账号、战斗和经济闭环。
4. 在 `outputs/development-prep-20261026/` 维护玩法与架构两个配置工作簿，使用 [配置维护说明](config/README.md) 校验和导出。
5. 环境启动字段参考 [conf](conf/README.md)，数据库约束参考 [数据库集合与索引契约](docs/数据库集合与索引契约.json)，协议准备状态见 [共享协议](shared/proto/README.md)。
6. 主界面以用户2026-10-26上传的[布局参考图](UI参考图/用户确认/主界面UI布局参考.png)为当前依据；分区和玩法适配见[主界面布局基准](client/docs/主界面布局基准.md)。

## 工程目录

```text
server/          Go服务端工程、go.mod/go.sum、服务端生成代码
client/          Creator客户端工程目录、客户端依赖与生成代码
shared/proto/    双端共用Protobuf与消息/错误码清单
config/          配置schema、检查报告与不可变双端预览
outputs/         唯一维护的Excel配置表
docs/            架构适配、开发任务、验收、数据库与环境说明
tools/           统一工具入口、生成器、资源转换与检查
资料索引/         来源、摘要和映射
资料参考/         选定参考源码与数据的独立副本
UI参考图/         原样参考图片
```

## 本地检查

在本项目目录执行，`tools/dev`选择已经安装并锁定的较新工具。Go命令自动以server为工作目录。macOS保留以下Make入口：

```bash
make environment
make check
make test
make protocol
make preview
make client-test
make client-build
make server-integration-test
make development-resources-check
```

Windows使用同一个Python分发器的PowerShell入口，不依赖系统旧Go/Python或全局PATH。首次引导会核验官方归档摘要，并安装到`%LOCALAPPDATA%/minimir-toolchain/windows-amd64`；Creator固定3.8.8，现有WinLibs GCC16.1.0按路径/摘要锁定用于race编译；Lima/QEMU仅保留可选诊断安装。身份和凭据不写入仓库。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/environment/bootstrap_windows.ps1 -InitializePython -BrowserTools
./tools/dev.ps1 python3 tools/environment/check.py --strict --skip-database
./tools/dev.ps1 task check
./tools/dev.ps1 task test
./tools/dev.ps1 task client-build
./tools/dev.ps1 task client-typecheck
./tools/dev.ps1 task development-resources-check
```

`--skip-database`只核验工具，不表示数据库或业务已通过。Windows平台锁在`conf/toolchain/windows-amd64.lock.json`，原有Mac锁保留其历史用途。Creator安装位置可用`MINIMIR_CREATOR_APP`指定；只读原客户端根目录可用`MINIMIR_CLIENT_180_ROOT`指定，资源检查仍须匹配原摘要。Windows来源检查核验项目内独立副本及可访问原文件，不冒称重新扫描Mac原始来源树。

Windows专属数据库通过`tools/environment/mongodb_dev.py init/start/status/stop`管理。用户已明确选择Windows Mongo8.0.34新副本集，固定127.0.0.1:27119、minimirWinDev和本任务独立数据目录；不接入已有数据库，不注册开机服务。事务及正常停止重启通过，完整Mongo/WS race379条通过、0跳过；历史软件QEMU失败另存，单项业务时限保持。

本次Windows迁移的命令、已通过范围和数据库阻断见[适配记录](docs/Windows工具链适配与当前验证.md)及[检查状态](outputs/windows-adaptation-20261028/current-status.json)。当前客户端240项测试、Creator实构、379条Mongo/WS race及限定浏览器触屏流程通过；rule10基础已完成限定验收，完整技能与发布门禁保留。

Windows使用`./tools/dev.ps1 task dev-start/dev-status/dev-stop`。macOS启动本地开发服务使用`make dev-start`，查看与停止使用`make dev-status`、`make dev-stop`。客户端同源入口为`http://127.0.0.1:18080`，account/game/gm分别监听18100/18101/18102；开发工具管理进程，数据库由独立Mongo开发环境管理。

`--check` 默认检查数据结构。预览配置中的 `runtime_ready=false` 表示还存在待核验内容或业务验收。正式门禁单独执行：

```bash
make release-check
```

历史报告分别保留：首组421ce1c为118项JS/54条Mongo race/144.12秒构建和未归因Auto观察；获取升级d1207bc为134项JS/20项Python/64条race/139.33秒；范围技能v5为153项JS/23项Python/96条race/116.84秒，均是当时限定证据。阶段B历史快照`dev-combat-modes-v0.6a-015eb33fec33`、rule_version6；98表897记录（candidate399/confirmed393/historical36/unverified69），26张必需空表及637项正式阻断。新增候选不表示正式配置成熟，`runtime_ready=false`、`production_ready=false`。

## 资料与配置边界

- 上级参考目录只读检索，转换和开发产物写在本项目中；不运行来源程序或部署脚本。
- 参考工程的技能、掉落和数值是候选来源，目标1.80归属逐条核验。
- 初次提取记录位于 `config/catalog/fragments/`，后续参数只在Excel中维护。`config/schema/catalog.json` 负责字段、导出目标和发布门槛，不是第二份数值表。
- 客户端只接收公共/展示字段；服务端控制字段、来源和历史经济样本不下发。
- 代码、注释和说明使用UTF-8；运营日界为Asia/Shanghai；金额按最终策划的整数精度表示。

## 阶段提交与推送

远程为`https://github.com/553150580/miniMir2_GPT.git`。每阶段完成对应检查后提交并推送，再核对远程分支；不强推覆盖已有历史。GitHub CLI2.103.2已从官方发行包独立安装、双重核对摘要，通过`./tools/dev gh`调用。首次认证由用户完成，可用`./tools/dev gh auth login --hostname github.com --git-protocol https --web`，不把密码或Token交给聊天或写入仓库。命令行不继承系统代理时，读取当前系统代理并临时使用。

## 当前开发状态

- 技能：21项开发候选、63条0–3阶升级、23项商店及84条书品质分解，初始仅一本对应职业首组书。8/27按用户审定距离逐阶变化，手动施放；21未开放，下一步先建角色绑定点。14/15护符入口因包单位和报价待定继续关闭。
- 配置：`dev-displacement-skills-v0.11b-88c1414ae0f8`、rule11；98表934记录（candidate435/confirmed393/historical36/unverified70），24张必需空表、684项正式阻断。参数只在两本Excel维护，`runtime_ready=false`、`production_ready=false`。
- 验证：242项JS、41项配置Python、4项Windows主机、377个PB边界及403条真实Mongo/WS全包race通过，0跳过；协议60个PB数据消息、114个64位字段。6400组来源对拍只证明登记子集。Creator实构80.56秒、exit36，10份保存资产不变。
- 本机观察：两技能正常购书/原书学习/触摸施放5项流程通过，隔离QA仅提供等级40和余额，不预置书本或技能；通用UI6项流程通过。真机、全阶GUI、自然养成、真实未知提交故障和容量尚未验收。
- 客户端：4个scene和6个prefab；17×17菱形地图、64×32地格、48像素土层、4件小物和285可走格保持，名字24×0.7稳定头顶布局保持。3332张来源候选PNG逐字节保持；聊天发送仍未实现。
- 环境：Windows使用锁定工具链及用户授权的专属原生MongoDB8.0.34副本集，回环27119；macOS保留Lima方案。所有工程命令经tools/dev，历史QEMU失败保留。
- 其余阶段及历史Auto观察按原报告保留，局部通过不解除完整P0、目标1.80、真机或正式发布门禁。

首组技能阶段（421ce1c）的历史构建与关键路径见`docs/技能阶段验收.json`。三职学习扣原书、初学0级、火球与治愈表现/耗蓝已核对；带追踪复测的一次请求对应一次受理，连续两点的第二次被动作锁拒绝。Auto已改用内置TOGGLE事件并通过真实引擎生命周期回归。首次无追踪时的状态变化尚未归因，保留观察，不把未复现写成已根治；当前首组材料升级已实现，完整技能及升级范围、手机、真实网络提交未知故障与平衡验收仍未完成。
