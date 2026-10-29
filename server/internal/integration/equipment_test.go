package integration

import (
	"context"
	"encoding/json"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

func TestEquipmentTransactionsVersionsReceiptsMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo装备事务")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 22 {
		t.Fatal("必须使用当前rule22装备合同")
	}
	cfg.Database = "minimir_dev_game_test_eq_" + domain.ID()
	ctx := context.Background()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(cleanup))
		owner.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	login, err := authority.Login(ctx, "eq_tx_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	key, err := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "装备事务" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
	must(t, err)
	ticket, err := authority.IssueTicket(ctx, account, key.Key)
	must(t, err)
	r, auth, err := owner.Enter(ctx, ticket.Ticket)
	must(t, err)
	var woods []domain.Item
	for _, item := range r.Items {
		if item.ItemID == 218 {
			woods = append(woods, item)
		}
	}
	if len(woods) != 2 {
		t.Fatal("必须正常Create两件原木剑")
	}
	request := func(id string, version uint64, unequip bool) persistence.AssetRequest {
		q := persistence.AssetRequest{Kind: "equip", ItemID: id, Slot: 0, ItemVersion: version, Unequip: unequip, Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
		raw, e := json.Marshal(q)
		must(t, e)
		q.Fingerprint = persistence.Hash(raw)
		return q
	}
	apply := func(q persistence.AssetRequest) (*pb.OperationResult, persistence.Operation) {
		out, res, op, e := owner.AssetWithReceipt(ctx, auth, q)
		must(t, e)
		r = out
		return res, op
	}
	originalWallet, originalSource := r.Wallet, r.SourceSeq
	first := request(woods[0].ID, uint64(woods[0].Version), false)
	res, firstReceipt := apply(first)
	if res.ErrorCode != 0 || firstReceipt.Equipment == nil || len(firstReceipt.Equipment.Moves) != 1 || firstReceipt.Equipment.Moves[0].Before.ID != woods[0].ID || firstReceipt.Equipment.Moves[0].After.Version != 2 {
		t.Fatal("正常穿入缺少实际typed原件回执", res, firstReceipt)
	}
	var audit struct {
		Equipment *persistence.EquipmentReceipt `bson:"equipment"`
	}
	must(t, owner.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": firstReceipt.ID}).Decode(&audit))
	if !reflect.DeepEqual(audit.Equipment, firstReceipt.Equipment) {
		t.Fatal("同事务audit与实际装备回执不一致")
	}
	res, swapReceipt := apply(request(woods[1].ID, uint64(woods[1].Version), false))
	if res.ErrorCode != 0 || len(swapReceipt.Equipment.Moves) != 2 || r.Equipped[0].Item.ID != woods[1].ID || r.Equipped[0].Item.Version != 2 {
		t.Fatal("换件未记录双方位置变更")
	}
	var returned domain.Item
	for _, item := range r.Items {
		if item.ID == woods[0].ID {
			returned = item
		}
	}
	if returned.Version != 3 || returned.Bound != woods[0].Bound || returned.SourceID != woods[0].SourceID {
		t.Fatal("回包旧件身份/版本改变不正确")
	}
	beforeReplay := r
	res, replayed := apply(first)
	if res.ErrorCode != 0 || !reflect.DeepEqual(r, beforeReplay) || !reflect.DeepEqual(replayed.Equipment, firstReceipt.Equipment) {
		t.Fatal("旧op按当前所在位置重算或回执变化")
	}
	changed := first
	changed.Unequip = true
	changed.Fingerprint = "changed-original-intent"
	_, _, _, err = owner.AssetWithReceipt(ctx, auth, changed)
	if persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("原op改动作未冲突")
	}
	beforeFailure := r
	res, rejected := apply(request(woods[1].ID, 1, true))
	if res.ErrorCode != p.CodeEquipmentChanged || rejected.Equipment != nil || r.Revision != beforeFailure.Revision+1 || r.EconomicSeq != beforeFailure.EconomicSeq+1 || !reflect.DeepEqual(r.Items, beforeFailure.Items) || !reflect.DeepEqual(r.Equipped, beforeFailure.Equipped) {
		t.Fatal("陈旧实例失败未遵原Asset业务失败合同")
	}
	res, removed := apply(request(woods[1].ID, 2, true))
	if res.ErrorCode != 0 || !removed.Equipment.Unequip || len(r.Equipped) != 0 || r.SourceSeq != originalSource || r.Wallet != originalWallet {
		t.Fatal("真正卸下改变来源流水/钱包或没有移回原件")
	}
	if firstReceipt.Equipment.Moves[0].After.Version != 2 {
		t.Fatal("后续移物改写旧回执")
	}
	// 显式永久水位极值资格，不伪装成自然产生MaxInt64资产。
	_, err = owner.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": bson.M{"revision": int64(math.MaxInt64)}})
	must(t, err)
	r, err = owner.Role(ctx, r.ID)
	must(t, err)
	q := request(woods[0].ID, uint64(returned.Version), false)
	countBefore, err := owner.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": r.ID})
	must(t, err)
	_, _, _, err = owner.AssetWithReceipt(ctx, auth, q)
	if persistence.Code(err) != p.CodeOperationConflict {
		t.Fatal("Revision满值仍提交装备")
	}
	countAfter, err := owner.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": r.ID})
	must(t, err)
	if countBefore != countAfter {
		t.Fatal("满值围栏仍生成op")
	}
}

func TestEquipmentWearSwapUnequipAndReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实WS装备穿脱")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 22 {
		t.Fatal("必须使用当前rule22装备合同")
	}
	var offer devconfig.SkillShop
	for _, row := range cfg.Fixture.SkillShop {
		if row.Currency == "gold" && row.RequiredRoleLevel == 1 {
			offer = row
			break
		}
	}
	if offer.ID == 0 {
		t.Fatal("缺正常金币原书报价")
	}
	// 满包和资金是明确的隔离Create资格；不预置穿戴/版本/锁/回执，不新增装备和属性。
	cfg.Fixture.InitialInventory = []uint32{218, 218, 193}
	for len(cfg.Fixture.InitialInventory) < cfg.Fixture.InventoryCapacity-1 {
		cfg.Fixture.InitialInventory = append(cfg.Fixture.InitialInventory, 35)
	}
	cfg.Fixture.InitialBoundGold = offer.PriceUnits
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_eqws_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx := context.Background()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	launch := func() func() {
		runCtx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		for _, mode := range []string{"account", "game", "gm"} {
			wg.Go(func() {
				if e := bootstrap.Run(runCtx, cfg, mode); e != nil && runCtx.Err() == nil {
					t.Errorf("装备%s节点:%v", mode, e)
				}
			})
		}
		for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
			healthy(t, endpoint)
		}
		return func() { cancel(); wg.Wait() }
	}
	stop := launch()
	t.Cleanup(func() {
		stop()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "eq_ws_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	key := &pb.RoleSummary{}
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: 1, Job: 1, Name: "穿脱法师" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, key, 0)
	c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	defer func() { c.close() }()
	c.auto(false, 8)
	stored := func() domain.Role { r, e := inspection.Role(ctx, domain.Hex(key.Key.RoleId)); must(t, e); return r }
	if len(c.inventory.Items) != int(c.inventory.Capacity) {
		t.Fatal("正常Create未形成声明的满包资格")
	}
	var woods []*pb.ItemView
	var cloth *pb.ItemView
	for _, item := range c.inventory.Items {
		if item.ItemId == 218 {
			woods = append(woods, item)
		}
		if item.ItemId == 193 {
			cloth = item
		}
	}
	if len(woods) != 2 || cloth == nil {
		t.Fatal("缺少正常原木剑/布衣")
	}
	baseline := func() *pb.EntityState {
		c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
		return proto.Clone(c.player()).(*pb.EntityState)
	}
	itemInBag := func(id []byte) *pb.ItemView {
		for _, item := range c.inventory.Items {
			if reflect.DeepEqual(item.InstanceId, id) {
				return item
			}
		}
		t.Fatal("原实例不在背包")
		return nil
	}
	asset := func(q *pb.EquipRequest) *pb.OperationResult {
		res := c.asset(p.MsgC2SEquip, q)
		c.inventoryAt(res.NewRevision)
		baseline()
		return res
	}
	baseAttrs := proto.Clone(c.player().CombatAttributes).(*pb.CombatAttributes)
	lock := c.asset(p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: woods[0].InstanceId, ItemVersion: woods[0].Version, Locked: true, Operation: c.operation()})
	if lock.ErrorCode != 0 {
		t.Fatal(lock)
	}
	c.inventoryAt(lock.NewRevision)
	baseline()
	wood := itemInBag(woods[0].InstanceId)
	first := &pb.EquipRequest{ItemInstanceId: wood.InstanceId, ItemVersion: wood.Version, Slot: 0, Operation: c.operation()}
	before := stored()
	if res := asset(first); res.ErrorCode != 0 {
		t.Fatal("锁定原件不能正常穿戴", res)
	}
	after := stored()
	if after.Wallet != before.Wallet || after.SourceSeq != before.SourceSeq || after.Revision != before.Revision+1 || after.EconomicSeq != before.EconomicSeq+1 || !after.Equipped[0].Item.Locked {
		t.Fatal("穿戴改变原钱包/来源/锁/水位")
	}
	woodDef, _ := cfg.Fixture.Item(218)
	attrs := c.player().CombatAttributes
	if attrs.PhysicalAttackMin != baseAttrs.PhysicalAttackMin+woodDef.AttackMinBonus || attrs.PhysicalAttackMax != baseAttrs.PhysicalAttackMax+woodDef.AttackMaxBonus || attrs.MagicAttackMin != baseAttrs.MagicAttackMin || attrs.SpiritAttackMax != baseAttrs.SpiritAttackMax {
		t.Fatal("真实源零MC/SC或DC权威投影错误")
	}
	if result := c.skillConfirm(c.skillPreview(&pb.SkillAssetPreviewRequest{Kind: 1, OfferId: offer.ID, Quantity: 1})); result.ErrorCode != 0 {
		t.Fatal("正常报价购书填回原空格失败", result)
	}
	baseline()
	spare := itemInBag(woods[1].InstanceId)
	if len(c.inventory.Items) != int(c.inventory.Capacity) {
		t.Fatal("正常NPC购书未填满")
	}
	if res := asset(&pb.EquipRequest{ItemInstanceId: spare.InstanceId, ItemVersion: spare.Version, Slot: 0, Operation: c.operation()}); res.ErrorCode != 0 {
		t.Fatal("满包同槽换件失败", res)
	}
	if len(c.inventory.Items) != int(c.inventory.Capacity) || len(c.inventory.Equipped) != 1 || !reflect.DeepEqual(c.inventory.Equipped[0].Item.InstanceId, spare.InstanceId) {
		t.Fatal("满包替换丢格或丢原件")
	}
	worn := c.inventory.Equipped[0].Item
	before = stored()
	full := &pb.EquipRequest{ItemInstanceId: worn.InstanceId, ItemVersion: worn.Version, Slot: 0, Unequip: true, Operation: c.operation()}
	if res := asset(full); res.ErrorCode != p.CodeEquipmentBagFull {
		t.Fatal("满包卸下没有明确3401", res)
	}
	after = stored()
	if !reflect.DeepEqual(before.Items, after.Items) || !reflect.DeepEqual(before.Equipped, after.Equipped) || before.Wallet != after.Wallet || before.SourceSeq != after.SourceSeq || after.Revision != before.Revision+1 || after.EconomicSeq != before.EconomicSeq+1 {
		t.Fatal("满包卸下失败部分移物或改变业务失败合同")
	}
	res := c.asset(p.MsgC2SRecycle, &pb.RecycleRequest{ItemInstanceIds: [][]byte{cloth.InstanceId}, Operation: c.operation()})
	if res.ErrorCode != 0 {
		t.Fatal("正常分解原衣服腾格失败", res)
	}
	c.inventoryAt(res.NewRevision)
	baseline()
	if res = asset(&pb.EquipRequest{ItemInstanceId: worn.InstanceId, ItemVersion: worn.Version, Slot: 0, Unequip: true, Operation: c.operation()}); res.ErrorCode != 0 {
		t.Fatal("真正卸下失败", res)
	}
	if len(c.inventory.Equipped) != 0 || !proto.Equal(c.player().CombatAttributes, baseAttrs) {
		t.Fatal("卸装没有消除装备属性")
	}
	returned := itemInBag(woods[0].InstanceId)
	if res = asset(&pb.EquipRequest{ItemInstanceId: returned.InstanceId, ItemVersion: returned.Version, Slot: 0, Operation: c.operation()}); res.ErrorCode != 0 {
		t.Fatal(res)
	}
	before = stored()
	if res = asset(first); res.ErrorCode != 0 {
		t.Fatal("原已提交请求不能按原指纹重放", res)
	}
	if after = stored(); !reflect.DeepEqual(before, after) {
		t.Fatal("重放旧穿入重新改变当前装备")
	}
	var receipt persistence.Operation
	must(t, inspection.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + before.ID + ":" + domain.Hex(first.Operation.OpId)}).Decode(&receipt))
	if receipt.Equipment == nil || receipt.Equipment.Moves[0].After.Version >= before.Equipped[0].Item.Version {
		t.Fatal("回执未保留首次旧版本事实")
	}
	oldScene, oldLife := c.entry.SceneEpoch, c.player().Life
	c.close()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	if c.entry.SceneEpoch != oldScene || c.player().Life != oldLife || !reflect.DeepEqual(stored().Equipped, before.Equipped) {
		t.Fatal("健康新票重建身体或重放旧移物")
	}
	c.close()
	stop()
	stop = launch()
	c = join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
	if c.entry.SceneEpoch == oldScene || !reflect.DeepEqual(stored().Equipped, before.Equipped) || !c.inventory.Equipped[0].Item.Locked {
		t.Fatal("冷建丢失永久原装备/锁或错误复用旧scene")
	}
	t.Log("正常Create/锁/穿戴/NPC填满/满包换件/3401原子拒绝/正常分解腾格/真正卸下/原op重放/健康与完整冷建均走真实协议；装备MC/SC保持主来源零，非零管线由隔离纯测覆盖。")
}

