# 客户端接口与阶段范围

产品TS/PB接线已收束，等待配置agent FINAL FREEZE后执行最终整套模型/strict/协议及before/after/current冻结。本文件不代替最终报告，不宣布DB、Creator或17商店闭环通过。

- world.proto新增OwnedPetState，字段1–9按root锁定；SceneBaselineChunk.owned_pet=10，无新ID。统一task protocol生成双端PB和声明；协议静态当前69消息/140个64位字段/458边界通过。
- OwnedPetModel严格保留Long，Life0合法，HP正数且不超上限；训练nil兼容legacy，超额EXP及满级carry不改写；唯一正uint32召回ID。归一化拷贝隔离外部包对象，标识/训练/抑制均参与分块一致性。
- BaselineAssembler与NavigationCancelAssembler完整同scope/stateSeq后才替换Owned，空值与非空也不得混块；当前图实体按handle/Life/definition/HP/训练逐项对拍，异图Owned不得出现body。GameSession经同一组装器，旧Entry碎片不能挂回新角色；退出ACK也携当前map身份校验。
- SkillModel/AppRoot只按服务端recallSkillIds决定summon_pet的普通CastIntent；没有Own或列表不含17就走原材料RPC。两条请求均不上传出生格，方向是合法输入，前格由服务端选。NoPets独立禁召，不以NoTeleport/安全区推断。无符合法Recall不受材料开关或装符门禁误挡，客户端不造经济成功或预扣MP/符，不清导航。
- GameSkills使用保存Use与快捷；模式/NoPets变化即刷新，描述区分新召消耗与Recall不耗符。Owned异图信息供保存PetPanel显示驻留地图/跟随抑制/HP/训练；当前原宠仍可显示实体真实战斗属性/状态；实体详情继续按当前世界handle/Life重取，尸体不视为Owned。
- 10份保存scene/prefab、所有来源PNG与布局没有修改；新增OwnedPetModel.ts及独立meta，未导出新FX。当前主rule17仍22技能、legacy免费未训宠、17/护符关闭；正常购书/购符/自然成长并未因这套观察协议完成。

已执行的中途日志在intermediate/：models、24项定向及strict通过；其后增加实际public17关闭候选合同测试，只作配置内容约束，不使用候选Count冒充主pack。最终完整运行以随后report.json为准。源完整规则与尚未运行的正常WS路线见../source-review/review.md。
