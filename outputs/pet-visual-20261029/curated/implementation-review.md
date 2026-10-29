# 资源、唯一配置与独立GUI工具交付

资源/配置输入已冻结。`resource-diff.json`记录完整摘要与增量：原3332个frame记录和20个effect记录保留；仅新增Magic1500–1509与Mon4 2860–2869，生成总3352帧；原7998张PNG、13158个地图相关文件、10份保存scene/prefab和9972个旧meta字节保持。两个WAV与独立来源副本逐字节相同。没有新造美术、转码声音、重写旧PNG、修改地图/其它profile或保存布局。

来源契约 `资料索引/宠物出生表现来源契约.json` 明确Lib仅本地技术资产候选，声音仅有既有PList Name/Length与verification记录，非本轮联网认证。四件独立副本均st_nlink=1。契约只维护跨来源原格/音频映射；资源编译器从锁定源码派生Magic起手count/600ms，从真实Lib FrameSet派生出生count/100ms/stride0/additive，不另维护1000ms副本。manifest的pet_birth_source锁契约和子编译器，converter_sha256仍锁主生成器。

唯一玩法Excel仅C4中的profile.parameters.rule_version由17改18。所有其它单元格值、460条公式与102个非目标ZIP成员保留；架构Excel/schema未改；主22技能、legacy不训练、材料关闭、赠书/商店及独立候选required_rule_version17均不变。`workbook-apply-report.json`保存旧新SHA与单格差异。first-loop/public/preview由Root统一导出，未由本工单另造运行配置。

资源生成/check均真实通过，最终manifest为 `5c5059f7a25c1821f0fd3704c697bf240361693ea883bf8a30e1c1a0a75e136b`；命令、stdout/stderr与实际退出码分别见 `checks/resource-generate-final.*`（15.035秒/exit0）、`checks/resource-check.*`（14.886秒/exit0）。来源派生与损坏输入4项纯Python测试见 `checks/resource-unit-tests.*`，0失败；没有执行Go/DB/Creator。音频importer是3.8.8封装ccc、没有可核默认meta样例，因此没有猜造音频meta，由Root后续真实Creator导入生成。

独立GUI工具为 `tools/client/verify_pet_visual_browser.cjs`，正常注册/Job2创角后，仅从10006 RoleSummary回包取role_id，经Root的--qualify stdin资格API在首次Enter前设置19级与Count2。以后均走保存UI学习原书25、装符、快捷施法、卸符Recall和换角健康新票。内部对象只读；使用公开Director AFTER_DRAW观察实际GPU帧0/4/9和实际材质blend，声音只读AudioSource.playing/playCalls；没有私调cast、born、unlock或改route。按fresh/recall/healthy分段监听，后两段的短暂appear也不能漏过；账号口令、Token与短票只在内存脱敏表，登录前不留截图。

工具严格核本轮隔离工程/完整实构输入和产物、18110/18111/18112三节点instance、18081静态build身份，核主client复制输入与六份配置的运行前后SHA。实际浏览器/Creator/DB由Root串行执行；本工单没有运行，也没有声称真实UI、声音或GPU已通过。尸体重召、自然购符/符包规格、自然成长、128容量与手机真机均不在本工具范围。

客户端peer指出最初GUI读取不存在的cooldown_ms，已改读实际初学0级candidate的cooldown_base_ms/global_spell_ms/action_ms并增加真实catalog回归。它是启动前工具字段错误，未发生产品失败；旧3项工具记录保存在 `checks/gui-tool-tests-pre-cooldown-fix.*`，最终4项纯Node测试0失败/0跳过见 `checks/gui-tool-tests.*`。最终GUI工具SHA `9d1c994eb0adf5b94ee0a2c815d23a67db83cbce8bf27b4cec7007d845c382a8`。

另已完成PetBorn服务端只读二审，未发现实质阻断。来源范围及10文件SHA见 `server-peer/review.md` / `inputs.json`；没有执行或代替Root统一Go/WS测试。
