# 城镇往返、健康现场移交与安全区绑定实施建议

日期：2026-10-29。只读审查结果，未修改生产代码。本文件是下一阶段实施建议，不表示城镇往返、绑定或21已经验收。产品优先级以根策划、架构适配登记及用户最新要求为准；所有新门禁/预算只维护唯一Excel，再导出schema和配置。

## 结论与实施顺序

可以先开放正常进入Town、在Town行走并经已核安全区业务取得绑定，不必先认证随机新生池。**城镇入口、到访安全区绑定、随机新生池、21施放资格是四件不同的事。** 不得为了打开安全区绑定，把StartPoint.txt全部行或当前个人出生格塞进initial start_points。

建议分三个可独立验收的步骤：

1. 完成同owner个人图↔Town健康往返、增量有界导航、地图加载与ACK、断线边界。Town使用已登记完整700×700目录2、开发入口(633,613)，不刷怪、不收费；入口不是新生池，也不等于银杏安全区中心。
2. 打开明确独立的安全区更新门禁，保留新生初始化关闭、起点池为空；新角色可以暂时binding缺省。正常到访并完成源触发后，把安全区中心作为永久业务事实提交。
3. 来源21的200ms、落点候选、占位及TemporalFlux单独验收。仅服务器确认为有效绑定的角色可施放；无绑定角色仍能正常进城、行走和取得绑定。不得退化为当前图随机空格，也不因21失败补初始化绑定。

## 已有结构与可以复用的部分

| 文件/定位 | 已有能力 | 下一阶段使用方式 |
| --- | --- | --- |
| `server/internal/game/runtime.go:91`，`room` | 同一个对象包含Role/Auth、连接、永久Pending、角色/宠物、怪物/掉落和计时器 | 保留其角色owner身份，拆出地图现场；不能复制第二份可写Role/Auth/资产Pending |
| `runtime.go:274`，`worker.loop` | role固定worker、控制/完成回调/命令与tick串行 | 同owner往返仍由原worker串行提交；不是永久owner迁移 |
| `runtime.go:529`，`join` | 仅`fresh := Scene==0`时调用initialize；健康重登保留既有实体 | 复用其附着握手顺序，不能重新调用Store.Enter来实现同连接切图 |
| `runtime.go:599`，`enterLineResult` | 已从当前MapID输出真实room_id/scene_kind/位置 | 新图准备完成并在owner实际提交后复用此响应内容 |
| `runtime.go:607`，`initialize` | 冷建primary，重建角色满HP/MP、宠物和8怪 | 仅明确冷建/故障初始世界使用；健康往返禁止调用 |
| `runtime.go:377`，EnterWorldAck处理 | scene+baseline匹配后Ready、补当前基线/资产/模式、SyncReady | 保留，额外校验当前附着/转移阶段；旧ACK不能激活新图 |
| `runtime.go:711`，`baseline` | 包含Life、HP/MP、运动水位、状态、模式和导航 | 可直接表达Town的角色/现存单宠；Town不能保留个人Monsters切片或MonsterCount |
| `game/map_context.go` | 当前room地图查询、显式未知ID拒绝 | 全部新移动/导航/传送/落点校验沿此目录，不新增全局地形旁路 |
| `persistence/auth.go:210`，`Store.Enter` | 新连接的票据消费、围栏、session推进及可选绑定补齐 | 每次新连接仍走此路径；同连接、同owner普通切图不消费新票据、不推进SessionEpoch |
| `client/.../GameSession.ts:44` | 新EnterLine重置assembler/scene观察，按scene/baseline收基线和SyncReady | 仍用Long/BigInt；增加世界附着协调器响应这一事件 |
| `client/.../WorldAssets.ts:70` | 按目录mapId选个人图或SourceMapAssets | 可按真实目标目录预备资源；禁止默认primary资源冒充健康重登Town |
| `client/.../GameWorldView.ts:59`、`:105` | 同一保存布局支持两种投影；prepareViewport等待权威位置对应的来源首视口 | 重建地图表现对象而非重建服务器现场；首视口完成后才ACK |
| `client/.../SourceMapAssets.ts:143`及`SourceMapView`订阅 | 资源换代/销毁前同步解绑，旧异步结果隔离 | 复用；先解绑HUD小地图及所有Sprite消费者，再释放旧资源 |

`client/...`均指`client/assets/scripts`。需要新增的不是另一个数据库owner，而是“当前地图现场”“个人保留现场”和“网络/客户端附着转移”三者的明确状态。