func TestEquipmentNonZeroMagicSpiritRoundtripMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实WS非零MC/SC隔离资格")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 22 {
		t.Fatal("必须从当前rule22派生非零管线资格")
	}
	for n := range cfg.Fixture.Items {
		if cfg.Fixture.Items[n].ID == 218 {
			cfg.Fixture.Items[n].MagicMinBonus, cfg.Fixture.Items[n].MagicMaxBonus = 2, 4
			cfg.Fixture.Items[n].TaoMinBonus, cfg.Fixture.Items[n].TaoMaxBonus = 3, 5
		}
	}
	// 只有怪物受击/造成受伤的隔离资格；不改技能、动作/恢复周期、掉率、刷新或任何临时结果。
	cfg.Fixture.Monsters[0].AttackMin, cfg.Fixture.Monsters[0].AttackMax = 4, 4
	cfg.Fixture.Monsters[0].Accuracy = 100
	cfg.Fixture.Monsters[0].MagicDefenceMin, cfg.Fixture.Monsters[0].MagicDefenceMax = 0, 0
	cfg.Fixture.Monsters[0].MagicResist = 0
	cfg.Fixture.Map.MonsterSpawns[0] = devconfig.Point{X: 8, Y: 9}
	raw, err := json.Marshal(cfg.Fixture)
	must(t, err)
	var roundtrip devconfig.Fixture
	must(t, json.Unmarshal(raw, &roundtrip))
	roundtrip.Hash = persistence.Hash(raw)
	cfg.Fixture = roundtrip
	must(t, cfg.Fixture.Validate())
	definition, ok := cfg.Fixture.Item(218)
	if !ok || !definition.EquipmentStatsKnown() || definition.MagicMinBonus != 2 || definition.MagicMaxBonus != 4 || definition.TaoMinBonus != 3 || definition.TaoMaxBonus != 5 {
		t.Fatal("JSON往返丢了四端点或显式阶段")
	}
	cfg.Database = "minimir_dev_game_test_eqnz_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = address(t), address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	var wg sync.WaitGroup
	for _, mode := range []string{"account", "game", "gm"} {
		wg.Go(func() {
			if e := bootstrap.Run(ctx, cfg, mode); e != nil && ctx.Err() == nil {
				t.Errorf("非零装备%s:%v", mode, e)
			}
		})
	}
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	for _, endpoint := range []string{cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress} {
		healthy(t, endpoint)
	}
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	login := &pb.LoginResult{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "eq_nz_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	for _, job := range []uint32{1, 2} {
		t.Run(map[uint32]string{1: "法师MC", 2: "道士SC"}[job], func(t *testing.T) {
			key := &pb.RoleSummary{}
			rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: 1, Slot: job, Job: job, Name: "非零装备" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())}, key, 0)
			// 明示首次Enter前等级7资格；不预置Equipped、技能、资源、回执或施法结果。
			_, e := inspection.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(key.Key.RoleId)}, bson.M{"$set": bson.M{"level": uint32(7)}})
			must(t, e)
			c := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, key.Key))
			defer c.close()
			c.auto(false, 1)
			baseline := func() { c.until(func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline }) }
			baseAttrs := proto.Clone(c.player().CombatAttributes).(*pb.CombatAttributes)
			skillID := map[uint32]uint32{1: 1, 2: 2}[job]
			skill, known := cfg.Fixture.Skill(skillID)
			if !known {
				t.Fatal("缺职业原技能")
			}
			var book, wood *pb.ItemView
			for _, item := range c.inventory.Items {
				if item.ItemId == 218 && wood == nil {
					wood = item
				}
				if item.ItemId == skill.BookItemID {
					book = item
				}
			}
			if wood == nil || book == nil {
				t.Fatal("正常Create原武器或原书缺失")
			}
			res := c.asset(p.MsgC2SLearnSkill, &pb.LearnSkillRequest{ItemInstanceId: book.InstanceId, SkillId: skillID, Operation: c.operation()})
			if res.ErrorCode != 0 {
				t.Fatal("资格等级后正常原书学习失败", res)
			}
			c.inventoryAt(res.NewRevision)
			baseline()
			res = c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: wood.InstanceId, ItemVersion: wood.Version, Slot: 0, Operation: c.operation()})
			if res.ErrorCode != 0 {
				t.Fatal(res)
			}
			c.inventoryAt(res.NewRevision)
			baseline()
			attrs := c.player().CombatAttributes
			if attrs.MagicAttackMin != baseAttrs.MagicAttackMin+2 || attrs.MagicAttackMax != baseAttrs.MagicAttackMax+4 || attrs.SpiritAttackMin != baseAttrs.SpiritAttackMin+3 || attrs.SpiritAttackMax != baseAttrs.SpiritAttackMax+5 {
				t.Fatalf("完整baseline丢非零四端点: before=%v after=%v", baseAttrs, attrs)
			}
			player := c.player()
			if job == 2 && player.Hp == player.HpMax {
				c.until(func(f wire.Frame) bool {
					return f.ID == p.MsgS2CSceneBaseline && c.player().Hp > 0 && c.player().Hp < c.player().HpMax
				})
				player = c.player()
			}
			var target *pb.EntityState
			if job == 2 {
				target = player
			} else {
				for _, entity := range c.scene.Entities {
					if entity.EntityKind == 3 && entity.Hp > 0 {
						target = entity
						break
					}
				}
			}
			if target == nil || player.Hp <= 0 {
				t.Fatal("没有合法存活施法目标")
			}
			var accepted pb.CastAccepted
			request := c.send(p.MsgC2SCastIntent, &pb.CastIntent{SceneEpoch: c.entry.SceneEpoch, InputSequence: 1, SkillId: skillID, TargetHandle: target.EntityHandle})
			c.until(func(f wire.Frame) bool {
				if f.ID != p.MsgS2CCastAccepted {
					return false
				}
				must(t, proto.Unmarshal(f.Payload, &accepted))
				return accepted.InputSequence == 1 && accepted.SkillId == skillID
			})
			if accepted.CastId == 0 || accepted.CasterLife != player.Life || accepted.TargetLife != target.Life {
				t.Fatal("新施法没有原身体/目标身份", request, &accepted)
			}
			var impact pb.CombatImpact
			c.until(func(f wire.Frame) bool {
				if f.ID != p.MsgS2CCombatImpact {
					return false
				}
				must(t, proto.Unmarshal(f.Payload, &impact))
				return impact.CastId == accepted.CastId && impact.SkillId == skillID
			})
			low, high := attrs.MagicAttackMin, attrs.MagicAttackMax
			if job == 2 {
				low, high = attrs.SpiritAttackMin*skill.HealingStatMultiplier, attrs.SpiritAttackMax*skill.HealingStatMultiplier
			}
			minimum := skill.SourceDamage(skill.LearnedLevel, low, skill.SourcePower(skill.LearnedLevel, skill.MPowerBase, skill.PowerBase))
			maximum := skill.SourceDamage(skill.LearnedLevel, high, skill.SourcePower(skill.LearnedLevel, skill.MPowerBase+max(int64(0), skill.MPowerBonus-1), skill.PowerBase+max(int64(0), skill.PowerBonus-1)))
			if job == 2 && skill.AddCasterLevel {
				minimum += 7
				maximum += 7
			}
			if impact.Amount < minimum || impact.Amount > maximum || impact.TargetLife != target.Life || impact.Healing != (job == 2) || impact.Outcome != map[bool]uint32{false: 1, true: 4}[job == 2] {
				t.Fatalf("非零属性未走原实际技能路径: impact=%v expected=%d..%d", &impact, minimum, maximum)
			}
			if job == 2 {
				previousHP := c.player().Hp
				c.until(func(f wire.Frame) bool {
					if f.ID != p.MsgS2CSceneBaseline {
						return false
					}
					current := c.player()
					if current.Hp <= 0 {
						t.Fatal("治疗观察前死亡")
					}
					increased := current.Hp > previousHP
					previousHP = current.Hp
					return increased
				})
			}
			worn := c.inventory.Equipped[0].Item
			res = c.asset(p.MsgC2SEquip, &pb.EquipRequest{ItemInstanceId: worn.InstanceId, ItemVersion: worn.Version, Slot: 0, Unequip: true, Operation: c.operation()})
			if res.ErrorCode != 0 {
				t.Fatal(res)
			}
			c.inventoryAt(res.NewRevision)
			baseline()
			if len(c.inventory.Equipped) != 0 || !proto.Equal(c.player().CombatAttributes, baseAttrs) {
				t.Fatal("真正卸下没有恢复四端点基础")
			}
			t.Logf("隔离218四端点JSON往返/Validate，正常原件穿卸/原书学习与实际技能%d cast%d impact=%d（源公式%d..%d）；level7仅资格非自然成长，effective_fixture_sha=%s", skillID, accepted.CastId, impact.Amount, minimum, maximum, cfg.Fixture.Hash)
		})
	}
}

