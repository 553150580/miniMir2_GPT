// 本文件联系账号节点登记本次游戏进程，并验证一组运行服授权完整匹配启动配置。

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

// gameRegistration 区分稳定节点、每次启动生成的实例及本次声明的执行范围。
type gameRegistration struct {
	NodeID string   `json:"game_node_id"`             // 跨重启保持的部署节点身份。
	BootID string   `json:"boot_id"`                  // 当前进程启动实例身份。
	RunID  uint64   `json:"run_server_id,omitempty"`  // 旧单服请求的运行服编号。
	RunIDs []uint64 `json:"run_server_ids,omitempty"` // 需要同时授权的多运行服集合。
}

// registerGameRuns 重试同一个启动实例与运行服集合，可接受账号节点对丢失响应的幂等重放。
func registerGameRuns(ctx context.Context, cfg devconfig.Config, nodeKey, bootID string, runIDs []uint64) ([]persistence.Run, error) {
	// payload 保存本次固定申请。只声明部署身份与运行服，不报客户端公告地址。
	payload := gameRegistration{NodeID: cfg.NodeID, BootID: bootID}
	multi := cfg.RunServerIDs != nil
	if multi {
		payload.RunIDs = runIDs
	} else {
		payload.RunID = runIDs[0]
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	endpoint := "http://" + cfg.AccountAddress + "/internal/register-game"
	for attempt := 0; attempt < 30; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return nil, requestErr
		}
		request.Header.Set("X-Dev-Node-Key", nodeKey)
		request.Header.Set("Content-Type", "application/json")
		response, attemptErr := client.Do(request)
		if attemptErr == nil {
			grants, decodeErr := decodeGameGrants(response, multi)
			attemptErr = decodeErr
			if attemptErr == nil {
				var ordered []persistence.Run
				ordered, attemptErr = matchGameGrants(grants, runIDs, cfg.NodeID, bootID)
				if attemptErr == nil {
					return ordered, nil
				}
			}
		}
		err = attemptErr
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("游戏注册失败: %w", err)
}

// decodeGameGrants 接受旧单服响应以及新接口的整组授权响应。
func decodeGameGrants(response *http.Response, multi bool) ([]persistence.Run, error) {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, fmt.Errorf("account未批准运行所有权，HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(response.Body)
	if multi {
		var result struct {
			Runs []persistence.Run `json:"runs"` // 同一授权事务返回的各运行服记录。
		}
		err := decoder.Decode(&result)
		return result.Runs, err
	}
	var run persistence.Run
	err := decoder.Decode(&run)
	return []persistence.Run{run}, err
}

// matchGameGrants 核对账号返回的完整集合及每服的实例归属，并按配置顺序安排 Store 视图。
func matchGameGrants(grants []persistence.Run, runIDs []uint64, nodeID, bootID string) ([]persistence.Run, error) {
	if len(grants) != len(runIDs) {
		return nil, fmt.Errorf("账号返回 %d 个运行服授权，预期 %d 个", len(grants), len(runIDs))
	}
	// slots 记录每个预期运行服的位置，成功匹配后移除，重复授权会直接拒绝。
	slots := make(map[uint64]int, len(runIDs))
	for index, id := range runIDs {
		slots[id] = index
	}
	ordered := make([]persistence.Run, len(runIDs))
	for _, grant := range grants {
		index, exists := slots[grant.ID]
		if !exists || grant.Owner != bootID || grant.GameNodeID != nodeID || grant.State != "active" || grant.Epoch <= 0 || grant.AssignmentVersion <= 0 {
			return nil, fmt.Errorf("账号返回的运行服 %d 授权与启动身份不匹配", grant.ID)
		}
		ordered[index] = grant
		delete(slots, grant.ID)
	}
	return ordered, nil
}
