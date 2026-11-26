// 账号区服目录与持久路由，管理永久入口、运行服部署和当前游戏进程之间的映射。

package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
)

// originRoute 区分角色归属与当前运行位置，展示字段只用于客户端区服目录。
type originRoute struct {
	OriginID    uint64 `bson:"_id"`           // 角色创建后永久不变的入口服编号。
	RunID       uint64 `bson:"run_server_id"` // 当前承载该入口的运行服。
	Version     int64  `bson:"route_version"` // 入口改派时应递增的版本。
	Name        string `bson:"name"`          // 客户端显示的入口服名称。
	RegionID    string `bson:"region_id"`     // 对入口服分组的展示标识，不参与资产归属。
	RegionName  string `bson:"region_name"`   // 客户端显示的分区名称。
	RegionOrder int    `bson:"region_order"`  // 分区显示顺序。
	Order       int    `bson:"order"`         // 分区内的入口服显示顺序。
}

// runAssignment 是运行服受控部署记录，独立于进程续租记录。
type runAssignment struct {
	RunID             uint64 `bson:"_id"`                // 可承载一个或多个入口的运行服编号。
	GameNodeID        string `bson:"game_node_id"`       // 当前被允许承载运行服的稳定节点身份。
	GameWSURL         string `bson:"game_ws_url"`        // 账号节点签票时返回的外部 WebSocket 地址。
	AssignmentVersion int64  `bson:"assignment_version"` // 部署变更时应递增的版本。
	State             string `bson:"state"`              // open 允许签发新入场票据。
}

// routedRun 将入口版本、部署版本和进程代次聚合为一次入场选择。
type routedRun struct {
	origin     originRoute
	assignment runAssignment
	run        Run
}

