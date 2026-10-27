# 配置维护与导出

## 数据源

正式开发配置源为 `outputs/development-prep-20261026/玩法配置.xlsx` 和 `架构配置.xlsx`。工作簿目录页显示记录状态与表覆盖。配置域按最终策划5.11拆分，架构实验参数单独维护。

每个数据表：第1行中文标题，第2行稳定英文 `snake_case` 字段，第3行类型、必填、单位和导出目标，第4行起是记录。空表已提供字段结构，缺失内容见各工作簿的“发布缺口”。同一张表内整数 `id` 唯一。空白代表未知，不用0占位。

`record_status` 的值为：

| 值 | 意义 | 导出行为 |
| --- | --- | --- |
| confirmed | 文档已确定的规则或已核验数据 | 可进入预览；正式发布仍要求业务验收 |
| candidate | 文档给出的首测候选 | 可进入研发预览，必需表不能据此正式发布 |
| unverified | 来源版本、数值或行为尚待核验 | 可进入研发预览，必需表禁止正式发布 |
| historical | 历史经济测算条件或结果 | 不进入双端运行数据 |

每条记录均保留 `source_ref` 和 `note`；导出时剔除它们和其它 `target=none` 字段。来源是既有文档章节/行号、用户已确认规则或提取记录，状态不能代替来源证据。参数修改需要同步其证据；关闭发布缺口需要填写具体报告或批准变更引用。

## 当前范围技能、时序与技能经济配置

当前快照为`dev-combat-area-v0.5-ddfb248d01b3`、rule_version5。98表841记录，candidate343、confirmed393、historical36、unverified69；26张必需空表及565项正式阻断。新候选不代表正式配置成熟。

- 8项职业基础候选仅维护于`game_skills`，另保留历史开发普攻0；profile只选择skill_ids、初始只赠首组三书中的本职业一本。
- `game_skill_upgrades`为8技能×3目标阶共24行，2/3/4本是用户确认的目标阶+1，页5/10/20与各自等级仍candidate；升级不改变目标、格集或射程，不启用随机练功。
- `game_shop_prices`唯一投影10项：旧三书各500金币，新五书按MYF Price候选600/2000/1600/3000/15000金币；书页50元宝单位或100绑元单位。`price_gold`为金币整数，`price_u`为元宝/绑元十倍定点，币种独立，购后绑定。
- `game_book_recycle`唯一四品质候选表投影8书×4品质32条，NPC/赠书/legacy禁拆；合法掉落书输出继承绑定。原Price不等于已核验NPC金币回收。
- Excel显式维护combat_timing、Undead、攻速与逐阶准确数组、目标类型/区域/遮挡/自动策略。自动优先级/目标数、事件128与逐杀16仅development候选预算，不证明原服AI或承载。 区域中心校验边界/射程，逐目标再过滤有效格，不将中心walkable作为施放前置。

预览与提交均引用当前配置版本、角色revision、具体原实例/数量/版本及原Plan。空材料不能由服务端自行选取，规则或资产变化则拒绝旧预览；永久扣费/rank、原操作回执及审计在owner事务提交。永久经济回执不设TTL，迁移只去掉`operations.created_at`单字段TTL并保留现存文档；`skill_quotes.expires_at`和票据的临时TTL另行保留。已被旧TTL删除的历史不能因此恢复。 本阶段浏览器精神力0→3已核2书5页/3书10页/4书20页，准确0/3/5/8；升级9书5400金币、35页350绑元，重入后rank3和绑元650保持。

当前`make test`完整退出0：153项JS（16场景、137项UI/应用）、23项Python配置回归、Go测试及258个PB64位边界通过；协议40个消息号、29个错误码、49个PB数据消息、79个Long字段。Creator3.8.8 Web Mobile最终实构116.84秒、exit36成功，全部脚本与7份scene/prefab哈希当前一致，7份布局构建前后未被覆盖。 当前真实Mongo/WS全包race为96条pass、0跳过，源码稳定；法师与道士限定范围本机浏览器已通过，新增warn/error为0；不扩大为完整技能、P0、正式或真机验收。本阶段报告为`docs/范围技能阶段验收.json`，各项结论按限定范围分别记录。 d1207bc的134项JS/20项Python/64条Mongo race/139.33秒及道士0→3、拆页/列表/取消/持久仅为上一阶段历史证据；隔离QA预置不是自然获取或正式平衡验收。

