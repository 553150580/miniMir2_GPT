// 本文件通过真实共享城镇、道士施法和地面移动报文验证自宠让格。
package integration

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSharedTownOwnPetGroundNavigationMongoWS 确认原导航身份、自己的宠物单步及他人身体独立。
func TestSharedTownOwnPetGroundNavigationMongoWS(t *testing.T) {
	cfg, ctx, store := m1FieldServices(t, m1FieldConfig(t))
	fixture := &cfg.Fixture
	skeleton, ok := fixture.Skill(devconfig.SummonSkeletonSkillID)
	if !ok || skeleton.MaterialItemID == 0 || !fixture.CastingSupply.Enabled {
		t.Fatal("本场需要已发布的骷髅召唤及装符规则")
	}
	townID := fixture.TownTravel.DestinationMapIDs[0]
	create := func() *client {
		t.Helper()
		endpoint, token, key := m1FieldRole(t, cfg, skeleton.JobID)
		// 只在隔离库准备适龄、已学和一份绑定护符；无临时宠物前态，正常上场后才施法。
		supply := domain.Item{ID: domain.ID(), ItemID: skeleton.MaterialItemID, Bound: true, Version: 1, Quantity: skeleton.MaterialQuantity}
		_, err := store.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": domain.Hex(key.RoleId)}, bson.M{
			"$set":  bson.M{"level": skeleton.LearnLevel, "learned_skills": []domain.LearnedSkill{{SkillID: skeleton.ID}}},
			"$push": bson.M{"items": supply}})
		must(t, err)
		c := joinWithAutoSkills(t, cfg.GameAddress, issue(t, endpoint, token, key), &pb.AutoSkillSettings{})
		t.Cleanup(c.close)
		c.auto(false, 1)
		result := c.asset(p.MsgC2SSetCastingSupply, &pb.SetCastingSupplyRequest{ItemInstanceId: domain.Bytes(supply.ID), ItemVersion: 1, Operation: c.operation()})
		if result.ErrorCode != 0 {
			t.Fatal("原护符穿戴未确认", result)
		}
		c.inventoryAt(result.NewRevision)
		travelToBaseline(c, 1, townID)
		travelAcknowledge(c, true)
		if c.scene.OwnedPet != nil {
			t.Fatal("召唤前错误出现临时宠物")
		}
		return c
	}
	owner, other := create(), create()
	t.Logf("双角色进镇：主人 %v，另一角色 %v", owner.player().Position, other.player().Position)
	move := func(c *client, point *pb.GridPoint) *pb.MoveIntent {
		t.Helper()
		intent := &pb.MoveIntent{SceneEpoch: c.entry.SceneEpoch, Destination: point}
		c.send(p.MsgC2SMoveIntent, intent)
		return intent
	}
	near := func(a, b *pb.GridPoint) bool {
		if a == nil || b == nil || proto.Equal(a, b) {
			return false
		}
		dx, dy := a.X-b.X, a.Y-b.Y
		return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1
	}
	navigated := func(c *client, intent *pb.MoveIntent, status uint32, check func(*pb.GridPoint) bool) bool {
		nav := c.scene.GetNavigationState()
		return nav.GetInputSequence() == intent.InputSequence && nav.GetStatus() == status &&
			proto.Equal(nav.GetDestination(), intent.Destination) && check(c.player().GetPosition())
	}
	otherPoint := &pb.GridPoint{X: 5, Y: 13}
	arrive := move(other, otherPoint)
	travelUntilWithin(other, 8*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && navigated(other, arrive, 2, func(p *pb.GridPoint) bool { return proto.Equal(p, otherPoint) })
	})
	t.Logf("另一角色已在其固定站位 %v", other.player().Position)
	// 到达确认可能早于最后一步的动作门禁结束，实际手动施法沿原时间。
	time.Sleep(time.Duration(other.player().MoveIntervalMs+100) * time.Millisecond)
	summon := func(c *client, direction uint32) *pb.EntityState {
		t.Helper()
		material := summonWSEquipped(t, c)
		result, accepted := summonWSMaterial(c, &pb.CastWithMaterialRequest{SceneEpoch: c.entry.SceneEpoch,
			CasterLife: c.player().Life, SkillId: skeleton.ID, Direction: direction, MaterialInstanceId: material.InstanceId,
			MaterialVersion: material.Version, Operation: c.operation()})
		if result.ErrorCode != 0 || accepted == nil || accepted.SkillId != skeleton.ID {
			t.Fatal("城镇实际召唤未确认", result, accepted)
		}
		var pet *pb.EntityState
		travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
			pet = petRuntimeWSEntity(c.scene, c.scene.GetOwnedPet().GetEntityHandle())
			return f.ID == p.MsgS2CSceneBaseline && pet != nil && pet.EntityKind == 2 && pet.Hp > 0
		})
		if !near(c.player().Position, pet.Position) {
			t.Fatal("实际召唤未产生邻格宠物", pet)
		}
		return proto.Clone(pet).(*pb.EntityState)
	}
	otherPet := summon(other, 6)
	t.Logf("另一角色实召，宠物格 %v", otherPet.Position)
	ownerPet := summon(owner, 2)
	t.Logf("主人实召，宠物格 %v", ownerPet.Position)
	ownerMoveStart := owner.player().MovementRevision
	otherMoveStart := other.player().MovementRevision
	goal := proto.Clone(ownerPet.Position).(*pb.GridPoint)
	request := move(owner, goal)
	travelUntilWithin(owner, 8*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && navigated(owner, request, 2, func(p *pb.GridPoint) bool { return proto.Equal(p, goal) })
	})
	pet := petRuntimeWSEntity(owner.scene, ownerPet.EntityHandle)
	if pet == nil || pet.Life != ownerPet.Life || pet.MovementRevision != ownerPet.MovementRevision+1 || pet.MovementKind != 1 ||
		!near(goal, pet.Position) || owner.player().MovementRevision != ownerMoveStart+1 || owner.scene.GetOwnedPet().GetMapId() != townID {
		t.Fatal("自宠让位必须是原城镇原宠单步移动，主人也只走一步", owner.scene.NavigationState, pet)
	}
	t.Logf("主人抵达原点击 %v，自己的宠物已至 %v", goal, pet.Position)
	parked := move(owner, otherPoint)
	travelUntilWithin(owner, 8*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && navigated(owner, parked, 1, func(p *pb.GridPoint) bool { return near(p, otherPoint) })
	})
	t.Logf("点击另一角色占格时仍停在其邻格 %v", owner.player().Position)
	// 改点另一侧，由另一客户端亲自观察其角色和宠物没有被强制移动。
	otherPetPoint := move(owner, proto.Clone(otherPet.Position).(*pb.GridPoint))
	travelUntilWithin(owner, 8*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && navigated(owner, otherPetPoint, 2, func(p *pb.GridPoint) bool { return proto.Equal(p, otherPet.Position) })
	})
	// 请求新的权威基线，避免第二连接先收到之前排队的旧场景快照。
	check := other.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: other.entry.SceneEpoch, CancelNavigation: true})
	travelUntilWithin(other, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && f.Request == check
	})
	still := petRuntimeWSEntity(other.scene, otherPet.EntityHandle)
	if !proto.Equal(other.player().Position, otherPoint) || other.player().MovementRevision != otherMoveStart || still == nil ||
		!proto.Equal(still.Position, otherPet.Position) || still.Life != otherPet.Life || still.MovementRevision != otherPet.MovementRevision {
		t.Fatal("点地导航错误移动同城另一角色或它的宠物", other.player(), still)
	}
	t.Logf("另一连接新基线确认角色仍在 %v，宠物仍在 %v", other.player().Position, still.Position)
}
