# 下一P0客户端与素材只读审计

审计日期：2026-10-29。范围：当前rule15、保存Game.scene/Entity.prefab、客户端代码、唯一导出public及独立来源副本。未启动Creator或服务，未生成/导出美术，未修改生产代码或保存布局。本报告是实施边界，不是新玩法验收。

## 建议闭环

优先完成道士MYF17/书25的正常购书、学习、耗符召唤、单宠经验成长、宠物死亡后正常重召。它能替换当前“按职业免费给骷髅”的开发机制，同时串通已有经济确认、临时战斗实体及健康重入。隐身/施毒还分别需要真正可见性/仇恨或持续伤害与毒物消耗；直接同时启用会扩大待核来源及验收面。完整自然成长1→19尚未真实验收，不应把已隔离设置等级/余额的21GUI流程写成自然成长。

根《最终策划案.md》282–296要求P0完整职业技能、材料先由owner原子扣除再生临时效果、健康重入保留；491–495要求真实宠物实体及独立成长，禁止无来源的免费瞬间重召/技能等级等同宠物等级。普通单宠是已登记产品例外。当前22技能分别为战士6/法师11/道士5；召唤17/30、隐身18/19、施毒6及灵符13仍不在public。推荐闭环是完整P0的下一小步，不是完整三职业完成。

## 当前真实UI与缺口

保存Game.scene已有Skills的List/Learn/Use/Shop/Upgrade/Salvage/BindQuick/ModeToggle、SkillAssets的OffersList/MaterialsList/Preview/PurchaseQuantity，以及PopupView保存Confirm/SkillQuote/StatusDetails。AppRoot.ts258–285已有入口绑定；可复用正常购书→原书学习→选技能→Use/快捷触屏施法，不需要生成固定控件。Entity.prefab已有Body/Name/HP/AttachedEffects，名字24字号与0.7保存缩放应保持。

PetPanel现只打开Help（AppRoot.ts286–291），没有保存宠物管理面板。初期宠物等级/经验/召唤资格可写入已有实体Details/StatusDetails文字与系统消息；若需跟随/休息/攻击等新命令或专用经验进度条，应另行授权编辑保存scene/prefab，不能运行时造固定面板。client/AGENTS.md规定固定布局、List复用、触屏入口、弹窗不清导航。现有EntityDetails没有宠物等级/经验/升级阈值的权威字段；应先由服务端定义观察合同，客户端只展示，不推测成长与复活。

server/internal/game/runtime.go683–684目前按道士职业直接创建开发宠物handle2，而非先学习召唤。GameWorldView.ts185–215首次handle时才选profile，同handle定义变化会沿用旧形象/层数/动作集；新Life只重置旧profile动作。该可达前置缺口已获root独立修复授权，尚不能把未来修复写成本审计已通过。WorldAssets.ts130–155已经预载本职业所有pet_profiles及所有动作，不需逐实体异步加载；187–191按kind/definition/job找profile。当前只有道士definition2的pet_skeleton，其它职业无正式宠物形象契约。

## 原始素材的实际可用范围

当前tools/resources/build_development_assets.py23–28只导出Mon4 raw2520骷髅standing/walking/attack/struck/die；manifest仍标official_180_animation_acceptance=false。现有身体、头顶、HP、碰撞显示可以复用为骷髅开发候选，但没有appear动作。不要把Crystal紧凑.lib帧索引当raw WIL编号。

只读WIL/WIX探针实测：Magic600–609(施毒cast)、1500–1509(骷髅cast)、1520–1529(隐身cast)、1540–1549(群隐impact)各10个头部非空帧，Magic2的0–9(神兽cast)同为10个；这些帧尚未导入当前manifest。Magic1510–1519全4×1占位，不能据连续编号补成地效。Magic1160的3帧×方向投射序列已经随盾术使用，可复用原帧来源但不能证明群隐业务。探针仅验证索引/尺寸/非空头部，不等于像素、色键、动作时序或Creator实际画面验收。

Crystal Client/MirObjects/PlayerObject.cs1681–1683、1724–1726、1756–1758、1864–1866、3009–3017分别给上述poison/skeleton/hiding/shinsu/mass-hiding映射。Client/MirGraphics/MLibrary.cs109使用逐怪物.lib；Shared/Enums.cs307–309的BoneFamiliar78、Shinsu79/80不是本项目raw WIL号。Server/MirObjects/Monsters/BoneFamiliar.cs15–25在base.Spawned后才设Summoned=true；base MapObject.cs339–354先BroadcastInfo，故首次Extra=false，后续Extra=true。Client/MirObjects/MonsterObject.cs222–232仅!Extra排Appear，说明健康重观不能无条件重播出场。当前EntityAnimation.ts动作集合无appear，应后续明确新召唤标识/动作合同。

Mon4的2860–2869(base+340)实际头部十个非空大帧，2870–2879为4×1占位：前者仅出场动作候选，未完成视觉及来源映射验证。资料参考/myf/Mud2/DB/ApexM2_Monster.csv381的变异骷髅Appr37与386–387的神兽Appr170/171是来源数据；Mon18→神兽双态仅按经典分包的推断。实际Mon18 WIL/WIX存在但使用旧WEMADE签名，当前inspect_wil转换器不支持；fallback_mir19为wzl/wzx亦不支持，不能宣称神兽全动作已备，更不能沿用骷髅冒充神兽。

实际只读探针使用manifest锁定D:/Mir2/mir2_gxx/assets/legacy/primary_180/Data，旧builder默认上级传奇1.80客户端路径当前不存在。Magic WIL SHA256 be46a0258349b26db9ba7dba595abac1f0767d52fef11d9f508e704f2f6deaac；Magic2 2bd7b8624c6bf6edb7fd5e66d2f0ca6b6a77c86a1ca108138499faca57a56441；Mon4 6a46fc8d8fd093181bde0a8ceca8c25b8d3faf71752dbf47bbbd0a6e5fad5412。未运行来源程序，未读取Setup、账号或部署脚本。

## 最小GUI验收路线及未定项

普通道士选角进入→商店购买来源书25与已批准护符→正常学习17→装备/持有实际施法材料→点击Use或保存快捷按钮→只看到服务端提交后的单宠、真实扣符及MP→杀怪达到来源宠物经验阈值→详情展示权威等级/经验→让宠物真实死亡、空窗中不自动免费复活→再次耗符召唤产生新Life。随后健康票保留原宠/生命与成长；Town往返保持原现场；冷重建临时宠物行为按已登记产品规则观察，不能假定永久宠经验。首次召唤与健康重观的appear应分别验收，旧Life/旧cast迟到不能改新宠。

护符187每包1000张/500金币仍是root待用户决定的候选，本报告不视为已定；材料槽、召唤成功/失败扣耗、宠成长持久性、死亡与重召冷却、受Town限制等均需唯一Excel与来源规则先确定。完整召唤或成长协议/素材导出、真实WS与GUI尚未执行；不将此只读审计写成行为、容量或目标1.80完整认证。
