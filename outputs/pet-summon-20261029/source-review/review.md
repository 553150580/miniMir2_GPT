# MYF17 / CrystalLatest完整路径独立审查

基点66cb2766cacfb0ed7358bd323a30d59dbbcddd11，2026-10-29。只读取玩法/技术白名单源码、现有客户端/材料链及文档；不运行来源程序，不读取Setup、账号、运营数据库或部署指令，不跑DB/Creator。下列“来源事实”不等于本作已实现；运行数字仍由唯一Excel决定。6份核心与本机CrystalLatest逐字相等，摘要见source-inputs.json。

## 来源身份与逐分支事实

1. MYF技能：资料参考/myf/Mud2/DB/ApexM2_Magic.csv:18为17召唤骷髅、职业2、学习19/23/26；stditems.csv:27为书25。stditems.csv:189的187护身符为Shape5、DuraMax1000、Price500，均不证明本作Count或“一包1000张”。Crystal GetAmulet默认Shape0，与MYF187是跨来源映射而非相同原配置。当前fixture仍casting_supply.enabled=false、187.max_stack=0且无17。
2. 通用CanCast：HumanObject.cs:152–158要求未死、ActionTime/SpellTime到期、无Stun/Dazed/Paralysis/Frozen且Mount.CanAttack；PlayerObject.cs:83–87另拒Fishing。它不是仅MP和技能CD判断。
3. HumanObject.Magic:3402–3416先CanCast/已知魔法/非零坐标射程校验；3418–3432移隐身Buff并推进AttackTime、SpellTime=now+1800及ActionTime（非冲撞）。3434–3447再检查专属GetDelay和MP。3450–3456才扣MP、推RegenTime、采用方向。因此来源某些CD/MP失败仍已推进通用动作锁，本作既有全预检后受理流程是已登记适配，不能称所有失败副作用完全相同。
4. SummonSkeleton是void（HumanObject.cs:3534–3536），不会把cast置false。故下列NoPets/容量/无符/模板缺失的return均发生于通用扣MP之后，Magic:3773–3779仍记录magic.CastTime并广播Cast=true。Recall同样消耗动作、MP及两层CD；“不耗符”不等于免费无动作。
5. 首先检查受理时主人当前图NoPets（4401–4405），然后遍历Pets按原Info.Name==Settings.SkeletonName、!Dead、Node!=null找首个（4408–4415），向该原宠ActionList排now+500 Recall并return。它发生在容量、护符及新模板查询之前；不同展示名、同definition_id或客户端看见一个Kind2都不能代替这项匹配。
6. 未Recall才查来源两个Monster名额（4417）；本作普通个人单一活动宠是明确策划例外，不复制两个名额，也不自动杀掉异名活宠来挤出名额。接着GetAmulet(1)→GetMonsterInfo→LevelMagic→ConsumeItem(1)（4419–4426）。GetAmulet:6859–6868只查Equipment、Amulet类型、Shape0及Count>=1；ConsumeItem:1346–1375减Count，余0移出装备/背包，不减DuraMax。
7. 新对象设置PetLevel=magic.Level、MaxPetLevel=4+magic.Level、Master、先写ActionTime=受理now+1000（4428–4433），捕获主人/对象/受理时Front，向受理原图ActionList排now+500（4437–4438）。不是先把宠加入Pets：4435的Pets.Add被注释。新生预建对象不能先进入baseline、占格或成为Recall候选。BoneFamiliar构造函数:10–13把方向定为DownLeft，并非继承主人方向。
8. Map.CompleteMagic:856–862先要求原主人非空、Info/Node仍有效；916–933再对召唤检查monster.Master.Dead。仍死亡则不出生；这里没有重查NoPets，也没有要求主人仍在原图。主人已经移图且仍有效时，原图capturedFront通过原图ValidPoint便Spawn原图，否则尝试到期主人当前图/脚点。不能把全部新生改为到期主人图，也不能把NoPets的Recall到期检查移植到新生分支。
9. ValidPoint:Map.cs:637–643只检查半开地图边界及Cell.Valid；Cell.Valid:2480–2482仅Attribute==Walk。Monster.Spawn:689–704、Teleport:938–962也只复用此条件，不查动态Objects，不查CheckDoorOpen（该方法另在Map.cs:646）。动态占格与关闭门不能悄悄加为17源判点门禁。
10. 来源边缘：Map:927–932忽略Spawn返回值并无条件Pets.Add；fallback脚点也不可走时可能留下Node为空的原对象。事实不能省略，但本作不能制造ghost居民/假出生；应明确失败终态并归还本次预约，是否保留已扣费用沿本作已接受施法合同记录，不能把未安装对象当活宠来“对齐”来源。
11. 真正Spawned把ActionTime覆盖为实际出生now+2000（MonsterObject:734–743），不能继续用受理时+1000。基础MapObject.Spawned:339–356先注册Node和BroadcastInfo；BoneFamiliar:15–25随后才Summoned=true。首次Extra=false，后续重观Extra=true；同birth健康重入不能重播出场。
12. 精确时钟：CanMove:634–648、CanAttack:650–665对ActionTime采用严格>；本作既有>=边界已在A29登记。+2000只拦普通移动/攻击，不拦所有AI或所有事件。ProcessAI:1671–1715不先查ActionTime，新生可先在原图出生，再于首AI AutoRecall至主人图；不能用“出生锁”把此分支冻掉。
13. 延迟Recall由原宠MapObject.ActionList在到期Process（MapObject:228–233、Monster:1325–1326）调用原对象PetRecall。PetRecall:1334–1374要求Master/主人图存在；按到期主人图NoPets且!IgnoresNoPetRestriction，设置Frozen=true、清Target、PMode=None并原地等待。允许时先清Frozen/Target、PMode=Both，只有异图才Teleport主人Back，失败再当前脚点；Back是到期主人位置与朝向的反向一格（MapObject:198–200）。同图不移动，但清目标/解冻等副作用仍发生。Teleport不重置原宠移动/攻击/ActionTime，不重造Life或训练。
14. 玩家进入NoPets图时ApplyMapEntryRules:PlayerObject.cs:1515–1545冻结已有未死且非豁免宠，离开禁宠图会解冻；首次调用点为登录1321附近、Teleport1486–1488及其他地图进入路径4595–4598。该地图入场动作与召唤法术里的NoPets不是同一个时点，不能互相替代。
15. AutoRecall:Monster.ProcessAI:1681–1715，Frozen且异图且主人允许宠物时强制Recall并return，不依赖主人当前PMode；Frozen且同图允许时先解冻。仍Frozen就返回；其后主人模式Both/MoveOnly/Focus且异图或距离超Globals.DataRange16时Recall。与主人同图但距离远时PetRecall仍不Teleport，不能误加“超16格就同图瞬移”。NoPets普通分支会在Recall本身冻结，不删除健康宠。
16. 主人真正死亡的完整链不能省略：PlayerObject.Die:629–633先Die所有现活Pets；Monster.Die:1000清Master；所以已排Recall通常对先死失主宠无效。当前rule16 game/combat.go:253–263只处理玩家DeathID/notice，未杀活宠，docs现只写新生主人Dead不出生；此差异已立即报root，root决定rule17采用完整Owner.Die杀现宠。脱场Despawn:MapObject:370–375清自身ActionList并杀Pets；MapObject.Process:206会清Node已失效的Master。不能只核PetRecall自身没有Master.Dead就允许尸体主人召回健康宠。

