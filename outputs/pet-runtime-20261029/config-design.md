# 宠物尸体与训练候选配置设计

2026-10-29。只读核对两本唯一Excel后形成设计；尚未修改真实工作簿、schema、rule15 fixture或免费bootstrap。护符187包数/单价仍未决，不填猜值，不选入17，不打开材料或商品入口。精确Go字段接口由root随后锁定，当前不将建议键名冒充已定API。

## 当前表源与可复用字段

唯一玩法书SHA：`157788372fe9db2a8dc800f23ddea90295e8f80acca5a22ce5863e852da7ebfb`；唯一架构书SHA：`b4cfd1336a200fa0bae8791c001ab6a189a729cf2999e630ad98120ae8bdc4ab`。

| 当前表/字段 | 当前值 | 本轮可复用方式 |
| --- | --- | --- |
| game_pet_templates.id1 / monster_template_id | 2 | 继续作为基础属性唯一引用；实际HP/DC/战斗Level来自game_monsters.id2 |
| 同行 / base_attributes | 旧HP60/DC10–20等重复对象 | 建议换为明确继承描述，拒绝再维护同一组数值；需替换本行既有JSON合同 |
| 同行 / follow_and_return | null | 复用该现有JSON字段放强类型回归与训练资格/公共规则；不新增profile.pet参数副本 |
| 同行 / lifetime_and_betrayal | null | 复用该现有JSON字段放召唤宠生命周期、尸体TTL和规则身份；容量本身来自架构预算引用 |
| 同行 / summon_skill_id | null | 本轮保持null，当前没有game_skills17行，避免制造悬空引用；source_skill_id17可以是审计元信息，后续完整17定义一起接线 |
| game_pet_growth | 0行 | 现有growth_level、attributes、required_conditions足够新增0–7逐级属性增量和经验门槛，无需增加Excel列 |
| arch_runtime_budgets | id1–39 | 新id40/code建议pet_corpse_capacity_per_owner、value128、unit具/owner，candidate/adapt/pending；该值只维护此行 |
| game_map_regions201/202 / entry_rules | map_policy+no_teleport | NoPets将来可同一policy显式增加；当前rule15投影严格只允许原2键，本轮不可先加第三键导致正常导出失效 |

当前`game_pet_templates`已有1行，不是空表；growth才是空表。表头和字段均可以保持。公共规则不进开发profile；两个已有安全区/旅行参数域也不接管宠物数值。

## 独立候选生成物的落点

建议后续新增独立生成器，读取两本Excel+schema+已登记来源，生成`config/development/pet-runtime-candidate.json`；候选审阅期间可先输出到本`.work`目录。它不成为另一个可编辑数值源，必须可由源书复现。

建议外壳：

```json
{
  "schema_version": 1,
  "runtime_enabled": false,
  "source_runtime_rule_version": 15,
  "candidate_kind": "pet_corpse_training",
  "pet_id": 1,
  "base_monster_template_id": 2,
  "rules": {},
  "growth": [],
  "budgets": {},
  "_source_evidence": {
    "workbooks": [],
    "schema_sha256": "...",
    "source_contract_sha256": "...",
    "generator_sha256": "..."
  }
}
```

数值只从Excel投影，`rules/growth/budgets`不是手写运行JSON。外壳开关明确false，正常`conf/dev/server.json`不增加候选文件引用，`first-loop.json`不自动合并该对象，普通room不据此开启训练/尸体流程；隔离单元/集成测试显式加载并校验候选，用同一typed规则调用待验API。不能仅因文件生成成功把17或候选运行开关置true。

生成器应同时锁两书摘要；独立候选编辑器接受的副本只限`.work`，须记录其原唯一源书SHA、候选SHA与逐格diff。root审阅并应用唯一书之后，正式候选生成物必须从新的唯一书重新生成，不能长期靠`.work`副本替代权威输入。数字无默认：缺少预算/TTL/训练层、错类型/重复层/非有限数、来源身份变化均拒绝；`runtime_enabled`缺失不能当false蒙混正常校验。

候选自身摘要取所选typed载荷及来源身份；把来源书SHA、schema、生成器与来源契约放证据域。不要把128再写成Go fallback、profile值或另一张JSON参数表。

## 玩法与架构数值分工

玩法PetDefs唯一规则建议包含：

- 已匹配来源DataRange16（`资料参考/crystal-supplement/Shared/Globals.cs:37`），用于同图近距活宠的训练资格与原回归距离判定。
- 初PetLevel逐rank `[0,1,2,3]`、MaxPetLevel逐rank `[4,5,6,7]`；与模板战斗Level15独立。
- 训练经验乘数3、实际主人获经验为基数、每次最多升1级、超额保留、到Max不积累、升级当前HP保持。数值与算法枚举必须由完整typed对象明确提供。
- 速度减量基于MaxPetLevel：每级移动130ms、攻击70ms，下限各400ms；从monster基础间隔派生，不把逐级最终速度再写一张表。
- 尸体TTL180000ms，source_kind为本作选择Crystal BoneFamiliar/AI23 default DeadDelay的跨MYF开发候选；不存在的真实MYF AI对应不补造。
- 明确临时现场不持久化；尸体只保留原对象/World/Life，不clone、不改写死亡对象身份。不因健康观察重新计时。

