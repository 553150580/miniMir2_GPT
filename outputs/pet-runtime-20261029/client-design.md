# 宠物尸体与训练观察客户端只读设计

2026-10-29。只读当前rule15协议与客户端；仅dev包装的读取/rg命令，不修改生产/测试，不跑测试、Creator、服务或素材导出。技术候选corpse cap128/TTL180s尚待唯一Excel与来源范围确认，不能写成17已开放或128尸体实机性能通过。技能17入口、真实购符仍未启用。

## 结论与最小接口

旧尸体与新活宠只要使用不同uint32 handle，当前基线组装器与视图可分别保留；同一handle不同Life不能同时出现于同一基线。当前SkillModel自动实体选择不会选HP0尸体。训练后的实际战斗属性、HP上限、移动及攻击间隔已经有权威PB字段；不应由客户端重新计算成长。建议仅给EntityState追加嵌套PetTrainingState观察，复用完整基线的身份/顺序，不增加独立训练S2C或永久经济水位。

草案（确切命名/号由root统一生成）：

```proto
message PetTrainingState {
  uint32 training_level = 1;
  uint32 max_training_level = 2;
  uint64 experience = 3;
  // 当前训练级升下一级所需总门槛；达到max时为0，不能作为本次可自行升级授权。
  uint64 next_level_experience = 4;
}
// EntityState现有最后字段movement_kind=22；追加，不改旧号。
PetTrainingState pet_training = 23;
```

训练级和模板Level15分开；可从0级开始，max不得低于当前级，不在客户端写死最高7或系数3/20000。EXP与门槛从codec到模型/字符串显示保持Long/BigInt，禁止Number窄化。满级可保留来源超额EXP，不强置0。来源每次PetExp只升一级且保留超额，因此合法experience可以仍超过下一门槛，不能强校验experience<next_level_experience，也不能客户端while升级。非宠物缺少pet_training合法；新实现支持的宠物须由服务端明确给出，尸体如携带训练状态只显示死亡时保留值，不继续本地训练。

嵌套对象已经受scene_epoch、baseline_id、state_sequence及实体handle/Life保护；不需重复role/scene/Life或独立revision。训练字段不改Inventory.revision、SourceSeq或command。健康入场直接显示新完整快照、保持静默；冷重建按临时现场政策显示服务端实际结果，不从浏览器或旧基线复造宠物/EXP。若后续希望升级提示，只对同一附着与同handle/Life的新完整基线level上升发一次；初次基线/健康重入不回放。

## 现有容量与句柄事实

- shared/proto/world.proto71–100：entity_handle为uint32，life/movement_revision为uint64；训练可用新字段23。159–169支持基线chunk_index/chunk_count，训练放在实体内可自然跨块。
- BaselineAssembler.ts18/44–46：最多64块、单块和整份最多2048实体；跨块重复handle直接抛错。当前没有宠物数量为1的客户端硬上限。解码uint32保证网络handle范围，未来服务端分配器仍须保证1..0xffffffff、与玩家/怪物/掉落互不冲突，溢出拒绝；不要仅靠TTL重用句柄掩盖旧Life身份。若允许同执行代复用，selectedTarget等缓存也须升级为handle+Life。
- GameSession.ts65–71只接受当前scene/baseline更高stateSequence的完整基线，实体对象原样交付，新增嵌套字段不需另建消息codec。GameConnection.ts42–51的通用命名转换保留Long；消息5仍为SceneBaselineChunk。
- 当前server runtime.go781固定chunk_count=1，803逐实体编码；客户端GameConnection.ts60/65和server wire.MaxFrame/ws.Deliver限制单帧65536字节（WS发送排队总预算256KiB）。128尸体低于2048数量门槛不证明完整statuses/attributes/EXP最坏包长合法。须测真实PB编码大小并有界分块，不能靠吞实体/剥掉源仍有效状态降低包长。
- 若服务端开始分块，AppRoot.ts891–892的退出导航取消确认目前硬要求chunkCount===1。普通入场路径已完整组装；退出路径必须同样等完整当前请求/scene/baseline/序列的取消确认，再判断navigation终态，不能取第一块或只删掉断言。保持原匹配及旧scene隔离。

## 视图、资源与尸体

GameWorldView.ts183–253按handle建visual；profile以definition/job查表而非handle2。死亡保留原node，HP0交EntityAnimation显示倒地；初次收到尸体时EntityAnimation初始corpse直接取die终帧，健康重入不重播死亡。新宠新handle自然另建visual，旧corpse不会被新Life替换；TTL到期只能由服务端后续完整基线移除，客户端不能自行计时删除。当前SourceMapView排序id使用entity_handle，也支持多个不同宠物/尸体。

最近外观生命周期修复已使同handle变definition/profile能正确替换，并在退役前同步清Sprite帧；本轮不再改该实现。WorldAssets.ts130–155已预载本职业所有pet_profiles动作，所有尸体与活宠共享该世界持有的SpriteFrame，不为每具尸体另加载整套帧。17骷髅可用现有正常身体动作；appear素材/动作仍是独立待核事项，训练并不授权导出新外观。