## 与现材料handler/客户端的必要分离

- server game/casting_material.go:18先硬要求16字节材料实例，35先CastingSupplyGate，skill_cast.go:190–192又拒普通材料技能。persistence/casting_supply.go:18–43按固定material_quantity检查并:63减数量，无法直接表达不耗材料的同名Recall。
- AppRoot:722–734所有material_quantity技能先selectedCastingSupply；CastingSupplyModel:26–32缺装备就报错。SkillModel:173“每次消耗”及GameSkills的Use禁用也必须区分新生/Recall。异图活宠不在当前entities，不能靠当前Kind2、展示名或definition_id推断免符。
- root已定乐观协议适配：权威Recall走普通CastIntent，新生走原CastWithMaterial；网络期间分支改变时按原请求失败回执拒绝，不扣符，不借旧成功永久receipt制造新Recall。客户端只能参考完整OwnedPetState，不自行生成成功回执或推进经济水位。单生预约、receipt一次生效、未知沿同job/指纹、旧Life/owner失效释放自己的预约仍必须保持。
- 新手默认免费宠将由显式bootstrap政策替换，不因学习17自动赠书或暗中安装对象；新生与旧尸体不同handle/Life，旧对象TTL仍继续，身体预算满额只拒新生。

## 正常协议隔离Mongo/WS验收设计（未运行）

本轮不启动DB。后续使用受控随机专属DB/RunID，3节点health.instance、端口及cfg摘要绑定，finally仅精确marker删自有库并恢复main；只读WS观察与普通协议输入。除明确隔离等级/余额资格外，不预置书/技能/符/绑定/尸体/位置，不把19级资格样本称自然成长。