架构新预算40建议：

```json
{
  "id": 40,
  "record_status": "candidate",
  "source_ref": "docs/职业宠物生命周期与召唤实施边界.md；本任务尸体有界容器技术候选",
  "note": "128具/owner为独立容量上限；180s TTL与1800ms全局新召受理间隔给出保守101重叠估计，依赖所有World清理和所有新生路径门禁，不是整机容量验收。",
  "code": "pet_corpse_capacity_per_owner",
  "value": 128,
  "value_unit": "具/owner",
  "applicability": "同owner两个保留World合计的职业宠物尸体对象",
  "adaptation_status": "adapt",
  "verification_status": "pending"
}
```

PetDefs生命周期引用预算code，生成器解析后投影为typed capacity；不在同一生命周期单元格另写128。架构表value是number泛型，新增特定code的导出检查仍必须要求严格正整数并拒绝bool/小数/0，不宜把整个架构value列一律改int影响其它技术参数。

## 128容量与101估计的成立前提

保守计算：`ceil(180000 / 1800) + 1 = 101`。来源TTL180000ms来自AI23所走default DeadDelay；1800ms来自Crystal MagicInfo默认技能CD及Human公共SpellTime，未来17候选确需由唯一技能行采用并由服务端保持。128是root选择的技术预算，不能称为源服既定数量或整体容量已通过。

101估计仅在以下条件同时满足时适用：

1. 同owner所有新宠生成受理共享该最短间隔；自动、手动、延迟队列、重放/unknown、管理替换均不能绕开或造第二个新生。
2. 一个受理至多出生一次；任何已有宠召回不产生新对象，出生失败或主人死亡不补造宠物。
3. 尸体以实际死亡时钟加绝对TTL；两个World合计清理，不能只清当前观察图。休眠World不运行新AI的规则不等于无限延长尸体；新生容量检查前必须处理到期项。
4. Pending冻结时不接新生；恢复后先清过期对象再检查容量。健康重连不重置最早期限；冷重建不恢复临时尸体。
5. 期限边界、同tick多个到期与时钟离散使“+1”成为保守项；若所有实际生成路径/时钟尚未实现，该式只能称设计估计，不能先标严格已证明。

容量满时应拒绝新的受理/安装，并确保发生在永久扣符之前；不能静默删未到期尸体，也不能先收费再靠“上界应该够”丢新宠。本轮可以用低容量隔离规则验证边界，测试值必须来自明确的隔离typed对象，不回写当前运行配置。128容器上限不等于客户端同步、帧渲染或进程总内存容量已验收。

## Growth0–7与JSON合同

attributes只包含相对monster基础模板的增量：HP `[0,20,40,60,80,100,120,140]`；AC/MAC两端各 `[0,2,4,6,8,10,12,14]`；DC两端各 `[0,1,2,3,4,5,6,7]`。阈值依次20000/40000/60000/80000/100000/120000/140000；7级为全native最高上限，明确terminal，下一阈值使用有类型null/可选字段，不用0混同未知。

每行required_conditions建议只放本级到下一级门槛/terminal身份。经验倍率3、最多一次升1级等公共规则只在PetDefs一次维护。技能native rank与训练level分别校验；Max4的宠不能因表有5–7行就越过其冻结上限。

需要的schema JSON契约最小集合：

- 替换PetDefs1.base_attributes既有数值shape为明确继承描述；原怪物2实际基础值不改。
- 新增PetDefs1.follow_and_return、lifetime_and_betrayal的精确对象shape；拒绝多余键和缺字段，训练与速度子对象严格类型。最终字段名按root的Go接口对齐。
- Growth1–8各新增attributes与required_conditions合同，共16个；终态7级形状单独验证，不给所有级都允许任意null。
- 架构预算新行不需JSON列/表头；追加语义检查其code唯一、整数值及正确单位/适用域。
- 新独立候选生成器需要验证PetDefs唯一、monster引用完整、0–7连续且不重复、增量非负/有界、终态唯一、最大等级数组引用已有层、TTL及预算引用严格存在、源合同/两书摘要；不通过当前旧profile降版绕过新合同。

以上先复用现列即可；如精确Go接口要求拆出独立training列，应再评估新增列成本，当前无必要先做表头迁移。

## 本轮不加入的业务配置

17书25/Effect17/Shop28/升级171–173以及完整NoPets政策，继续见上一份`next-p0-20261029/summon17-configuration-blueprint.md`。当前rule15 map_policy精确2键，不能为了候选测试直接加no_pets导致旧导出失效；可在root锁定独立候选投影/后续版本时整体引入。护符选择pending，不能新建187商品或补Count，也不能把false候选模块接成日常逻辑。

本轮可独立完成的是同源typed corpse/training规则、孤立加载、原对象/Life/World与confirmed EXP事实推进的隔离验证。免费bootstrap保持既有行为；完整17正常业务仍须用户材料决定、完整配置/协议接线和真实端到端验收。
