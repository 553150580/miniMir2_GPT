# Windows工具链适配与当前验证

日期：2026-10-28。当前适配现场为Windows AMD64，统一入口是`./tools/dev.ps1`。Windows工具链及rule10权威位移与导航观察基础已通过限定开发验收，19项技能范围保持；不把位移原语计作抗拒火环、野蛮冲撞或瞬息移动完整技能开放。最终证据见`权威位移与导航观察阶段验收.json`。

## 工具与使用方式

版本及官方归档摘要统一维护在`conf/toolchain/windows-amd64.lock.json`。Go1.27.3、Python3.14.9、Node24.22.1、protoc36.2、Go PB插件1.36.12、gh2.103.2、.NET10.0.403均安装在`%LOCALAPPDATA%/minimir-toolchain/windows-amd64`。Pillow13.0.0及Mongo驱动使用独立venv与哈希锁；浏览器检查使用单独安装的Playwright1.64.1和本机Chrome。

Creator仍固定3.8.8，本机安装在`D:/Program Files/CocosCreator/3.8.8`，其它位置用`MINIMIR_CREATOR_APP`指定。race使用现有WinLibs GCC16.1.0；QEMU便携包通过已有7-Zip提取，不运行全局安装器。工具入口不修改系统Go/Python、全局PATH或Codex依赖。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/environment/bootstrap_windows.ps1 -InitializePython -BrowserTools
./tools/dev.ps1 python3 tools/environment/check.py --strict --skip-database
./tools/dev.ps1 task check
./tools/dev.ps1 task test
./tools/dev.ps1 task client-build
./tools/dev.ps1 task client-typecheck
./tools/dev.ps1 task development-resources-check
```

浏览器检查需要先启动本项目数据库与开发服务，并使用当前源码的成功构建；执行`./tools/dev.ps1 task client-browser-test`。它通过正常注册、创角、入场及触摸操作验证导航、背包、加高视口、退出取消确认与重入，只保存导航元数据和截图，不保存账号密码或票据。它不替代手机真机检查。

服务端/静态客户端的启动、状态和停止改为核对进程路径、创建时间、项目根和实例标记，再发送Windows正常退出信号。外来或旧Mac状态不会被认作当前进程。Go监督进程按平台拆分子进程控制，不改变租约、事务或RPC业务时限。

## 来源与生成产物

用户提供的MYF根目录为`D:/123DownLoad/20231121沉默世界【沐夜风】群服毕业版/MirServer沉默世界_MYF/Mir200`。`Exps.ini`及`Envir/MonItems/稻草人.txt`与原锁定SHA-256一致，已按原字节补入项目独立副本。Crystal的`Client/MirScenes/GameScene.cs`同样按既有摘要定位并补齐。共325份参考副本有摘要记录；Windows校验不会把不存在的Mac原始目录伪装成已重扫。

原始美术根目录定位为`D:/Mir2/mir2_gxx/assets/legacy/primary_180`。10个WIL/WIX包和地图摘要与历史来源一致，隔离目录再生成的3332张PNG逐字节一致；地图图片一致，地图JSON仅排除历史绝对来源路径后比较。现有资源清单、原PNG、scene/prefab与两本Excel未因平台迁移重写。

配置生成器跨平台路径变化只改变first-loop的生成器证据摘要，运行字段未变；对比证据保存在`outputs/windows-adaptation-20261028/fixture-migration.json`。rule10本身是迁移前已有工作区变更，不能将它与此前rule9的数值差异归因于Windows迁移。

## 验证范围与历史环境问题

已完成的工具、配置、协议、客户端测试、Creator实构、资源再生成及浏览器证据统一保存在`outputs/windows-adaptation-20261028/`；每条命令记录实际退出码。Windows主机边界4项、客户端240项（场景16、UI/应用224）、配置Python41项、PB边界377项及真实.NET来源对拍3892组通过。最新Creator实构171.48秒、exit36，10份保存布局保持，当前引擎类型检查通过。浏览器触屏6项流程通过、JS错误0，保留1条无头浏览器屏幕方向警告；5张成功截图仅作本机限定观察。

`docs/开发环境检查.json`只说明所列环境检查，不证明完整业务稳定。浏览器现场发现的旧组件退出回调问题已修复：场景销毁后不再访问被Creator清空的导航或节点字段，迟到错误也不覆盖新场景状态。新增2项回归覆盖成功销毁、切换失败恢复输入及旧按钮异步拒绝，真实浏览器换角重入也已复核。

用户于2026-10-28明确选择本任务专属Windows MongoDB8.0.34新副本集，并授权相应修改原Linux VM约定；`AGENTS.md`已同步，macOS仍保留Lima。Windows使用`127.0.0.1:27119`、`minimirWinDev`，数据目录为`%LOCALAPPDATA%/minimir-mongodb/windows/8.0.34/<project-key>/data`。首次目录为空，安装标记绑定项目根、二进制官方归档摘要、数据目录和端口；每次连接/停止前核对PID创建时钟、映像、完整命令行和回环监听。正常停止后日志确认完成关机，再启动和事务提交/回滚均通过。未接入已有Mongo9、来源服或旧VM数据。

历史软件TCG通过副本集和事务探测，但真实全量Mongo/WS race曾出现2秒续租及3秒事务时限问题，节点按现有规则安全隔离；原始失败事件保留，不改写成成功。原Lima VM已正常关闭；Lima与QEMU平台锁留作可选诊断依赖，Windows默认引导不再安装它们。

最终在原生副本集上执行全量Mongo/WS race：379条通过、0跳过、源码稳定，单项RPC、事务和租约时限未放宽。群疗集成中的旧HP全等断言已修正：接收技能期间仍可正常受击，实际施法身份、MP、后续净治疗与同一生命仍须核验；延迟控制复现保留在本机诊断目录。

`conf/dev/server.json`显式登记Windows URI覆盖，Go与测试工具按当前平台选择唯一固定端点，外来端点拒绝；macOS原Lima端点不变。系统Windows功能与全局工具未改动，无重启。限定rule10基础通过后，完整P0、目标1.80完整认证、手机真机、容量及正式发布仍未完成。
