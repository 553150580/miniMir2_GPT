package main

import (
	"context"
	"fmt"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/sync/errgroup"
)

type loadIdentity struct {
	ID        string `bson:"_id"`
	AccountID string `bson:"account_id"`
}

func seedRoles(ctx context.Context, cfg devconfig.Config, runID string, count int) error {
	if count < 1 || count > 5000 {
		return fmt.Errorf("无效的压测预置角色数 %d", count)
	}
	store, err := persistence.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close(context.Background())
	total, err := store.DB.Collection("roles").EstimatedDocumentCount(ctx)
	if err != nil {
		return err
	}
	if total != 0 {
		return fmt.Errorf("隔离测试库已有 %d 个角色", total)
	}
	_, err = store.DB.Collection("application_meta").UpdateOne(ctx, bson.M{"_id": "minimir_development"}, bson.M{"$set": bson.M{"hosting_load_run": runID}})
	if err != nil {
		return err
	}

	start := time.Now()
	accounts := make([]domain.Account, count)
	for i := range accounts {
		accounts[i] = domain.Account{ID: domain.ID(), Name: fmt.Sprintf("load%05d", i), Active: true, AuthVersion: 1}
	}
	if _, err = store.DB.Collection("accounts").InsertMany(ctx, accounts); err != nil {
		return err
	}
	group, work := errgroup.WithContext(ctx)
	group.SetLimit(12)
	for i, account := range accounts {
		group.Go(func() error {
			// 生成正式角色模板，随后仅提高测试战士的等级，使满负载时段持续战斗。
			role, createErr := store.CreateRole(work, persistence.Auth{AccountID: account.ID, AuthVersion: 1}, &pb.CreateRoleRequest{
				OriginServerId: cfg.OriginID, Slot: 1, Job: 0, Name: fmt.Sprintf("load%05d", i), OpId: domain.Bytes(domain.ID()),
			})
			if createErr != nil {
				return fmt.Errorf("创建测试角色 %d: %w", i, createErr)
			}
			_, updateErr := store.DB.Collection("roles").UpdateOne(work, bson.M{"_id": domain.Hex(role.Key.RoleId)}, bson.M{"$set": bson.M{"level": 45}})
			return updateErr
		})
	}
	if err = group.Wait(); err != nil {
		return err
	}
	return emit(map[string]any{"event": "seed", "roles": count, "job": "warrior", "level": 45, "seconds": time.Since(start).Seconds()})
}

func loadIdentities(ctx context.Context, store *persistence.Store, limit int) ([]loadIdentity, error) {
	cursor, err := store.DB.Collection("roles").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name_key", Value: 1}}).SetLimit(int64(limit)).SetProjection(bson.M{"account_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []loadIdentity
	if err = cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	if len(rows) != limit {
		return nil, fmt.Errorf("预期 %d 角色，实际 %d", limit, len(rows))
	}
	return rows, nil
}
