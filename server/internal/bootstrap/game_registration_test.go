// 本文件验证游戏进程启动时必须获得完整的多运行服授权及其各自归属。

package bootstrap

import (
	"testing"

	"minimir/internal/persistence"
)

// TestMatchGameGrantsRequiresExactSet 确保顺序可调整，缺失、重复、其他节点或过期授权不能据此开服。
func TestMatchGameGrantsRequiresExactSet(t *testing.T) {
	// first 和 second 属于同一启动实例，分别承载两个逻辑运行服。
	first := persistence.Run{ID: 10, Owner: "boot-a", GameNodeID: "node-a", State: "active", Epoch: 2, AssignmentVersion: 1}
	second := persistence.Run{ID: 20, Owner: "boot-a", GameNodeID: "node-a", State: "active", Epoch: 5, AssignmentVersion: 2}
	want := []uint64{10, 20}
	grants, err := matchGameGrants([]persistence.Run{second, first}, want, "node-a", "boot-a")
	if err != nil || len(grants) != len(want) || grants[0] != first || grants[1] != second {
		t.Fatalf("未按配置顺序保留两服授权：%+v，错误 %v", grants, err)
	}
	for _, bad := range [][]persistence.Run{
		{first}, {first, first}, {first, {ID: 20, Owner: "boot-b", GameNodeID: "node-a", State: "active", Epoch: 5, AssignmentVersion: 2}},
		{first, {ID: 20, Owner: "boot-a", GameNodeID: "node-b", State: "active", Epoch: 5, AssignmentVersion: 2}},
		{first, {ID: 20, Owner: "boot-a", GameNodeID: "node-a", State: "stopped", Epoch: 5, AssignmentVersion: 2}},
	} {
		if _, err := matchGameGrants(bad, want, "node-a", "boot-a"); err == nil {
			t.Fatalf("接受了不完整或不匹配的授权：%+v", bad)
		}
	}
}
