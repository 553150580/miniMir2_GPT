// 游戏节点租约中的运行服成员关系，以及共用连接池的单运行服业务视图。

package persistence

import (
	"cmp"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// gameRunMembership 保存节点本次启动占用的一条运行服租约及其独立代次。
type gameRunMembership struct {
	RunID uint64 `bson:"run_server_id"`
	Epoch int64  `bson:"run_epoch"`
}

// leasedGameNode 包含当前游戏进程，以及有序成员或旧版单运行服字段。
type leasedGameNode struct {
	ID          string              `bson:"_id"`
	Owner       string              `bson:"owner_instance"`
	Memberships []gameRunMembership `bson:"run_memberships"`
	LegacyRunID uint64              `bson:"run_server_id"`
	LegacyEpoch int64               `bson:"run_epoch"`
}

// runs 返回持久节点租约所绑定的实际成员关系，旧格式仍可读取。
func (n leasedGameNode) runs() []gameRunMembership {
	if n.Memberships == nil && n.LegacyRunID != 0 && n.LegacyEpoch > 0 {
		return []gameRunMembership{{RunID: n.LegacyRunID, Epoch: n.LegacyEpoch}}
	}
	return n.Memberships
}

// matchingNode 检查运行服和节点是否属于同一进程、成员和代次。
func matchingNode(node leasedGameNode, run Run) bool {
	return node.Owner == run.Owner && slices.Contains(node.runs(), gameRunMembership{RunID: run.ID, Epoch: run.Epoch})
}

// nodeLeaseFilter 核对单个运行服的当前节点租约，用于入场、仓库读和资产写围栏。
func nodeLeaseFilter(node, instance string, run gameRunMembership) bson.M {
	return bson.M{
		"_id": node, "kind": "game", "owner_instance": instance, "$expr": leaseFilter(),
		"$or": bson.A{
			bson.M{"run_memberships": bson.M{"$elemMatch": run}},
			bson.M{"run_memberships": bson.M{"$exists": false}, "run_server_id": run.RunID, "run_epoch": run.Epoch},
		},
	}
}

// exactNodeMembershipFilter 比较本次启动的整组成员，防止部分续约、释放或重放。
func exactNodeMembershipFilter(node, instance string, runs []gameRunMembership) bson.M {
	filter := bson.M{"_id": node, "kind": "game", "owner_instance": instance}
	if len(runs) == 1 {
		filter["$or"] = bson.A{
			bson.M{"run_memberships": runs},
			bson.M{"run_memberships": bson.M{"$exists": false}, "run_server_id": runs[0].RunID, "run_epoch": runs[0].Epoch},
		}
	} else {
		filter["run_memberships"] = runs
	}
	return filter
}

// GameRunStores 校验注册响应，在监听前安装完整授权并创建单运行服业务视图。
// 视图只借用根存储的 MongoDB 连接；本地事务围栏各自按运行服隔离。
func (s *Store) GameRunStores(grants []Run) ([]*Store, error) {
	requested, err := s.Cfg.HostedRunIDs()
	if err != nil {
		return nil, err
	}
	if s.borrowedRunView || len(s.gameMemberships) != 0 || len(grants) != len(requested) {
		return nil, Fail(p.CodeOperationConflict, "运行服授权集合不匹配或已安装")
	}
	assigned := make(map[uint64]Run, len(grants))
	memberships := make([]gameRunMembership, 0, len(grants))
	for _, grant := range grants {
		if _, duplicate := assigned[grant.ID]; duplicate || !devconfig.ValidRunID(grant.ID) ||
			grant.GameNodeID != s.Cfg.NodeID || grant.Owner != s.Instance || grant.State != "active" ||
			grant.Epoch <= 0 || grant.AssignmentVersion <= 0 {
			return nil, Fail(p.CodeRouteChanged, "运行服授权与当前启动身份不匹配")
		}
		assigned[grant.ID] = grant
		memberships = append(memberships, gameRunMembership{RunID: grant.ID, Epoch: grant.Epoch})
	}
	slices.SortFunc(memberships, func(a, b gameRunMembership) int { return cmp.Compare(a.RunID, b.RunID) })
	views := make([]*Store, 0, len(requested))
	for _, runID := range requested {
		grant, ok := assigned[runID]
		if !ok {
			return nil, Fail(p.CodeRouteChanged, "运行服授权缺少当前启动请求的运行服")
		}
		cfg := s.Cfg
		cfg.RunID, cfg.RunServerIDs = runID, nil
		views = append(views, &Store{
			Client: s.Client, Control: s.Control, DB: s.DB, Cfg: cfg, Instance: s.Instance, Epoch: grant.Epoch,
			runGuardSlots: newRunGuardSlots(), borrowedRunView: true,
		})
	}
	s.gameMemberships = memberships
	s.Epoch = assigned[requested[0]].Epoch
	return views, nil
}

// ownedGameRuns 为旧版单运行服调用方保留手工设置 Epoch 的入口。
func (s *Store) ownedGameRuns() ([]gameRunMembership, error) {
	if s.borrowedRunView {
		return nil, Fail(p.CodeInvalidArgument, "运行服视图不持有节点控制权")
	}
	if len(s.gameMemberships) > 0 {
		return s.gameMemberships, nil
	}
	if !devconfig.ValidRunID(s.Cfg.RunID) || s.Epoch <= 0 || len(s.Cfg.RunServerIDs) > 1 {
		return nil, Fail(p.CodeRouteChanged, "本实例尚未取得完整运行服授权")
	}
	return []gameRunMembership{{RunID: s.Cfg.RunID, Epoch: s.Epoch}}, nil
}

// activeRunOwnership 限定对运行服控制记录的操作只能来自原节点、启动实例和运行服代次。
func activeRunOwnership(node, instance string, run gameRunMembership) bson.M {
	return bson.M{"_id": run.RunID, "game_node_id": node, "owner_instance": instance, "run_epoch": run.Epoch, "state": "active"}
}