// seedRoutingCatalog 只建立缺失记录和补齐历史展示字段，不覆盖持续运营中的路由与部署。
func (s *Store) seedRoutingCatalog(ctx context.Context) error {
	origins := s.Cfg.OriginRoutes
	runs := s.Cfg.RunServers
	// 多运行服进程可从账号节点读取已有运营分配，启动成员清单不会隐式生成第一服的路由。
	if len(s.Cfg.RunServerIDs) > 1 && len(origins) == 0 && len(runs) == 0 {
		return nil
	}
	if len(origins) == 0 {
		origins = []devconfig.OriginRouteSeed{{
			OriginID: s.Cfg.OriginID, Name: fmt.Sprintf("%d服", s.Cfg.OriginID),
			RegionID: "local", RegionName: "试玩区", RunID: s.Cfg.RunID,
		}}
	}
	if len(runs) == 0 {
		runs = []devconfig.RunServerSeed{{
			RunID: s.Cfg.RunID, GameNodeID: s.Cfg.NodeID, GameWSURL: "ws://" + s.Cfg.GameAddress + "/ws",
		}}
	}
	for _, seed := range runs {
		assignment := runAssignment{
			RunID: seed.RunID, GameNodeID: seed.GameNodeID, GameWSURL: seed.GameWSURL,
			AssignmentVersion: 1, State: "open",
		}
		if _, err := s.DB.Collection("run_servers").UpdateOne(ctx, bson.M{"_id": seed.RunID},
			bson.M{"$setOnInsert": assignment}, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
		initial := Run{ID: seed.RunID, State: "stopped", LeaseUntil: time.Unix(0, 0)}
		if _, err := s.DB.Collection("run_control").UpdateOne(ctx, bson.M{"_id": seed.RunID},
			bson.M{"$setOnInsert": initial}, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
	}
	for _, seed := range origins {
		// $literal 使显示名称可按原样写入，不把开头的美元字符当成字段引用。
		data := bson.D{
			{Key: "run_server_id", Value: seed.RunID}, {Key: "route_version", Value: int64(1)},
			{Key: "name", Value: seed.Name}, {Key: "region_id", Value: seed.RegionID},
			{Key: "region_name", Value: seed.RegionName}, {Key: "region_order", Value: seed.RegionOrder},
			{Key: "order", Value: seed.Order},
		}
		for i, field := range data {
			data[i].Value = bson.M{"$ifNull": bson.A{"$" + field.Key, bson.M{"$literal": field.Value}}}
		}
		if _, err := s.DB.Collection("origin_routes").UpdateOne(ctx, bson.M{"_id": seed.OriginID},
			mongo.Pipeline{bson.D{{Key: "$set", Value: data}}}, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
	}
	return nil
}

// originRoute 找出已登记的永久入口及其当前映射。
func (s *Store) originRoute(ctx context.Context, origin uint64) (originRoute, error) {
	var route originRoute
	if origin == 0 {
		return route, Fail(p.CodeRouteChanged, "入口不存在")
	}
	err := s.DB.Collection("origin_routes").FindOne(ctx, bson.M{"_id": origin}).Decode(&route)
	if err != nil {
		return route, authorityReadError(err, p.CodeRouteChanged, "入口不存在")
	}
	if route.RunID == 0 || route.Version <= 0 {
		return route, Fail(p.CodeRouteChanged, "入口路由未登记")
	}
	return route, nil
}

// openAssignment 检查持久部署允许新连接，并验证将要交给客户端的地址。
func openAssignment(assignment runAssignment) bool {
	return assignment.RunID != 0 && devconfig.ValidGameNodeID(assignment.GameNodeID) && assignment.AssignmentVersion > 0 &&
		assignment.State == "open" && devconfig.ValidateGameWSURL(assignment.GameWSURL) == nil
}

// leaseFilter 同时用于目录和实际签票，在 MongoDB 服务端判断租约是否过期。
func leaseFilter() bson.M {
	return bson.M{"$gt": bson.A{"$lease_until", "$$NOW"}}
}

// matchingRun 确保登记的部署与当前运行服租约属于同一个获准版本。
func matchingRun(assignment runAssignment, run Run) bool {
	return run.ID == assignment.RunID && run.Owner != "" && run.Epoch > 0 &&
		run.GameNodeID == assignment.GameNodeID && run.AssignmentVersion == assignment.AssignmentVersion
}

// liveRoute 解析入口、部署和双份有效租约。不存在或失效的运营状态返回明确业务错误。
func (s *Store) liveRoute(ctx context.Context, origin uint64) (routedRun, error) {
	var result routedRun
	route, err := s.originRoute(ctx, origin)
	if err != nil {
		return result, err
	}
	result.origin = route
	if err = s.DB.Collection("run_servers").FindOne(ctx, bson.M{"_id": route.RunID}).Decode(&result.assignment); err != nil {
		return result, authorityReadError(err, p.CodeLineRecovering, "运行服未部署")
	}
	if !openAssignment(result.assignment) {
		return result, Fail(p.CodeLineRecovering, "运行服维护中或地址未登记")
	}
	if err = s.DB.Collection("run_control").FindOne(ctx,
		bson.M{"_id": route.RunID, "state": "active", "$expr": leaseFilter()}).Decode(&result.run); err != nil {
		return result, authorityReadError(err, p.CodeLineRecovering, "运行服尚未就绪")
	}
	if !matchingRun(result.assignment, result.run) {
		return result, Fail(p.CodeLineRecovering, "运行服尚未取得当前部署的许可")
	}
	member := gameRunMembership{RunID: result.run.ID, Epoch: result.run.Epoch}
	if err = s.DB.Collection("node_leases").FindOne(ctx, nodeLeaseFilter(result.assignment.GameNodeID, result.run.Owner, member)).Err(); err != nil {
		return result, authorityReadError(err, p.CodeLineRecovering, "游戏节点尚未就绪")
	}
	return result, nil
}

// ServerDirectory 返回持久登记的分区和入口服，分批读取可用性，避免逐服访问数据库。
func (s *Store) ServerDirectory(ctx context.Context, a Auth) (*pb.ServerDirectoryResult, error) {
	if a.AccountID == "" || a.AuthVersion <= 0 {
		return nil, Fail(p.CodeUnauthenticated, "需要账号认证")
	}
	cursor, err := s.DB.Collection("origin_routes").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{
		{Key: "region_order", Value: 1}, {Key: "region_id", Value: 1}, {Key: "order", Value: 1}, {Key: "_id", Value: 1},
	}))
	if err != nil {
		return nil, err
	}
	var routes []originRoute
	if err = cursor.All(ctx, &routes); err != nil {
		return nil, err
	}
	available, err := s.availableRuns(ctx, routes)
	if err != nil {
		return nil, err
	}
	out := &pb.ServerDirectoryResult{}
	regions := make(map[string]*pb.ServerRegion)
	for _, route := range routes {
		if route.OriginID == 0 || route.Version <= 0 || route.RegionID == "" || route.RegionName == "" || route.Name == "" {
			continue
		}
		region := regions[route.RegionID]
		if region == nil {
			region = &pb.ServerRegion{RegionId: route.RegionID, Name: route.RegionName}
			regions[route.RegionID] = region
			out.Regions = append(out.Regions, region)
		}
		region.Servers = append(region.Servers, &pb.ServerEntry{
			OriginServerId: route.OriginID, Name: route.Name, Available: available[route.RunID],
		})
	}
	return out, nil
}

// availableRuns 合并部署、运行服租约和节点租约。目录的在线标记只供展示，签票另做实时确认。
func (s *Store) availableRuns(ctx context.Context, routes []originRoute) (map[uint64]bool, error) {
	runSet := make(map[uint64]struct{}, len(routes))
	for _, route := range routes {
		runSet[route.RunID] = struct{}{}
	}
	runIDs := make([]uint64, 0, len(runSet))
	for id := range runSet {
		runIDs = append(runIDs, id)
	}
	available := make(map[uint64]bool)
	if len(runIDs) == 0 {
		return available, nil
	}
	cursor, err := s.DB.Collection("run_servers").Find(ctx, bson.M{"_id": bson.M{"$in": runIDs}, "state": "open"})
	if err != nil {
		return nil, err
	}
	var assignments []runAssignment
	if err = cursor.All(ctx, &assignments); err != nil {
		return nil, err
	}
	assigned := make(map[uint64]runAssignment, len(assignments))
	nodeSet := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if openAssignment(assignment) {
			assigned[assignment.RunID] = assignment
			nodeSet[assignment.GameNodeID] = struct{}{}
		}
	}
	if len(assigned) == 0 {
		return available, nil
	}
	nodeIDs := make([]string, 0, len(nodeSet))
	for id := range nodeSet {
		nodeIDs = append(nodeIDs, id)
	}
	cursor, err = s.DB.Collection("node_leases").Find(ctx,
		bson.M{"_id": bson.M{"$in": nodeIDs}, "kind": "game", "$expr": leaseFilter()})
	if err != nil {
		return nil, err
	}
	var nodes []leasedGameNode
	if err = cursor.All(ctx, &nodes); err != nil {
		return nil, err
	}
	leases := make(map[string]leasedGameNode, len(nodes))
	for _, node := range nodes {
		leases[node.ID] = node
	}
	cursor, err = s.DB.Collection("run_control").Find(ctx,
		bson.M{"_id": bson.M{"$in": runIDs}, "state": "active", "$expr": leaseFilter()})
	if err != nil {
		return nil, err
	}
	var controls []Run
	if err = cursor.All(ctx, &controls); err != nil {
		return nil, err
	}
	for _, current := range controls {
		if assigned, ok := assigned[current.ID]; ok && matchingRun(assigned, current) && matchingNode(leases[assigned.GameNodeID], current) {
			available[current.ID] = true
		}
	}
	return available, nil
}

