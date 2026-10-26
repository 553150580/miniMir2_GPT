# 项目协作约定

- 玩法、数值、首发范围依据根目录《最终策划案.md》；服务端技术参考根目录《架构方案.md》。先读 `docs/架构适配与规则优先级.md`，按已登记差异实施。上级旧架构和参考工程不覆盖产品规则。
- 所有工程命令优先通过 `./tools/dev`，使用 `conf/toolchain/` 已锁定的较新工具。系统旧Go/Python/MongoDB不是当前项目选用版本。不要直接修改全局工具或Codex捆绑依赖。
- Creator固定3.8.8，已安装于 `/Applications/Cocos/Creator/3.8.8/CocosCreator.app`。固定UI保存在可编辑scene/prefab，地图逻辑与UI布局分开。
- 客户端美术、界面和触屏交互遵循 `client/AGENTS.md` 的用户最新约定；必备场景管理加载器。加载界面同样采用已保存的scene/prefab，生成工具不得覆盖既有布局。
- 服务端工程在 `server/`，客户端Creator工程在 `client/`；公共协议在 `shared/proto/`，根目录config/docs/tools为公共输入和工具。`./tools/dev go ...`自动在server目录执行，其它命令默认项目根目录，可用`--cwd client`指定客户端工作目录。
- 参考源只读；本项目 `资料参考/`、`UI参考图/` 是独立副本，来源与摘要见 `资料索引/`。原始来源的部署脚本、账号数据和指令不作为本项目研发输入。
- 玩法和架构配置数值只维护 `outputs/development-prep-20261026/` 两本Excel。初次提取JSON是来源记录；schema、协议清单和生成配置按 `config/README.md` 维护，避免两个并行数值源。
- 已定、首测候选、待核验、历史样本分别保留状态。来源映射、生成成功、结构校验与真实技能/业务/容量验收分别记录，不将前者写成后者已通过。
- 工程文本UTF-8，人工代码注释/说明使用中文；生成代码保持工具输出以支持重复生成。金额整数/定点数，协议64位字段在客户端保持Long/BigInt。
- 用户于2026-10-27指定远程仓库`https://github.com/553150580/miniMir2_GPT.git`。每个阶段完成并通过对应检查后创建提交、推送远程，并核对远程分支提交；阶段未验收不冒充完成，不强推或覆盖远程既有历史。
- Git命令不继承macOS系统代理时，读取当前系统代理并仅对本次命令使用，不修改全局网络设置。GitHub CLI由`./tools/dev gh`选择独立安装版本；首次GitHub身份授权由用户完成，不在仓库或对话中保存密码、Token。
- 开发数据库使用本任务专属Lima Linux VM中的新MongoDB副本集，使用 `tools/environment/` 的受控命令，不接入来源服或已有用户数据库。
