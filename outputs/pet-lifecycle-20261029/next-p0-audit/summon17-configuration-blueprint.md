# MYF17召唤骷髅候选配置蓝图

状态：供root锁定typed API后实施的设计，**不是可执行编辑清单**。未修改唯一工作簿、catalog/schema或生成fixture；不将17加入skillIDs，不开放新商品/升级/拆页白名单，不填护符187包内Count或售价，保持护符运行开关关闭。本阶段仅完成宠物World/Life/表现替换底座，17正常业务仍实施中。

唯一工作簿基线：`outputs/development-prep-20261026/玩法配置.xlsx`，SHA256 `157788372fe9db2a8dc800f23ddea90295e8f80acca5a22ce5863e852da7ebfb`。具体来源与冲突见`skills-source-audit.md`、`资料索引/职业宠物补充来源契约.json`。以下JSON键均为接口建议，不能先写进生产形成另一套合同。

## 行与字段修改范围

| 表/行 | 建议变更 | 来源/约束 |
| --- | --- | --- |
| game_items / 新25 | 《召唤骷髅》、skill_book、独立实例max_stack1，沿既有书来源/品质规则 | MYF Stditems.csv:27，Price2000只写价格表；不新增初始赠书 |
| game_skills / 新17 | 职业2，book_item_id25，behavior候选summon_skeleton，native0–3，learn19，MP12+4×rank，技能CD1800/global1800/action600，delay500，自动关闭、自身目标/射程0，伤害参数显式中性值 | Crystal Envir997–1012、MagicInfo18、Human3424–3451/4399–4438；不是MYF16困魔咒 |
| game_effects / 新17（确认空闲后） | 独立召唤效果，唯一引用pet_template_id1，不引用game_buffs；阶段为原Map延迟动作，不能套defensive_buff或teleport_success | 新生与活宠Recall分支不同；不得重复永久扣耗后再次生成 |
| game_shop_prices / 新28（确认空闲后） | 书25、每次1本、price_gold2000候选、产物绑定，沿既有金币优先和NPC书禁拆页 | MYF Price只作开发候选；暂不加入profile skill_shop_entries |
| game_skill_upgrades / 新171–173 | skill17，目标rank1/2/3，原书25数量2/3/4；页数沿既有候选5/10/20，page_item900001；required_role_level选Crystal19/22/26 | root选择的本任务开发候选；**MYF为19/23/26**，显式记录差异，不称用户审定；保留基础行为，不加目标数或范围 |
| game_pet_templates / 现1 | summon_skill_id17，monster_template_id继续2，资源pet_skeleton；补完整召唤/训练/Recall/生命周期typed规则 | 不把MYF模板Level15当PetLevel；单活宠，不套来源2槽 |
| game_pet_growth / 新1–8（当前空表） | 对应PetLevel0–7的属性增量及到下一级训练经验阈值；不复制monster基础属性 | 下表；经验乘3与上限/速度公共规则只维护一次 |
| game_map_regions / 现201、202 | 未来相应版本在现map_policy各补显式no_pets=false候选，与no_teleport分开；保留范围、ID与原NoTeleport | 延续当前个人/Town可携宠健康移交的本作开发范围，不认证MYF源图标记；原5个safe_zone不动，ZoneSetHash不加入policy |
| dev_first_loop_profile / 现1 | **本蓝图不改**。后续激活阶段只选择ID及明确版本/能力门禁，不内嵌pet数值 | 当前22个skillIDs、初始3职业书、binding初始池关闭、护符关闭、背包/宠物出生等保持；不得只给当前15降版/删字段伪装新合同可用 |
| game_items / 现187 | **不改包数、单价或unknown字段** | 用户护符Count适配仍pending；不能从本蓝图填1000或500 |

召唤书25应沿原书正常学习和原实例升级；只有后续正式开启时，才同时加入相应正常商品/合法来源品质拆页选择。不能单独新增商品后使其在未支持技能阶段可购买。

## 属性与经验：推荐落表形式

`game_monsters.id=2`继续唯一保存基础HP60/DC10–20/AC与MAC10、模板战斗Level15及原1000ms移动/攻击间隔。当前pet_templates.base_attributes里的相同数字是旧初始化遗留，建议未来改为明确继承规则，拒绝覆盖数值，例如`{"kind":"inherit_monster_template"}`；由强类型投影只读取monster_template_id2，避免两张表重复维护同一基础值。

growth.attributes只写以下增量（JSON字段可命名hp_bonus/physical_defence_min_bonus/physical_defence_max_bonus/magic_defence_min_bonus/magic_defence_max_bonus/physical_attack_min_bonus/physical_attack_max_bonus），不写最终HP/DC。模板Level不在本表重复。

| growth行ID | PetLevel | HP增量 | AC两端各增量 | MAC两端各增量 | DC两端各增量 | 到下一PetLevel所需训练经验 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 0 | 0 | 0 | 0 | 0 | 20000 |
| 2 | 1 | 20 | 2 | 2 | 1 | 40000 |
| 3 | 2 | 40 | 4 | 4 | 2 | 60000 |
| 4 | 3 | 60 | 6 | 6 | 3 | 80000 |
| 5 | 4 | 80 | 8 | 8 | 4 | 100000 |
| 6 | 5 | 100 | 10 | 10 | 5 | 120000 |
| 7 | 6 | 120 | 12 | 12 | 6 | 140000 |
| 8 | 7 | 140 | 14 | 14 | 7 | 明确终态null；全技能native最高3时不可能超过7 |

每次训练是否还能升级仍先查该宠**受理时冻结的MaxPetLevel**，不能因表存在更高行就自动升到7；rank0新宠初0/最高4，rank1初1/最高5，rank2初2/最高6，rank3初3/最高7。活宠Recall不更新其初等级或最大等级。技能升级本身不能给旧宠免费增加属性/恢复HP。

