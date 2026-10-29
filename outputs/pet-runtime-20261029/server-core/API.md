# Rule16宠物尸体、训练及基线交接

当前只启用宠物死亡/原World尸体/TTL与观察。免费bootstrap仍在，显式LegacyUntrained、Train=false且Fighter冻结为原Fixture.Pet；17、购符、正常训练宠物获取入口仍关闭。

## 调用接口

- `w.newPet(r, definitionID, rank, point, spawnNow) (*entity,bool)`：正式训练定义factory，DefinitionByID返回Clone，FighterAt冻结初始/最大训练级和实体统计，首次ActionReadyAt取实际Spawn时刻+显式SpawnReadyMS；不安装、不消费资源。未来17在实际500ms落点确定后调用。调用者仍须执行已学、NoPets、MasterDead、源静态ValidPoint等技能规则。
- `w.canBeginPetBirth(r,now)`：显式预算/无活动宠/无在途预约/非Pending非Isolated/身份水位可分配，处理已到期尸体后判定。没有1800ms新guard。
- `petBirthToken{ID,ExecutionEpoch,PlayerLife}`；`reservePetBirth`同token幂等、不同token拒绝；`cancelPetBirth`只释放匹配原token。预约包括在retained body预算中。
- `w.installReservedPet(r,world,draft,token)`：安装匹配预约的factory草稿，原子转换预约→活宠名额、分配动态handle与单调Life并清预约；未知永久提交未完成前不能调用。
- `w.installPet(r,world,draft)`：无预约的受信单步安装，规则16要求factory状态及Handle0；只用于明确受信业务/隔离模型，不能绕过普通技能入口。若有预约，必须走installReservedPet。旧规则15测试仍兼容原Handle2/清槽合同。
- 动态handle高水位`room.PetHandleSequence`不回收；跳过player1、全部八个预分配敌方10..17、掉落1000以及全部真实居民handle。bootstrap2保留，下一birth通常3，Life独立递增；溢出拒绝。
- `w.archivePetDeath(r,world,expectedRef,now)`由真实applyDamage致死分支调用：原*entity原地转`world.PetCorpses`，保存原handle/Life/World/属性/状态，IsPetCorpse=true，固定绝对ExpiresAt，清唯一活动槽和导航。死亡不因名额满而删除尸体；出生准入预留未来尸体名额。
- `w.expirePetCorpses(r,now)`：两个World有界清理，Pending先冻结、不延长绝对截止。移除原registry和其对象拥有的未来动作；不唤醒inactive怪物AI，不刷新怪/掉落截止。原对象退役后旧引用无效。
- `r.petBodies(world)`：活动宠+该图原尸体；`worldPet`仍只活动宠。baseline/旧ref解析包含原尸体；活动索敌/治疗/友方/占格不包含无主尸体。

预算为`PetRuntime.Budgets.RetainedBodyCapacityPerOwner`，两个World共享：尸体+活动宠+在途新生预约≤上限。满额拒fresh，不丢最老尸体，不拒合法死亡。当前唯一Excel候选128不等于GPU/内存/吞吐验收。

## 训练确认

`capturePetTraining`在原killFact捕获时，冻结同主人当前World、存活、范围内的原训练宠ref；不要求最后一击。LegacyUntrained与满级不捕获。`preparePetTraining`在原队首q排号时绑定SourceSeq、OwnerEXP和安全乘算结果；`finishKillFact`仅在原job成功的finished closure中一次award再移除队首。unknown保持原q/credit，迟到旧Auth/旧Pending仍由原settlementBinding围栏拒绝。

`findPetBirth`识别同owner原出生：可以是同birth健康迁移后的活动宠或保留原尸体；不给当前新宠补旧经验，Despawn/冷owner不恢复临时宠。一次grant只升一级并保留超额，Max时不累计，成长不回血。新queued-credit极值已修：前一grant升满后，后续预先捕获credit绑定原q但Amount0，不对本应忽略的经验做溢出加法。

`fighter`/Pet AI间隔读取该实体冻结Fighter再叠状态。模板Level与TrainingLevel/Maximum/Experience分离；legacy仍原Fixture.Pet统计。PB23投影训练，PB24标记NoMaster尸体；max next=0但保留carry，无独立经济command/持久pet快照。

## 基线分块

`splitBaseline`先构造全部合法块再发送，实际wire头+protobuf编码≤65536；同一次快照保持scene/baseline/stateSequence、navigation/safezone/modes一致，handle跨块唯一。单实体过帧、重复handle、超过双端64块/2048实体合同整体拒绝并隔离，不截尸体发半套。正常小基线仍1块。

客户端已由rules配合取消/leave完整组装，不再假设1块；ACK仍要求完整组装。现有WS累计256KiB/128消息背压保持，不据结构分块承诺慢连接或容量通过。

## 测试前态与历史范围

常规observedWorker默认当前rule16，手造旧宠补合法Def及legacy_untrained快照。仅4个A38历史方法显式`legacyPetRule15`：旧retire/旧手造HP0尸体随行/旧清槽身份测试，名字以Legacy标记。snapshot两个方法已改当前16真实死亡归档+正规factory；绑定Pending旧事件杀宠断言改查原World尸体。

新模型测试的预算4/TTL1000是明确隔离前态，另有真实fixture16与独立rawdefs投影一致/原bootstrap统计不变/配置TTL测试。128身体真实baseline样本和PB极值wire分块分别记录；不混称正常17、真实Mongo训练或手机容量证明。

未实施：17 handler/500ms Map-owned派生、NoPets/Recall技能规则、材料永久扣除后的17一次效果、购符/书25正常获取、主动重召/完整宠物AI来源认证。未来17应复用本接口并补正常Mongo/WS闭环。