A. 普通道士注册/创角，bootstrap=none，完整基线Owned为空。正常商店报价/购买来源书25及187、原书学习17、正常装入材料。Count从本轮唯一Excel派生fixture的明确商品数量及每次Inventory/receipt读取；将C0、每次消耗及每个instance/version逐项记录，禁止把DuraMax或“包”猜成Count。若唯一Excel尚未给Count/售价，正向购符验收保持未执行。
B. 新生：记录材料C0→C0-1与原receipt，MP只减一次；受理后500ms前Owned/实体不提前出现，首次可执行tick出生；延迟不为等待客户端资源修改。保留网络重复同op、pending/未知原job重试反例，不重扣/重抽/重生。主角仍活而原图Front被动态占用仍按源静态Walk出生；静态阻挡时仅原图→到期脚点fallback，不能随机找邻格。
C. 同名原活宠后移除/用尽护符，普通CastIntent仍合法Recall：Count/经济counter不变、MP/CD/动作推进、同图位置/Life/训练不变，原宠目标清理。提前下一次施放必须被通用CD/动作门禁挡住。过时Owned选择Fresh但权威已Recall，或选Recall而原宠已死，均验证错模式失败不耗材料，不将旧receipt转成新效果。
D. 原宠真实死亡并变NoMaster尸体，Owned清空；新生再次真实耗符，取得不同handle/Life，旧尸体仍可见并按原TTL退役。训练用确认EXP推进原birth，健康新票/城镇往返保留当前临时成长；不宣称冷重建恢复临时宠。
E. 已受理新生后主人在500ms前死亡、脱场或新Life，不能安装；完整Owner.Die同步将现活宠归档。主人移图时保原图捕获点优先；新生NoPets只按源受理点政策，Recall按到期主人图政策。普通旅行若收束事件才移交，不能拿该路线冒称已覆盖未收束异图分支，需独立受控game时间反例。
F. NoPets当前图新施放阻断、到期Recall冻结原宠且不删、离开允许图后解冻/AutoRecall；Frozen强制AutoRecall不因主人PMode关闭而漏掉。出生2秒锁内AutoRecall可发生，普通走/攻不能提前；来源严格>与本作>=边界另记。
G. 真实Creator/Chrome后续独立验收：保存技能Use/快捷、PetPanel显示同图/异图/抑制/训练，点击旧尸体详情与新活宠区分，真实触屏耗尽符后Recall可用，无新固定布局；不以组件替身冒充真实WS/GPU/128容量。

## 最小OwnedPetState S2C审查与已锁接口

方向可行，root已锁定并授权实现：OwnedPetState{entity_handle1:uint32,life2:uint64(0合法),definition_id3:uint32,map_id4:uint64,hp5:int64,hp_max6:int64,pet_training7:PetTrainingState?,recall_skill_ids8:repeated uint32,follow_suppressed9:bool}；SceneBaselineChunk.owned_pet=10。只指已安装/仍有owner的唯一活宠，预生对象/无主尸体不进入此观察，不传位置或展示名，不把异图对象塞entities。

需要整组同scene/baseline/stateSeq、相同Owned元数据（含空/非空、训练、recall列表、抑制）才原子替换；旧scope/旧sequence不能更新新scene，取消ACK也要校验它。MapID/Life/HP/训练EXP保持Long；HP>0且<=HPMax、标识有效、recall IDs唯一。当前图有同handle实体时交叉核Life/definition/HP，异图Owned不创建visual、不参与友方实体选择。PetPanel以Owned快照为准，当前实体详情仍按handle/Life实时重取。Legacy训练nil不编造级别，supressed只按服务端bool展示。Skill17只按server recall_skill_ids派生请求模式；NoPets只用于禁召，不借NoTeleport或安全区推定。

## 本轮阶段范围补充（root随后锁定）

主rule17继续legacy_untrained_pet、22技能及关闭的17/护符入口，不移除免费未训开发宠，不改旧NormalTown/尸体GUI前提。本轮独立WS17 fixture明确bootstrap none、等级19、初始原书25及护符Count2/包cap2，只验证受理后的真实协议及耗材/出生/Recall，不证明主profile的包单位/售价、正常商店购书/购符或自然成长。上文A–G是后续正常获取设计，未在本轮执行；不得混作本轮已经完成的业务闭环。

方向审查补充：源Human.Magic3456先采用合法请求dir再捕获Front；当前game首版prepareSummonPet捕获受理前r.Player.Direction，召唤input direction未被使用，已报root/server确认采用或登记差异。源新BoneFamiliar自身朝向另为DownLeft=5，与主人Front方向分开。