## 最小内存模型

保留一个角色owner、一个活动旅行角色/单宠，以及最多一个保留的普通个人现场。不要在Town复制一份满血角色/宠物，同时让个人图原角色继续产出；普通挑战并行是另一个已登记规则，Town不是新开特殊挑战。

推荐拆分：

- **RoleOwner**：Role/Auth、经济/来源序号、Pending/原job/原指纹/重试、Waiting、连接授权、托管账本、永久回执观察。往返不换owner，不创建第二份钱包/背包。
- **TravellerCombatState**：现存Player/Pet、HP/MP/死亡/Life、方向/动作、NextMove/NextAttack/ActionReadyAt、NextSpell/SkillCooldowns、Statuses、HealAmount/NextHeal、Potion队列/NextTick/NextUse/LastAppliedSeq、Melee模式/充能、施放和攻击观察水位。原对象或完整一次性移交，不能按配置重建。
- **WorldState**：MapID、实例/执行身份、怪物及死亡期限、掉落及寿命、动态占格/入格顺序、该图原角色脚点与原导航目标、Auto/敌方数量等个人设置。个人world在Town期间保留，无新AI/新战斗收益；Town无怪物生成配置。
- **Attachment/TravelState**：当前scene/baseline、Ready、转移ID及from/to身份、客户端准备阶段/超时、预览代次、导航任务代次。网络附着scene每次切换更新；角色Life不因换图递增。

个人返回点是**临时内存业务上下文**，不是角色永久binding。Town普通移动、返回个人图均不把当前位置写入永久存档。仅新绑定业务事实及其防重回执可入库；不保存HP/MP/CD/路径/怪物/掉落或临时世界快照。

角色CD、Buff和既有治疗/药效的时钟沿健康时间继续推进；换图本身不加满血蓝、不清药效、不重置药品使用水位。死亡宠物保留死亡，不借Town重召。Town宠物应跟随主人，不能因保存的个人Auto=true且无nearest怪物而永久留在入口（现有`combat.go`宠物分支在Auto=true时只寻敌）。清除旧图目标，按来源跟随/回归合法性处理；不要发明固定Town宠物出生点。

个人world的时间策略必须登记，不能由字段交换时偶然决定。建议保持原怪物RespawnAt、LootUntil等原截止，不重设为“回图后10秒/新掉落寿命”；停AI期间不回放攻击或补造收益，到期只按既定规则处理一次。若产品选择冻结个人世界的复活/掉落寿命，应另行明确使用个人活动时钟，保存剩余量并只调整world计时，不冻结角色CD/Buff。两种策略不能混用或称已由来源自动确定。

## 往返状态机、协议与ACK

最小可靠流程是“目标资源预备→原owner一次提交地图移交→现有基线ACK”，不是数据库两阶段提交。建议增加有界内存TravelPlan：稳定travel_id、原连接Binding、from_scene、目标目录/合法位置、原角色Life、owner/配置身份、期限。协议只允许“到已配置Town/返回原个人world”，不接受客户端任意map/x/y。

1. **预备**：客户端提出目的地意图；服务器校验原scene/绑定/存活/授权和目录，返回目标目录与合法准备位置、travel_id。此时旧世界仍是权威活动世界，客户端预加载目标地图/common资源及准确首视口。准备成功不授予传送权。资源失败/取消/超时只撤销此plan，原世界不重建、不退款（无费用）。
2. **提交前复核**：客户端带同travel_id确认资源准备。owner重验原连接、scene、Life、状态/配置及目录；未完成永久Pending或unknown时不能提交。进入短暂quiescing阶段，停止新动作/AI调度，按真实时钟自然处理已释放的Casts/Attacks及其后续格，不提前结算、不丢已发生KillFacts。致死、围栏失效或不可收束则中止/隔离，不能借进Town复活。
3. **内存提交**：全部旧场景事件和持久结果已确定后，保存个人现场、移交原Traveller到目标world，更新当前MapID/占格/新scene与baseline，Ready=false。原个人world不丢。same-owner切图不改Auth/SessionEpoch、钱包或经济命令序号。
4. **附着握手**：发送对应新scene的EnterLine/SceneBaseline；客户端只挂接该travel/scene的WorldAssets和GameWorldView，确认资源、首视口、基线匹配后发送EnterWorldAck。服务端匹配scene+baseline+当前plan再发SyncReady。Ready前只允许规定握手，拒绝新移动/技能/经济意图。
5. **终态与重复**：同travel_id重传返回同一确定状态，旧确认不能再执行一次；新travel只能在上一plan结束后创建。提交后客户端加载/ACK失败不能默默回滚服务端到旧地图；保留当前健康world，重连后同步它。若另做回滚，必须是新一次有身份的健康移交，不是initialize。