## 命令

```bash
# 仅检查结构、字段、ID、引用和当前缺口
./tools/dev python3 tools/config/export_configs.py --check

# 从实际Excel读取，生成带哈希的不可变双端研发预览和Go/TS字段绑定
./tools/dev python3 tools/config/export_configs.py --version dev-prep-v0.1

# 正式门禁；当前未实现/未核验部分会返回退出码2
./tools/dev python3 tools/config/export_configs.py --check --mode release
```

检查报告在 `config/reports/配置校验报告.json`。导出到 `config/releases/<版本>-<内容哈希>/`，完成后原子切换 `preview_current.json` 指针；正式门禁不通过时不会生成半套发布数据。它只生成本地文件，尚未连接gameNode，也不对线上环境发布。

客户端和服务端JSON分别使用字段的 `client/both` 与 `server/both` 目标。逐记录状态仅放在发布清单的 `record_states` 内，双端载荷均不包含它；来源文案也不投影给客户端。客户端JSON不是伤害、掉落、权益或交易的裁决依据。

实际 `config_version` 包含内容摘要，例如 `dev-prep-v0.1-<摘要>`。它与目录和发布指针一致，业务引用此完整标识；同一个人类可读前缀不会掩盖不同内容。清单另保存来源文件、工作簿、schema及每个产物的摘要。

Excel数据区公式需要在表格引擎中重算保存后再导出；没有缓存或存在公式错误时拒绝处理。协议64位ID、版本等字段使用BigInt/Long；配置表整数禁止超出Excel精确整数范围。元宝内部单位为显示值的10倍，佣金/概率尽量使用整数万分比，特殊小概率需使用字段注明的更高精度。

## 修改规则

- 添加记录：保持字段名不变，分配稳定ID，填入状态、来源和实际数据。
- 添加字段/新表：先修改schema和对应导出检查，再扩展Excel第1～3行。现有ID和枚举值不随排序改变。
- 通用JSON规则需声明嵌套类型/键契约，整数费用/秒数不能用小数绕过。已定JSON规则还保存来源值摘要；改变已定规则须先更新策划与契约，不能通过把记录状态降为候选绕过。
- 修改两份依据文件：先完成规则与配置对齐，再更新 `source_locks`；导出器会拒绝依据已变而schema未审视的情况。
- `config/catalog/fragments/` 是本次准备阶段的初始提取记录，后续不能同时编辑种子和Excel作为两个当前数值源。`build_workbooks.mjs` 遇到已有工作簿直接拒绝覆盖。
- `config/releases/` 产物不手工修改。变更Excel后使用新的配置版本重新导出。

## 工具范围

当前已提供结构/类型/ID/枚举/可声明跨表引用、基础精度和状态校验，以及来源锁、缺口证据、双端字段裁剪、不可变导出与原子指针。复杂技能行为、绑定链守恒、限额窗口、世界BOSS供给、托管真实等价和热切换安全需要开发阶段的业务用例验证，仍受工作簿发布缺口约束。

目录统计预留每表10000条记录；超出后应扩展目录公式。导出器读取工作表所有实际数据行，不按目录统计截断。

工作簿由 `tools/config/build_workbooks.mjs` 使用Codex捆绑的 `@oai/artifact-tool` 初次创建并视觉检查；业务导出器使用Python标准库，后续修改数据不依赖该作者工具。

`make preview`从Excel生成不可变预览后，会按清单摘要把Go类型同步到 `server/internal/configtypes/`、TypeScript类型同步到 `client/assets/scripts/generated/`。这些工程文件是生成产物，修改字段或数据后重新导出/同步。完整版本化数据和历史类型快照仍保留在公共 `config/releases/`。

