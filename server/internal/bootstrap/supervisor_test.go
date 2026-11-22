// 本文件核对监管进程把调用者提供的配置文件和节点模式传给原程序。

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"minimir/internal/devconfig"
)

// TestSupervisorChildrenRetainSelectedConfiguration 核验子进程应保留选中配置，用诊断「子进程切换了原数据库、监听地址或候选规则」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSupervisorChildrenRetainSelectedConfiguration(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.FixtureFile = filepath.Join(cfg.Root, cfg.FixtureFile)
	cfg.Database = "minimir_dev_game_test_selected_config"
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = "127.0.0.1:33001", "127.0.0.1:33002", "127.0.0.1:33003"
	// raw 保存由序列化取得的原始值。
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// path 保存由加入取得的路径。
	path := filepath.Join(t.TempDir(), "候选 配置.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// selected 保存由加载取得的已选择。
	selected, err := devconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game", "gm"} {
		// cmd 保存由进程监管器子进程取得的“cmd”。
		cmd := supervisorChild("nbserver", selected, mode)
		if len(cmd.Args) != 5 || cmd.Args[1] != "-config" || cmd.Args[2] != path || cmd.Args[3] != "-mode" || cmd.Args[4] != mode {
			t.Fatalf("子进程没有保留选中配置: %v", cmd.Args)
		}
		// actual 保存由加载取得的实测，err 接收加载过程中可能产生的错误，供后续失败分支处理。
		actual, err := devconfig.Load(cmd.Args[2])
		if err != nil {
			t.Fatal(err)
		}
		if actual.Database != cfg.Database || actual.AccountAddress != cfg.AccountAddress || actual.GameAddress != cfg.GameAddress || actual.GMAddress != cfg.GMAddress || actual.Fixture.Hash != cfg.Fixture.Hash {
			t.Fatal("子进程切换了原数据库、监听地址或候选规则")
		}
	}
}

// TestSupervisorRejectsMissingConfigurationSourceBeforeStarting 核对「没有实际加载来源仍启动子节点」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSupervisorRejectsMissingConfigurationSourceBeforeStarting(t *testing.T) {
	// err 接收“supervise”过程中可能产生的错误，供后续失败分支处理。
	if err := supervise(context.Background(), devconfig.Config{}); err == nil {
		t.Fatal("没有实际加载来源仍启动子节点")
	}
}

func TestSupervisorReportsGameFailureOnRequestedShutdown(t *testing.T) {
	drainFailure := errors.New("停服等待结算未完成")
	for _, gameError := range []error{nil, drainFailure} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		exits := make(chan supervisedChildExit, 3)
		err := waitForChildren(ctx, exits, 3, func() {
			exits <- supervisedChildExit{mode: "account"}
			exits <- supervisedChildExit{mode: "gm"}
			exits <- supervisedChildExit{mode: "game", err: gameError}
		})
		if !errors.Is(err, gameError) || err != nil && !strings.Contains(err.Error(), "game节点退出") || len(exits) != 0 {
			t.Fatalf("监管进程没有完整确认各节点的退出: %v", err)
		}
	}
}