当前视图没有实体viewport裁剪：所有基线实体都建动态Entity模板，每帧sample、setPosition并参与深度排序；来源地板的chunk缓存不会自动裁掉这些实体。128个尸体意味着约128套Body/Name/HP/AttachedEffects子树，加上活体与地图draw；不能从2048协议上限推算GPU、触摸或内存承载。当前尸体仍显示名字和空HP条、可触屏打开实体详情；密集叠尸需真实Creator检查地面点击/详情是否可达，不能以source map LRU通过代替。无新固定布局需求，必要的动态尸体显示策略另据实际画面选择。

## 选择与状态显示的实际行为

SkillModel.ts209–224先要求HP>0再按友方(本人或Kind2)/敌方(Kind3)选择；手选尸体会提示“所选目标已倒下”，不会自动治疗尸体。AppRoot.ts807–808普通攻击也只选存活Kind3。没有自动查找第一个Kind2就当活宠的施法路径。14/15是friendly_grid，走AppRoot.ts755–764/validateGroundTarget，不应把实体治疗的HP门禁泛化为禁止源尸体Buff。使用17应继续server-owned friendly_self，不向客户端开放出生格或指定宠物。

GameCombatStatus.ts52–55的概览则确实遍历所有Kind2，包括尸体且统一标“宠物”；128具含状态尸体会产生重复长列表。最小可复用UI方案是普通概览只列活宠，尸体仍可点实体查看其实际14/15保留状态，并明确“宠物尸体”身份；也可以保留尸体分组，但不能把同名多尸体误当多只活动宠物。状态详情本身已按handle+Life查找（58–61），StatusPlayback也按scene/handle/Life隔离，不会把旧corpse盾迁到新宠。

## 动态训练属性与最小客户端改动

world.proto89–95已有hp/hp_max、move_interval_ms、attack_interval_ms、combat_attributes。CombatStatusModel.ts8–16/37–41可原样显示实际物攻/防御/魔防等Long，GameWorldView每次基线使用新hpMax算比例，不能升级时本地回血。宠物普通走的显示帧长仍按A29资源walking时间，服务器攻击/移动间隔是业务节拍，不把成长后的interval直接拉伸原动画。

现有AppRoot.ts366–372实体点击详情是一次静态lines；GameCombatStatus.showDetails65–67将detail清空，仅专门的状态详情会随基线刷新。因此训练进度不能只在一次点击时拼字符串并宣称实时展示。建议新增纯PetTrainingModel（可只提供严格校验/格式化/身份选择），扩CombatEntity可选训练观察，并让保存StatusDetails支持entity handle+Life的持续详情。每次新完整基线重取对应实体的HP/训练/实际属性，实体离场或Life改变时明确“已离场”；保留滚动与原Popup层级。AppRoot调用传实体身份；旧world回调不能更新新详情。

保存BottomHUD/Skills/PetPanel目前仅Help，可改为打开同一保存StatusDetails展示当前活宠训练/HP/属性，若无活宠显示“当前没有存活宠物”；无需新建固定控件。点击尸体则显示其死亡/保留状态与身份。暂不增加管理命令、训练按钮或材料商店入口。没有新美术依赖；训练等级用文字，原名字0.7缩放与HP绑定保持。

可能授权的最小文件范围：新增PetTrainingModel及tests、CombatStatusModel/GameCombatStatus、AppRoot实体详情与PetPanel绑定、transport/PB边界和必要GameWorldView多corpse实际组件tests；若真实PB包长需要分块，再修改退出ACK接收路径及对应身份回归。GameSession/codec通常不需生产逻辑改动，协议源及生成由root统一处理。保持scene/prefab/Excel/素材不改。

## 必要回归清单与范围

1. 128不同corpse handle +新活宠同基线，乱序分块完整/去重/2048边界、64KiB真实编码；首次尸体终帧、原实体死亡过渡、移除时clear frame后destroy。
2. 尸体不进入自动治疗/攻击；手选尸体反馈，14/15地面尸体语义保留；旧句柄/Life、旧scope、旧chunk不改新宠/详情/导航。
3. 大于2^53的EXP/阈值仍精确；单次升级后保留超额、满级保留、HP不回满；同一生命动态attributes/interval/训练详情跟随最新完整基线，初次/健康重入不刷旧升级消息。
4. 新宠和旧尸体同屏触摸、名字/HP/深度与资源释放；原固定布局和来源FX摘要稳定。上述组件/模型测试、真实WS、Creator画面和128压力分别记账。17入口/购符不因本底座与观察字段完成而开放。

## 后续来源澄清与授权实现合同

server-core补核Crystal Monster.Die1000清Master、IsFriendlyTarget2541起Master==null拒绝：新的World-owned旧宠尸体不再是owner友方。14/15的AllowDeadTargets不能泛化授权这些无主尸体；原玩家尸体例外及既有开发r.Pet HP0未脱主兼容须分开。root据此确认EntityState.is_pet_corpse=24，所有客户端实体友方选择与活宠概览排除此标记；14/15 grid上行仍只传地面点，服务端按来源选真正友方。pet_training=23及四字段已由统一task protocol生成。本节覆盖前面未区分尸体归属的早期表述，不扩大17/购符开放范围。