资源准备与旧世界实际移交应分开，避免700图首次块/纹理加载占满现有10秒ACK期限。既有ACK超时不是变更授权，也不证明角色需要冷建。

客户端必须新增按`(binding, travel_id, scene_epoch, baseline_id)`管理的地图附着协调器：

- `core/scene/SceneManager.ts:64–69`对同名Game直接返回，不能靠再调用`switchTo('Game')`重新挂图。
- `app/GameWorldView.ts:54`构造时固定sceneEpoch，`AppRoot.ts:184`只构造一次。只推送新EnterLine会让旧world保留旧scene/投影。
- `AppRoot.ts:233–238`现在只等baseline便发ACK；Town需先`await world.prepareViewport(权威位置)`并确认`isMapReady`，再ACK。旧准备回调必须销毁自有资源，不能挂回新图。
- `AppRoot.ts:137–149`选角取票前默认预载primary；健康重登Town时应保留common预加载，再按真实Entry.roomId选择地图资源，不能把这一默认预载当Town已准备。
- `GameSession.ts:44–62`可复用scene/baseline收集，但必须触发上述重新附着流程；不能仅清session数据而不卸旧world。
- 同一保存Game场景可只替换地图表现、更新HUD小地图与资源归属；加载UI仍使用保存的scene/prefab。不能生成新固定UI，也不清客户端永久PendingOperations。

## 已释放事件、回调与unknown边界

现有`actorRef`使用scene/handle/life（`timeline.go:179`），旧scene事件无法直接作用新scene。最小实现先自然收束队列，再换scene；不能简单清空Casts/Attacks当作“隔离已完成”，也不能把旧事件actorRef全部重写为新scene。后续若要保留跨地图飞弹/地面效果，需要独立世界执行身份和目标规则，属于另一项明确工作。

`combat.go:454`的settle回调会把成功q.Item临时显示在当前Player.Pos（`:482`）；`runtime.go:840`通用asset回调也直接操作当前room。现阶段依赖“Pending/KillFacts未收束不切图”保证它们不会跨场景落物/用药/复活。进一步拆owner/world时应参照`casting_material.go:90`：

- 已提交永久Role按角色与revision更新，不因旧scene回调而丢弃资产事实。
- 临时药效/施法/掉落必须匹配原world、scene、life与一次性水位，旧回调不清新代Pending，不复用新连接Request。
- `skill_assets.go:45`报价已有连接/Auth/scene/代次校验，可在转移时推进PreviewGeneration使旧报价失效，不必让纯报价占永久Pending。
- unknown保留原op/指纹/候选/原job及必要预留，用原授权对账；暂时查不到、加载超时和scene改变均不是重新扣费/发奖/退款或释放预留的依据。缺终态时维持busy，不能通过转Town逃离这条约束。
- `potions.go:83`按原scene及LastAppliedSeq防重；必须先应用确定回执再移交完整药效队列，避免跨图掉药效或重传再喝一次。

Town断线不能沿`runtime.go:351`无条件开HostSession，也不能沿`combat.go:301–320`仅凭Conn==nil扣时。Town转健康OFFLINE、不刷怪不产出不扣托管；个人保留现场仍占有界内存。新连接照常account取票并复用Town现场/原个人world。缓存压力、重连失败、地图加载失败均不能伪装成故障丢现场回血。

Town需在服务端拒绝个人敌方数量操作：现`runtime.go`的AutoBattle处理会直接把MonsterCount改为请求值；Town若保留空Monsters却接受数量1，后续切片会越界。个人Auto偏好保留在个人world，Town只运行所需跟随/移动，不借Auto入口刷怪。

## 来源安全区和绑定的精确语义

以下只是本地Crystal源码与MYF数据的可追溯行为，不代表官方1.80认证：

