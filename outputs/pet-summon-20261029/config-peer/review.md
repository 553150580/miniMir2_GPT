# rule17配置与隔离WS只读二审

当前结论：两处实证配置门禁缺口已由root修复，原输入反向重放通过；当前生成物与preview的来源摘要匹配。没有运行数据库，没有改生产Go、编译器、两书或schema。本次执行仅devconfig的临时Go overlay与只读JSON摘要核对，不代替完整Go或真实WS验收。

## 已关闭的实证问题

1. 旧schema1/旧rule16用解码后的RecallName非空判断新字段污染，会接受显式`recall_name:""`。最初两个overlay均真实FAIL，见`legacy-before-fix/legacy-probe.txt/json`。root改为原始JSON字段presence门禁后，同一两输入均PASS，见`legacy-probe.txt/json`；5个相关输入摘要前后相同，package1.713s，exit0，无DB。历史schema1省略Name仍由root的正式源码测试覆盖，新schema2/当前rule17要求Name有效；未用Name的缺省不能拿空字符串冒充旧合同。

2. 独立catalog及完整Fixture先前都接受三个升级阶仅各需1本书，违反既定目标阶+1。`cost-before-fix/cost-probe.txt/json`保留同一真实反例：不是绕过Fixture.Validate。root同时收紧新catalog与共享经济校验后，原probe PASS，见`cost-probe.txt/json`；5输入稳定，package1.574s，exit0。页数仍读取唯一表，没有在Go另写5/10/20。

## 当前一致性

- `SkillSummonRules`只引用DefinitionID，500ms仍来自Skill.DelayBaseMS，Count1来自现材料字段，出生2000ms仍在PetLifecycle。Name与同名判断不混用模板ID；DataRange由Training唯一字段提供别名，没有第二个16。
- 新候选严格要求完整Skill/Book/三阶、关闭runtime，拒绝未知字段与缺省/null；非召唤技能不能携新召唤子合同，召唤不能带teleport/displacement空合同或未用伤害参数。主保持22技能、legacy不训练和材料关闭。
- 当前`dev-pet-summon-v0.17-9cfc98f544bf`的全部manifest文件摘要已逐项核对。两本唯一书、schema、三个编译器、来源契约摘要均匹配候选source_evidence；Pet候选schema2的定义与主fixture完全相等。具体路径和当前Go摘要见`source-evidence-check.json`。

## 隔离WS前提与表述

`integration/summon_pet_test.go`显式从当前主配置建立局部样本：19级、正常本地创角配置给书25、两个独立Count单位且局部MaxStack2、怪HP1/零伤/长时钟、禁宠Town、bootstrap none。资格之后走真实注册/创角/Enter/原书学习/材料装卸/施法/攻击/旅行/健康票/冷建协议，没有向临时room注入宠物，Count2也没有被写成购买包规格或主护符售价。500ms原Front叠格、新召扣1、无装符Recall、玩家击杀训练及NoPets异图Owned均有对应断言。

一处需收口的覆盖表述：当前末句日志声称冷建后“永久技能和耗材保留”，而冷建后的断言只直接检查无临时宠物、无临时BSON字段及全程唯一材料回执；没有显式核对冷建Role.LearnedSkills17和剩余Count1。已建议root补持久Role/Inventory断言或缩窄日志，不能只靠旧receipt存在代替剩余资产与技能仍在。本审查未执行该WS，不提前宣称其业务通过。