// reserveTicketRoute 把新签票与运行服冻结、租约换代的组写屏障对齐，分散同服并发签票。
func (s *Store) reserveTicketRoute(ctx context.Context, run Run, accountID string) error {
	guard := bson.M{"_id": fmt.Sprintf("%d:%d", run.ID, runGuardGroup(accountID)),
		"owner_instance": run.Owner, "run_epoch": run.Epoch, "control_version": run.ControlVersion}
	updated, err := s.DB.Collection("run_guards").UpdateOne(ctx, guard, bson.M{"$inc": bson.M{"ticket_issue_seq": int64(1)}})
	if err != nil {
		return err
	}
	if updated.MatchedCount != 1 {
		return Fail(p.CodeLineRecovering, "签票期间运行服授权已变化")
	}
	return nil
}

// confirmTicketRoute 在入场事务中核对原票据是否仍绑定当前入口路由与部署。数据库异常由事务层处理。
func (s *Store) confirmTicketRoute(ctx context.Context, ticket Ticket) error {
	current, err := s.liveRoute(ctx, ticket.Origin)
	if err != nil {
		var fault *Fault
		if errors.As(err, &fault) {
			return Fail(p.CodeRouteChanged, "票据目标路由已变化")
		}
		return err
	}
	if current.origin.RunID != ticket.Run || current.origin.Version != ticket.RouteVersion ||
		current.assignment.GameNodeID != ticket.GameNodeID || current.assignment.AssignmentVersion != ticket.AssignmentVersion ||
		current.run.Owner != ticket.Owner || current.run.Epoch != ticket.Epoch {
		return Fail(p.CodeRouteChanged, "票据目标路由已变化")
	}
	return nil
}