| 来源定位 | 精确事实 |
| --- | --- |
| `crystal/Server/MirEnvir/Map.cs:2386` | 按Info.SafeZones原顺序返回首个匹配区，不按距离选区、不对重复中心去重 |
| `crystal/Shared/Functions/Functions.cs:71` | `abs(dx)<=Size && abs(dy)<=Size`，含边界的Chebyshev方形 |
| `crystal/Server/MirObjects/PlayerObject.cs:1057` | SetBind从Envir.StartPoints列表随机取初始绑定；这个列表的来源/权重必须另证 |
| `PlayerObject.cs:1064` | SetBindSafeZone保存安全区Location中心和当前地图，**不检查StartPoint**，也不保存当前脚点 |
| `HumanObject.cs:2480/2605` | 成功Walk/Run后查询安全区并调用SetBindSafeZone，区外更新InSafeZone=false |
| `PlayerObject.cs:4264–4284` | 合法Turn且CheckMovement未提前返回时也会更新安全区/中心绑定 |
| `HumanObject.cs:6985–7028` | 成功Teleport后查询落点安全区并更新中心绑定；失败传送不执行 |
| `HumanObject.cs:2649–2720` | Pushed在正常退出循环且result>0时进行安全区/绑定后置检查；`:2658`无效静态地格会直接return result，可能保留成功前缀却跳过该后置检查。若本作选择对全部成功前缀统一刷新绑定，须登记为适配，不能冒称来源逐分支相同 |

本作目录2当前安全区顺序是106、107、108、113、117；前三行同中心(650,631)、半径8，仍保留三行。另两中心为(289,618)、半径8及(331,267)、半径10。开发Town入口(633,613)在银杏安全区外，进Town本身不会凭空取得银杏中心绑定。

**安全区不是由“关闭门”或“不刷怪”推导的统一伤害免疫。** `MapObject.cs:428–453`泛型MapObject入口检查双方InSafeZone，并有训练AI56例外；但存在不同静态参数类型的重载：`PlayerObject.cs:4650`的Human攻击过滤双方安全区；`:4675–4681`的Monster重载中，特定AI有独立分支，`attacker.Master==null`会先返回true，受控宠物分支才检查本人/宠物/主人安全区。`MonsterObject.cs:2150–2156`实际Attack会以MonsterObject参数调用目标重载。因此不能在applyDamage统一写`safe => damage0`后声称复刻源行为；持续伤害、地面效果、宠物和各受击路径要分别追踪测试。Town无怪是当前内容范围，不是这些伤害路径已验证的证据；野外PK仍按本作关闭。

观察安全区几何归属可以实时由`MapTemplate.SafeZoneAt`计算，避免复制长期失效的布尔值；但“触发永久绑定”仍须按已审定动作语义办理，不能每tick盲写数据库。治疗/Buff是否允许与攻击目标过滤分开，不用“安全”名称一并禁止。

## 初始池未认证时的安全区绑定与21门禁

当前`devconfig/binding.go:66–90`把Enabled=true与非空StartPoints绑定，`persistence/binding.go:43`随Enabled初始化；所以**现合同不能直接打开safe-zone更新而保持初始池未知**。下一版应在唯一Excel/schema/双端配置中明确拆分“初始绑定初始化门禁”和“安全区更新门禁”，不补隐含默认、不放宽旧版本完整字段校验。

要求：初始化关闭时start_points为空，CreateRole和缺绑定的Enter不造值；已有合法binding始终保留。安全区更新打开时，服务端核验当前owner/scene/life/合法动作、当前地图、匹配区及中心可走，然后以稳定事件ID+指纹、围栏、revision/CAS、永久回执原子写`binding.map_id/x/y`。未知提交继续原ID对账，不能开新事件再写。客户端仅观察提交后的绑定与角色revision，不能上传任意绑定坐标。

21的“已验证绑定”至少须同时满足：完整严格存档、配置内正确目标地图、已核安全区中心身份/可走地格、服务端可信取得路径。不能仅因`binding!=nil`或`ValidBinding`在任意地图返回Walkable，就把个人图/历史任意点当作可施放绑定。本开发库运行绑定此前关闭，新取得绑定可限定唯一来源为安全区提交；若出现缺取得证据的历史绑定，保持原事实、不开放21，允许角色正常步入安全区完成验证，不能擅自重设。取得证据可引用已有永久操作/审计回执，binding对象本身仍只存map_id/x/y。对于“同中心但尚缺验证证据”的触发，不应因值相同直接绕过验证回执。

21的来源是`PlayerObject.cs:4633–4648`：目标为BindMap；各轴范围为完整目标图宽/高整除(rank+1)，每次以绑定中心加随机偏移，最多200次尝试Teleport，不是保证落在安全区，也不是保证落到精确绑定中心。`HumanObject.cs:3537–3539`为200ms延迟；`:6095–6107`到期还检查NoTeleport，成功才添加30秒TemporalFlux及30%传送耗蓝惩罚来源状态。后续实现要核对费用、动作、失败、占位与状态全过程；不能从“绑定先可用”直接宣布21通过。自动/手动入口资格一致；未绑定的学习/购书是否开放须另明确，不从本方案推导。