func TestEquipmentLegacyWearCanBeRemovedAndReplacedMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式真实Mongo旧穿戴物解除资格")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	if cfg.Fixture.RuleVersion != 22 {
		t.Fatal("必须使用当前rule22")
	}
	for n := range cfg.Fixture.Items {
		i := &cfg.Fixture.Items[n]
		if i.ID == 218 {
			i.RequiredLevel = new(uint32(19))
			i.EquipmentStage = 1
			i.AllowedJobs = []uint32{0}
		}
		// 同测试槽的合格替换件资格，仅复用已选布衣定义，不扩目录/数值/回收配方。
		if i.ID == 193 {
			i.EquipmentSlot = 0
		}
	}
	must(t, cfg.Fixture.Validate())
	cfg.Database = "minimir_dev_game_test_eqold_" + domain.ID()
	ctx := context.Background()
	authority, err := persistence.Open(ctx, cfg)
	must(t, err)
	owner, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		must(t, authority.DB.Drop(cleanup))
		owner.Close(cleanup)
		authority.Close(cleanup)
	})
	must(t, authority.Singleton(ctx, "account"))
	grant, err := authority.ApproveGame(ctx, cfg.NodeID, owner.Instance, cfg.RunID)
	must(t, err)
	owner.Epoch = grant.Epoch
	login, err := authority.Login(ctx, "eq_old_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	account, err := authority.Authenticate(ctx, login.AccountToken)
	must(t, err)
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "卸下", true: "替换"}[replace], func(t *testing.T) {
			slot := uint32(1)
			if replace {
				slot = 2
			}
			key, e := authority.CreateRole(ctx, account, &pb.CreateRoleRequest{OriginServerId: 1, Slot: slot, Job: 1, Name: "旧装解除" + domain.ID()[:5], OpId: domain.Bytes(domain.ID())})
			must(t, e)
			legacy, e := authority.Role(ctx, domain.Hex(key.Key.RoleId))
			must(t, e)
			var old domain.Item
			var replacement domain.Item
			items := make([]domain.Item, 0, len(legacy.Items))
			for _, item := range legacy.Items {
				if item.ItemID == 218 && old.ID == "" {
					old = item
					continue
				}
				items = append(items, item)
				if item.ItemID == 193 {
					replacement = item
				}
			}
			if old.ID == "" || replacement.ID == "" {
				t.Fatal("旧存档资格缺正常Create原件")
			}
			old.Version, old.Quality, old.Locked = 3, 4, true
			// 明确隔离旧存档：仅首次Enter前安放原已穿戴物。它不符合新定义的level19/战士，但不伪装成一次正常新穿。
			_, e = authority.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": legacy.ID}, bson.M{"$set": bson.M{"items": items, "equipped": []domain.Equipped{{Slot: 0, Item: old}}}})
			must(t, e)
			ticket, e := authority.IssueTicket(ctx, account, key.Key)
			must(t, e)
			r, auth, e := owner.Enter(ctx, ticket.Ticket)
			must(t, e)
			makeRequest := func(id string, version uint64, unequip bool) persistence.AssetRequest {
				q := persistence.AssetRequest{Kind: "equip", ItemID: id, ItemVersion: version, Slot: 0, Unequip: unequip, Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(r.EconomicSeq + 1), ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
				data, e := json.Marshal(q)
				must(t, e)
				q.Fingerprint = persistence.Hash(data)
				return q
			}
			q := makeRequest(old.ID, 3, true)
			if replace {
				q = makeRequest(replacement.ID, uint64(replacement.Version), false)
			}
			before := r
			out, result, op, e := owner.AssetWithReceipt(ctx, auth, q)
			must(t, e)
			r = out
			if result.ErrorCode != 0 || op.Equipment == nil || r.Wallet != before.Wallet || r.SourceSeq != before.SourceSeq || r.Revision != before.Revision+1 || r.EconomicSeq != before.EconomicSeq+1 {
				t.Fatal("旧等级/职业/合法旧品质被困住或永久水位错误", result)
			}
			var returned domain.Item
			for _, item := range r.Items {
				if item.ID == old.ID {
					returned = item
				}
			}
			wanted := old
			wanted.Version++
			if !reflect.DeepEqual(returned, wanted) {
				t.Fatal("解除旧穿戴改变原ID/品质/绑定/来源/锁")
			}
			before = r
			out, result, rejected, e := owner.AssetWithReceipt(ctx, auth, makeRequest(old.ID, uint64(returned.Version), false))
			must(t, e)
			r = out
			if result.ErrorCode != p.CodeEquipmentChanged || rejected.Equipment != nil || !reflect.DeepEqual(r.Items, before.Items) || !reflect.DeepEqual(r.Equipped, before.Equipped) {
				t.Fatal("旧物解除后获得不符合当前职业/等级/品质的新穿资格")
			}
			before = r
			out, result, replayed, e := owner.AssetWithReceipt(ctx, auth, q)
			must(t, e)
			if result.ErrorCode != 0 || !reflect.DeepEqual(out, before) || !reflect.DeepEqual(replayed.Equipment, op.Equipment) {
				t.Fatal("旧解除回执重放再次移动或被当前新穿资格拒绝")
			}
			t.Logf("明确旧存档资格原件%s quality4/version3：解除或替换→原样返包v4；当前level1法师不满足新level19战士定义，重新穿入仍3402；未扩主目录或回收资格", old.ID)
		})
	}
}
