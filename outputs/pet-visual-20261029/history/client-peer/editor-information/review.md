# Creator 3.8.8 information.json只读复核

复核对象是当前隔离工程 `run-425e92f75bf946afb173ef23be024c21`；没有修改工具、主工程或来源，没有运行Go、数据库、浏览器或Creator。读取本机安装 `D:/Program Files/CocosCreator/3.8.8/resources/app.asar` 的索引和可读包声明/静态页面，保留其摘要与提取副本；未解密或执行包内二进制 `.ccc`。

## 原新字节与精确变更

- 原始文件SHA256：`7021c886e57b3a052cc64c641bb189a0d2612a1fc285ad939e0c1d3b1d1d3f1c`，同时匹配 `project-copy.json` 的 source_sha256 / project_sha256。
- Creator结束文件SHA256：`33acf5c6708a3b741da6b3b63cc05496c284c94f90a24fecf3e0c3401a5bca1f`，匹配 `creator-report.json.input_sha256`。
- JSON仅两处leaf变化：`information.customSplash.customSplash.form`、`information.removeSplash.removeSplash.form`。
- 两个URL均仅 `sid` 查询参数变化；删除该参数后，原新都严格为 `https://creator-api.cocos.com/api/form/show`。其它键、类型和值保持。

机器报告为 `json-diff.json`。原新raw仅在本.work目录分别保留为 `before.raw.json`、`after.raw.json`，本报告及工具stdout均不输出SID值，也不将它解释为本项目游戏账号Token。

## 本机primary依据

本机ASAR路径与条目摘要见 `asar-source-index.json`。提取出的文本保留原行号：

1. `builtin/information/@types/public.d.ts:5–16` 明确定义 `IInformationItem` 是“某个功能的问卷数据”，形状为id/label/enable以及[id]{complete,form}，与实际项目information文件一致。`public.d.ts:29–33` 的查询状态包括 `success/cache/network_failure/network_exception`。
2. `builtin/information/@types/message.d.ts:4–36` 只声明编辑器查询问卷、打开/判断/关闭问卷窗口的消息。`builtin/information/package.json:7–31` 将这些Editor消息注册到该包browser模块。
3. `builtin/information/static/survey-form.html:468` 说明收集开发者项目相关信息。`:635–647` 从页面URL解析参数；`:1018` 使用SID查询 `/api/form/queryUserInfo`；`:1054–1058` 将SID随表单提交到 `/api/questionnaire/submit`。它是Editor问卷/服务开通接口，不是本项目运行协议。
4. 本机引擎 `resources/resources/3d/engine/cocos/game/splash-screen.ts:139–146` 在运行时读取 `SettingsCategory.SPLASH_SCREEN` 的 `displayRatio/totalTime/watermarkLocation/autoFit/logo/background`；`:150–151` 按这些真实参数决定是否显示。此处没有读取information.form或SID。

限制：实际编辑器写入函数在 `builtin/information/dist/browser/index.ccc` 与 `utils.ccc` 二进制模块内，未作为明文读取。因此不声称已经看到具体 `setConfig/setProject` 调用，也不声称任意未来enable/complete变化与编辑器功能开通流程完全无关。

## 最小建议

当前证据支持把精确 `client/settings/v2/packages/information.json` 定义为可再生的Editor功能问卷/服务查询缓存，从受控工程复制和产品源码输入清单中明确分离；只登记这一个文件，不能忽略整个settings目录。先保留原始缓存字节，再由Creator正常生成；每次构建单列该缓存的存在性、原始before/after SHA与键级变化。这个分类是缓存归属处理，不是绕过Creator功能授权。

其它settings、项目/Builder启动画面设置、场景、TS、资源及全部编译产物仍按原严格规则校验。旧 `source_stable=false` 的构建报告保持历史原样，不能重写为通过；采用新缓存合同后需由受控工具重新建立可核验的输入/产物证据。是否复用现有产物，由根代理按新的精确分类和所有其它输入完整一致来判定，不由本只读审查代验收。
