# rule17关闭召唤候选配置

主profile仍22项技能、免费legacy未训练宠物、casting_supply.enabled=false。规则版本升17只接入显式NoPets和来源RecallName合同；两个地图政策分别明确no_pets=false，并保留各自NoTeleport。17、书25及三阶升级只进入关闭的独立catalog，未增加商店、赠书、护符堆叠或售价。

唯一玩法Excel实际82格变化，架构Excel0格变化。原玩法460个公式、全部未请求单元格、97个未编辑ZIP成员逐字保留。第一次应用后发现技能备注沿用“未新增升级行”，再用受控编辑器只纠正同一备注格；候选数值不变。最终重新相对本轮最初源表核对仍是82格，见 `final-source-review.json`、`final-gameplay-cell-diff.json`、`metadata-correction-report.json`；不要把首次应用SHA当成最终SHA。

三个数字各有唯一位置：Skill17.DelayBaseMS500共用于新生/召回延迟；MaterialQuantity1仅描述新生材料数量；Lifecycle.SpawnReadyMS2000仍在PetDefs。自动Recall与训练共用现有Training.owner_range_cells16，通过Go别名表达双用途，没有新填第二个16。RecallName来自已登记Settings技术片段的BoneFamiliar，大小写敏感；与MYF模板的组合为明确跨来源开发候选，不能改用DefinitionID或客户端显示名判断源同名。

Skill17未用伤害、命中、治疗、倍率、除数与模式字段全部显式0/false/空数组，未继承魔法盾或其它技能的未用power。候选书25由与主技能书共用的函数生成，字段完整且不带装备效果。三阶171/172/173复用共用升级投影，书25×2/3/4、页5/10/20沿已审policy；采用本任务既定Crystal19/22/26候选，并在唯一表来源中保留MYF19/23/26差异。不为隔离测试降低等级或MaxLevel，也不绕过完整Cfg.Validate。

独立 `summon-candidates.json` 为schema1、required_rule_version17、runtime_enabled=false，包含完整Skill17/Book25/Upgrades及PetDefs身份引用、两书/schema/三个编译器摘要；`pet-runtime-candidates.json`当前为schema2且显式RecallName。历史16的schema1不带新Name，混入新字段拒绝。NoPets新三键地图政策不允许冒充旧15/16两键合同。

源码位置：Crystal HumanObject.cs4399–4438（同名活宠Node/Recall先于材料、500ms）；MonsterObject.cs1334–1367（到期NoPets、异图Back失败回主人脚点、同图不强移）、734–736（出生动作2000ms）；Map.cs914–932（原图静态Front/当前图fallback与死主人取消）；Envir.cs997–1012（MP12+4及19/22/26），MagicInfo.cs18（默认CD1800）；MYF ApexM2_Magic.csv18、ApexM2_stditems.csv27（稳定17/书25）。不以MYF DuraMax1000推护符Count。

本轮只运行纯Python与生成/复验，不运行Go、数据库、Creator、preview/sync，不改任何PNG或保存布局。配置编译不等于17正常业务、自然养成或容量验收。初轮旧阶段断言失败在checks历史目录保留；最终命令、退出码和输入稳定性以checks/final为准。
