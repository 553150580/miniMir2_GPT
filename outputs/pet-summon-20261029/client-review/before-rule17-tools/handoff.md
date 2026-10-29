# 客户端交接

最终report.json SHA256 `d7780960e5546475488b5b56a85d0d8e70554a815706c530a8ed8187b823b88b`。在最终preview/sync之后开始冻结：224份输入 before=after=current=current_frozen。实际命令、退出码、日志摘要均在report中；models、358项UI/app（0fail/skip）、strict、69消息/140个64位字段/458个PB边界及协议清单53全部通过。10份保存scene/prefab未改；ID列表无新增。未运行DB、Creator或浏览器，本报告不把组件替身当真实WS。

OwnedPetState字段1–9已生成双端PB，SceneBaselineChunk.owned_pet=10。完整分块元数据比较包含空/非空、生命/HP/训练、召回ID和抑制；同图实体交叉对拍，异图不造body。PetPanel以Owned为准并保留匹配当前实体的属性/状态；实体详情仍按原handle/Life/scene重取。Legacy训练nil不造级别。

SkillModel/AppRoot仅按Owned.recall_skill_ids选择普通召回；新召使用既有材料RPC，均不上传出生坐标。NoPets独立禁召。Use/快捷不因无符误挡合法Recall，不预扣资产、不造经济成功、不清导航。错模式与旧成功receipt是否绝不重生由服务端独立回归承担。实现细节见implementation.md。

主profile依旧22技能、legacy未训练免费开发宠、护符关闭；召唤候选runtime_enabled=false。358项包含对实际public17/关闭候选的契约约束，不能当正常商店/技能17GUI验收。root独立WS17的等级19、初始书25和Count2/包cap2均属明确局部fixture输入；主pack/售价、自然成长、17普通购书购符闭环未因此完成。

来源报告 ../source-review/review.md 与source-inputs.json：6份核心与CrystalLatest逐字匹配。完整Owner.Die杀现活宠已报并由root决定采用；500ms新生原图优先、Recall到期NoPets、Frozen/AutoRecall和2秒普通走攻锁范围均逐分支记录。朝向输入与捕获Front的首版差异已报root/server，不擅自改Go。未运行的后续正常协议获取设计单列，不能冒充本轮已执行。

中途日志intermediate/保留。最终本轮所有测试通过，不声称保存了实施前产品失败；pb-before.json为PB修改前实际采样，最终before为最终命令前采样。产品TS/PB现已冻结，后续实构/真实GUI由root统一推进。所有说明人工中文、相关代码/测试/meta为LF；没有改素材PNG、Excel或固定布局。

召唤升级预览补充：STAT_NAMES现认new_pet_training_level/new_pet_max_training_level/new_pet_material_quantity/new_pet_hp_max四中文键，mp_cost沿用；保存预览明确仅影响后续新生，已有宠Recall保留原训练。只有SkillAssetsModel及其对应测试在第一次224/357冻结后变化；旧13文件/report/log原字节在before-preview-fix/，最终5命令已全部重新执行，224输入仍稳定。Creator此前尚未执行，下一次由root统一实构；本agent不把类型检查当实构。
