package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"minimir/internal/devconfig"
)

func TestSupervisorChildrenRetainSelectedConfiguration(t *testing.T) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.FixtureFile = filepath.Join(cfg.Root, cfg.FixtureFile)
	cfg.Database = "minimir_dev_game_test_selected_config"
	cfg.AccountAddress, cfg.GameAddress, cfg.GMAddress = "127.0.0.1:33001", "127.0.0.1:33002", "127.0.0.1:33003"
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "候选 配置.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := devconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"account", "game", "gm"} {
		cmd := supervisorChild("nbserver", selected, mode)
		if len(cmd.Args) != 5 || cmd.Args[1] != "-config" || cmd.Args[2] != path || cmd.Args[3] != "-mode" || cmd.Args[4] != mode {
			t.Fatalf("子进程没有保留选中配置: %v", cmd.Args)
		}
		actual, err := devconfig.Load(cmd.Args[2])
		if err != nil {
			t.Fatal(err)
		}
		if actual.Database != cfg.Database || actual.AccountAddress != cfg.AccountAddress || actual.GameAddress != cfg.GameAddress || actual.GMAddress != cfg.GMAddress || actual.Fixture.Hash != cfg.Fixture.Hash {
			t.Fatal("子进程切换了原数据库、监听地址或候选规则")
		}
	}
}

func TestSupervisorRejectsMissingConfigurationSourceBeforeStarting(t *testing.T) {
	if err := supervise(context.Background(), devconfig.Config{}); err == nil {
		t.Fatal("没有实际加载来源仍启动子节点")
	}
}
