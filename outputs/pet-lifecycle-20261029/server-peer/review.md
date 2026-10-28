# 宠物生命周期底座只读peer审查

审查日期：2026-10-29。这是server-core尚在收束时的静态审查；仅运行dev包装的rg、git diff与Python读取/摘要命令，没有运行Go测试、overlay、Creator或服务，没有修改生产或测试。读取摘要见read-snapshot.json；不把一次读取摘要冒称冻结前后稳定，也不复述他人测试为本审查执行结果。

## 结论

除root已独立提出的SavedPersonal.HasPet/PetPosition缺少PetLife身份外，本轮未发现其它可由当前rule15正常入口触发的实质新阻断。17/购符仍未开放，本结论不代表其材料、原图500ms新生、召回、NoPets、经验成长或尸体TTL业务已实现。已有居民/单槽Life基础没有通过“总是当前图出生”之类规则抹掉未来来源分支。

## 已核对的边界

- pet_lifecycle.go12–37用PetWorld解析驻留：只有兼容零值才回退currentWorld，明确未知非零World不回退。world_context.go125–139只把玩家加入当前World、把宠物加入其真实驻留World；resolveCredit144–153保留旧地图动作的原Life信用，但不是受击居民注册。
- runtime.go788起的baseline、combat.go44–51的free、displacement_skills.go31–40的cellObjects、town_navigation.go109–123/194–235的actor及动态占位均过滤worldPet。旧图宠物不能在新图显示或阻挡；尸体仍可见且不阻挡活体占格。
- skills.go93–110的单目标治疗和skill_cast.go58–75的区域友方只含该World居民，区域输出再按handle排序；敌方仍仅Kind3。combat_status.go194–211保持14/15尸体目标政策，没有把新生API的HP>0门禁泛化到状态技能。普通move/nearest/attack/commitWalk/relocate/pushed也阻止操作异图当前宠物。
- installPet46–61拒绝已有槽（包括尸体）、外国World、预置非零Life及birth/cell溢出，只有真实安装时分配新Life；retirePet66–76按完整World/handle/Life检查并保留跨nil的最大Life水位。relocatePet80–94同样检查原完整引用及target归属，失败预检在改写前；仅移动宠物，不更换玩家scene。
- activateWorld165–201保留唯一宠物对象、HP、状态与Life，只显式更新居民World、位置及移动revision；没有自动复活。该函数属于可信移交原语，地形/是否召回由业务选择，不能把其Contains检查理解为来源出生/召回完整政策。
- Pending/Isolated门禁仍保留，当前API没有绕过原永久unknown冻结。角色重建重置执行epoch，PetLife在原owner执行代内单调；旧攻击与训练等未来回调仍须捕获完整actorRef，而不能只取当前r.Pet。

## 已知问题与未来合同，不能隐去

1. root已报的旅行快照：world_context.go183–185保存HasPet/PetPosition，map_travel.go379–380无PetLife便复用位置。旧宠退役、新宠安装后，返个人可能把新宠套用旧宠偏好点。此项应由server-core补带Life的实际反例及修复后验收；本审查未擅自运行或修改测试。
2. 目前retirePet只返回原对象，并没有独立尸体居民容器；resolveInWorld/baseline只纳入r.Pet。未来要在旧尸体仍应可见时装新宠，必须先显式登记独立尸体及其解析/基线/有界到期，不能把retire+install直接当成完整来源尸体支持。docs/职业宠物生命周期与召唤实施边界.md9、31、33已明确此待实现范围；当前17关闭阶段可保持底座验收，不可扩大完成宣称。
3. 新install仅提供安装点Contains及实体基本值校验，未添加动态空格/关门判断；这没有抹掉文档25的原capturedFront静态ValidPoint成功先在原World出生、失败才读主人当前图的分支。未来业务必须按原受理World/Front与到期主人Life选点，而不能统一调用currentWorld，不能把伤害/尸体21特例套到17。NoPets读取到期主人图及同图Recall不位移也仍需业务实现。
4. combat.go158–159当前宠物fighter仍统一取开发Fixture.Pet，回合宠物AI也是当前World单槽。该底座尚不是多模板/训练成长实现；后续17必须按唯一Excel推进自身模板/训练属性，不能仅改Def/Profile便宣布召唤成长完成。驻留于非观察World的新生和既有已释放地图事件不得被休眠/自动策略悄悄丢弃。

## 证据边界

pet_lifecycle_test.go现有反例覆盖居民不随观察、友方过滤、跨尸体/nil单调Life、溢出无部分变更、未知World、唯一尸体移交、旧图已释放命中、尸体基线和Town任务归还。本轮只读其断言，不把它写作我运行的race或真实WS结果。当前阶段可提交的应是必要生命周期底座与已实构的rule15回归；17/护符来源和完整召唤闭环继续实施中。

## 后续补审：PetLife旅行快照修复

收到server-core通知后只读补看了新diff与实际反例，仍未运行测试。world_context.go新增PetLife；pet_lifecycle.go44–47集中按真实PetWorld记录Life/位置。install在分配Life并写PetWorld后commitRelocation，retire在清槽前记录，relocate在切PetWorld前记录旧位置、切后commit记录新位置；activate也在切居民身份后提交宠物新点。因此不会把新点误记到旧World。

map_travel.go379起仅同Life历史hint可复用，随后目的World已有实际resident时优先其当前位置；覆盖root原报的跨nil新生误用旧位置，并保留“宠物已经在返回目的图/进Town目的图”的真实现场。pet_lifecycle_test.go283起分别构造新宠驻Town/个人两子例，335起覆盖Town预先resident。静态读取结果与修复意图一致，原已知问题在新diff中已消除；实际专项/full-race结果仍由执行者记录，不在此冒称重跑。

未来独立尸体列表还需要与客户端handle唯一性一起设计：当前可活动宠固定handle2且视图按handle建索引，不能在同一baseline放两个handle2的不同Life来表示旧尸体和新宠。该项仍是17未开放的实现依赖，不构成当前rule15已开放业务回归。
