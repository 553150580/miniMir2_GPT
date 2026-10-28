# 宠物驻留与birth底座交接

本阶段仅做World居民和生命周期身份，rule15免费bootstrap保持。新增game/pet_lifecycle.go；11个既有game生产文件接入，新测试pet_lifecycle_test.go。没有改devconfig、Excel、协议、Store、材料或技能handler。

## 可调用接口

- `room.PetWorld worldRef`：唯一`room.Pet`对象实际所在World，既可描述活宠也可描述尸体；不随attachment隐式变化。正式initialize明确填当前World；仅旧手造测试room的零值沿当前World兼容，未知非零World不会回退。
- `room.PetLifeSequence uint64`：room生命周期的新生水位。规则15免费bootstrap保留原life=0；新安装从水位递增。retire将原Life纳入水位，跨nil/死亡/替换不归零；冷owner有新ExecutionEpoch。溢出拒绝。
- `r.worldPet(world)`：只返回该World的真实resident，包含HP0尸体；是否可攻击/治疗由来源业务再筛选。`r.petReference()`/`reference(r,r.Pet)`使用实际PetWorld。
- `w.installPet(r, world, pet) bool`：只接受已登记原room的World、handle2/kind2、Life0、正HP且池值合法的预建实体。若已有活宠或尸体，拒绝且不改变现场。安装时一次分配birth、入格水位和真实居民身份，不抽随机数、不提供训练数值、不扣资源。只校验地图身份和Contains；合法静态点/门/动态占格策略由调用它的来源业务负责，不能将此API直接开放给客户端。
- `w.retirePet(r, expectedRef) (*entity,bool)`：只对匹配原World/handle/Life的当前宠有效；归还Pet导航任务，保留水位，返回完整原对象后清owner单槽。不会杀死、复活或销毁原对象；正常死亡不会自动调用。**未来17不能将retire+install当成已经保留可见尸体**，必须先有审定的有界尸体承载方案。
- `w.relocatePet(r, expectedRef, targetWorld, point) bool`：只移交同一宠物居民，不移动主人、不换scene、不重建HP/MP/状态/birth，归还旧Pet导航任务。调用方已完成来源地形和落点规则；API只校验已登记World、Contains与身份/运动水位。旧World ref不能重放迁移。尚未在业务入口开放。
- `worldState.HasPet/PetPosition/PetLife`：历史位置提示，**不是居民登记**。实际Pet落格、安装与移交记录所属World；返回图只有同birth可复用历史提示，目的World已有resident时实际位置优先。新birth不继承旧尸体位置。

## 已接入的生产规则

`resolveInWorld`按原World解析Pet，不会在旧World注册Player。原World已释放攻击可命中原residentPet；观察不同World时不投影新scene命中帧。当前baseline、普通占格、cellObjects、友方目标/治疗/区域/Buff和Town导航占位均只看当前World的residentPet。

普通move/nearest/attack/commitWalk/Pushed不把另一World宠物当作当前World实体。Pet恢复和状态仍处理唯一对象，未新增任何宠物或World长协程。rule15正常旅行仍用原指针（包括原尸体）健康移交并更新PetWorld，保留所有临时现场；没有用新生API替换现有旅行。

## 留给完整17的工作

1. NoPets与静态ValidPoint来源规则；500ms原Map/front有效则原World出生，否则主人当前World/脚点；MasterDead拒绝；事件容量/到期批后缀整合。当前API能表达原World安装，尚无17事件。
2. 同宠Recall优先且不耗符，fresh才永久扣除；MP/CD、原材料回执unknown与一次临时效果，正常购书25/学17/购符装槽。护符包数与报价待唯一Excel审定。
3. 模板Fighter.Level与训练PetLevel/Max/Exp、每实体统计快照、训练经验与原确认killRoleEXP的捕获/一次award。当前fighter仍用Fixture.Pet，未填训练默认或假装多模板成长。
4. 非当前World的Pet AI/来源PetRecall：底座拒绝误用当前World寻路，尚不主动推进非当前World Pet AI；完整17需在正确时钟/NoPets与位置规则下迁移同一resident。不能仅留Pet在inactive World不动就宣称完整来源行为。
5. 旧尸体与新宠同时可见：当前原尸体在原residentWorld基线正常保留，安装新宠不会悄悄覆盖它。后续若转为World-owned corpse，需要显式容量/TTL来源输入、唯一基线handle（不能同屏两条handle2）、解析/目标过滤和原对象承载；不得直接采用未审定180000ms覆盖所有模板，也不得新增无界尸体数组。具体方案需root确认后实现。

本阶段没有Mongo/WS新业务测试，没有声称17、购符、训练、尸体列表、完整道士或自然成长已完成。测试证据见report.json；before与snapshot-before保留真实失败，首轮与final两轮通过输出分别保存。