公共训练规则建议放现pet_templates的typed规则域（可在follow_and_return下增加training，或另建正式字段，待root锁定），不放profile：

```json
{
  "training": {
    "initial_level_by_native_rank": [0, 1, 2, 3],
    "maximum_level_by_native_rank": [4, 5, 6, 7],
    "experience_multiplier": 3,
    "experience_basis": "owner_actual_granted_experience",
    "eligibility": "alive_same_world_within_owner_range",
    "max_level_ups_per_grant": 1,
    "excess_experience": "keep",
    "at_maximum": "do_not_accumulate",
    "current_hp_on_growth": "keep"
  },
  "speed": {
    "basis": "maximum_training_level",
    "move_reduction_ms_per_level": 130,
    "attack_reduction_ms_per_level": 70,
    "minimum_move_interval_ms": 400,
    "minimum_attack_interval_ms": 400
  }
}
```

阈值只在growth各行维护，不再同时保存base20000与同一逐级数组。HP/防御/攻击每级系数已展开成增量行，也不另保留20/2/1的运行乘法数值。公共range只维护一次：`owner_range_cells=16`，同一来源DataRange用于训练资格和远距回归门槛；实际普通跟随贴身范围仍需按来源独立核，不从16推导出另一个缺省距离。

## 召唤、Recall与生命周期的强类型建议

技能cast_timing.delay_base_ms500为新生延迟唯一数值来源；若Recall共用相同来源500ms，可显式声明`recall_delay_source="skill_impact_delay"`，不另复制500。若root选择独立可调Recall延迟，则应作为不同语义的独立字段记录，不同时让同一概念从两处取值。

```json
{
  "summon": {
    "pet_template_id": 1,
    "existing_alive_pet": "recall_same_template_reject_other",
    "material_scope": "new_birth_only",
    "delayed_execution": "origin_world_map_action",
    "primary_landing": "captured_front_in_origin_world",
    "fallback_landing": "owner_current_world_current_cell",
    "landing_rule": "source_static_walk",
    "allow_occupied": true,
    "owner_dead_at_due": "cancel_birth_without_resource_refund",
    "no_pets_check": "at_new_cast_source_world"
  },
  "follow_and_return": {
    "owner_range_cells": 16,
    "recall_delay_source": "skill_impact_delay",
    "recall_same_world_teleport": false,
    "recall_destination": "owner_back_one_cell_then_owner_current_cell",
    "no_pets_on_recall": "freeze_in_resident_world",
    "first_action_after_spawn_ms": 2000
  },
  "lifetime_and_betrayal": {
    "kind": "summoned_no_tame_expiry",
    "corpse_ttl_ms": 180000,
    "corpse_ttl_source_candidate": "Crystal_BoneFamiliar_AI23_default_DeadDelay",
    "new_birth_life": "owner_monotonic_pet_life",
    "healthy_reconnect_and_transfer": "keep_same_instance",
    "cold_enter": "do_not_restore_temporary_pet",
    "corpse_capacity_per_owner": null
  }
}
```

上述NoPets在受理/Recall的时点不能折叠成每tick一条“当前图不许宠则直接删除”。新生进入原World后可能暂时与主人异图；PetWorld驻留、未知材料回调与新scene ACK分别校验。尸体TTL180000是root可选的AI23/BoneFamiliar跨MYF显式候选，不认证源服实际配置。

`corpse_capacity_per_owner`的null是**本蓝图明确未决，不能导出运行**；它不是0/无上限默认。必须由root在技术预算合同中明确上限与满额拒绝新受理策略，再接正常17业务。不能静默提前删除来源尸体，也不能产生无限历史宠物对象。尸体不占第二活动宠槽，但仍占有限内存/表现/同步预算。

`material_scope=new_birth_only`也不能省略：当前Skill.MaterialQuantity为静态值1，而Recall不扣符；受理必须在原owner中先按既有活宠模板身份分支，不能让客户端/Store只按“技能17固定材料1”裁定所有施放。新生依然携原材料实例ID/版本、Auth/原job和Life，确定永久结果后一次受理；已付事件被死亡取消时不造退款，未知提交继续原q。

## 效果与表现建议

Effect17建议`effect_type="summon_pet"`、`trigger_phase="origin_map_delayed_birth"`、parameters仅引用pet_template_id1、target friendly_self、per_cast1、recursive=false；召回作为同一技能的独立执行分支，不创建第二个宠物。最终枚举命名由root与typed handler一并锁定。

技能起手可选已核Magic1500–1509，原生10帧；**appear暂不填写raw_frame默认**。Mon4 2860–2869只是未认证候选，Magic1510–1519为4×1占位；原Crystal首次Extra=false/健康重观Extra=true须转为明确的新生/观察合同。未完成appear原格来源核对时，可清楚限定阶段表现范围，但不得写完整源出场已还原。神兽不在本组候选，不以未支持的Mon18格式或骷髅profile代替。

## 未决项与激活门槛

- 用户护符单位与报价仍pending，不落数值、不开放商品。
- typed召唤/训练/NoPets字段名、能力版本、尸体数量预算、正常/异图出生与Recall测试由root锁定。
- 当前profile/runtime仍是rule15；增加候选基础行不等于开放技能。激活时必须完整升级合同，不能仅向当前skillIDs追加17或把NoPets等新字段混入旧版本。
- 本候选门槛采用Crystal19/22/26，MYF19/23/26差异保留，状态candidate。原书升级的技能rank与宠物临时训练等级始终分离。
- 最终正常材料/召唤、训练与再召、真实Mongo/WS、Creator/保存UI各自验收；不将已有驻留/Life/同handle外观底座测试计作17业务完成。