## 700图导航：已实测阻塞，必须先改成增量有界

当前`combat.go:69–117`每次move新建queue、seen和first两个哈希表，最多490000格、每格8方向；free还构造实体集合。遇不可达目标不推进NextMove，Destination保持，下一tick又完整重算。即使可达，也只保留第一步，下一次走路再次搜全路。

只读探针直接调用当前worker.move，使用Go overlay，物理server目录未写入临时文件。报告`.work/town-nav-probe-20261029/measurements.json`；fixture SHA为`28727e18f02587027c448f6ea6dbe4030bbe41ac33cae1899ba72ad52dba85cb`，Go1.27.3。每情形3次：

| 输入 | 同步调用耗时 | 每次总分配 | 结果 |
| --- | --- | --- | --- |
| 真实Town (633,613)→(289,618) | 83.0–93.3ms | 约47.45MB | 前进一步 |
| 真实Town合法但不可达(396,47) | 205.1–235.8ms | 约101.06MB | 不移动；起点连通区327165格 |
| 独立700空图、隔离合法终点、单宠（非运行地图） | 353.4–371.5ms | 约190MB | 不移动 |

`worker.loop:288–291`在一个tick内串行遍历本worker所有room，时钟为100ms。一次真实不可达查询已超过两个tick；缓存图格预算490000只限制总工作量，不能限制owner的单次停顿。这些数字不是最坏墙钟保证、混合负载或容量验收。

最小建议：

1. 地图加载阶段预计算只读静态连通分量，严格采用当前8向及斜角规则；不同分量的合法目标立即给出不可达，不能进入全图搜索。标记数组可用每格32位，700图约1.96MB，不能给每角色复制一份。
2. 地面目标只创建一项带scene/life/导航revision/配置hash/起脚运动水位的NavTask。A*使用Chebyshev启发式（当前8向各步同成本），用索引数组/有界复用arena存g/parent/访问代，不为每次步进建两份Point哈希表。成功后缓存整条路径，普通走路只核验并消费下一步。
3. 将搜索拆成可取消的小批扩展；**worker整体**设每轮导航扩展/时间预算，不是给每个room各一份无限预算。具体批量、总截止、并发任务数/队列上限作为待实测候选登记在唯一Excel，不能凭空写入硬编码数值或声称已有容量承诺。
4. 可由共享有界导航worker执行，但输入只能是不可变地形和小型占格快照；不得把可变room指针交给异步搜索。结果回原owner，原scene/life/目标revision/配置/起脚身份不匹配即丢弃，重验下一格占位。新目标取消旧任务并归还arena，不占永久经济Pending，也不冻结战斗。
5. 宠物等动态占格使目标暂不可进入时，用局部重规划/等待占格变化及有界退避；不要把“走到相邻但尚未到目标”变成每100ms再扫全图。保留原导航目标与明确planning/blocked/unreachable观察，不假发arrived。摇杆单步仍O(1)邻格检查，不走全图搜索。

## 下一阶段关键验收

- 受伤/耗蓝、死亡宠、未满Buff、技能CD/动作锁、药品恢复余额与水位、模式充能，往返及Town健康重连均不重置；合法时间推进单独计量。
- 原个人怪物受伤/死亡/原复活期限、原掉落、1–8选择及导航目标保留；连续往返不重生整批、不补发收益；Town数量输入不能造怪或越界。
- 旧释放事件自然收束、致死中止、逐杀提交竞争、材料/药品unknown与迟到回调；全部沿原身份，无跨图落物/重扣/重用回执。
- 加载失败/取消、同ID重复、过期confirm/ACK、换代、断线及Town重连；旧scene不能激活/修改新图，客户端未准备首视口不能ACK，地图失败不调用initialize。
- 安全区边/角/边外、重叠首匹配、合法Turn、失败移动/失败传送、Pushed正常后置与早退差异；绑定中心而非脚点，区外不改绑。
- 未认证初始池保持空；无绑定角色可正常到访和步入安全区，提交后资格变化；任意可走但非可信绑定点不开放21。Town无怪与关门不作为统一伤害免疫验收。
- 同worker多个个人world+Town增量导航+DB回执混合调度下，测真正最大停顿/分配/队列上限，再确定导航预算；当前BFS数据只作必须改造的证据。
