package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/game"
	"minimir/internal/persistence"
	"minimir/internal/transport"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
)

type arguments struct {
	mode, configPath, mongoURI, runID, profile string
	account, game, diagnostic, seed, parallel  int
	fullHold                                   time.Duration
}

func main() {
	var a arguments
	flag.StringVar(&a.mode, "mode", "", "进程职责")
	flag.StringVar(&a.configPath, "config", "conf/dev/server.json", "原始开发配置")
	flag.StringVar(&a.mongoURI, "mongo", "", "隔离副本集")
	flag.StringVar(&a.runID, "run", "", "隔离运行标识")
	flag.StringVar(&a.profile, "profile", "default", "压测入口配额")
	flag.IntVar(&a.account, "account", 0, "隔离账号端口")
	flag.IntVar(&a.game, "game", 0, "隔离游戏端口")
	flag.IntVar(&a.diagnostic, "diagnostic", 0, "隔离监测端口")
	flag.IntVar(&a.seed, "seed", 5000, "测试角色数量")
	flag.IntVar(&a.parallel, "parallel", 16, "入场并行数")
	flag.DurationVar(&a.fullHold, "hold", 180*time.Second, "满负载观察时长")
	flag.Parse()
	if err := run(a); err != nil {
		log.Fatal(err)
	}
}

func run(a arguments) error {
	ctx, cancel := bootstrap.MainContext()
	defer cancel()
	if !regexp.MustCompile("^[a-f0-9]{32}$").MatchString(a.runID) {
		return errors.New("隔离运行标识无效")
	}
	u, err := url.Parse(a.mongoURI)
	if err != nil || u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Query().Get("directConnection") != "true" || u.Query().Get("replicaSet") != "minimirLoad_"+a.runID {
		return errors.New("隔离副本集身份无效")
	}
	mongoPort, err := strconv.Atoi(u.Port())
	if err != nil || mongoPort < 1024 || mongoPort == 27118 || mongoPort == 27119 {
		return errors.New("隔离副本集端口无效")
	}
	ports := map[int]bool{mongoPort: true}
	for _, port := range []int{a.account, a.game, a.diagnostic} {
		if port < 1024 || port > 65535 || port >= 18080 && port <= 18102 || ports[port] {
			return errors.New("隔离服务端口无效")
		}
		ports[port] = true
	}
	cfg, err := devconfig.Load(a.configPath)
	if err != nil {
		return err
	}
	if cfg.Fixture.RuleVersion != 26 || cfg.Fixture.Map.Width != 13 || cfg.Fixture.Map.Height != 13 {
		return errors.New("需要规则26的13×13个人地图")
	}
	cfg.Database = "minimir_dev_game_test_load_" + a.runID
	cfg.MongoURI = a.mongoURI
	cfg.NodeID = "hosting-load-" + a.runID[:12]
	cfg.AccountAddress = fmt.Sprintf("127.0.0.1:%d", a.account)
	cfg.GameAddress = fmt.Sprintf("127.0.0.1:%d", a.game)
	statsAddress := fmt.Sprintf("127.0.0.1:%d", a.diagnostic)
	switch a.mode {
	case "account", "game":
		go func() { io.Copy(io.Discard, os.Stdin); cancel() }()
		if a.mode == "game" {
			switch a.profile {
			case "default":
			case "raised", "prejoin":
				transport.HostingLoadConnectionLimit = 5500
				game.HostingLoadRoomLimit = 768
			default:
				return errors.New("未知配额配置")
			}
			stop, err := startDiagnostics(statsAddress)
			if err != nil {
				return err
			}
			defer stop()
		}
		return bootstrap.Run(ctx, cfg, a.mode)
	case "seed":
		return seedRoles(ctx, cfg, a.runID, a.seed)
	case "clients":
		return runClients(ctx, cfg, statsAddress, a)
	default:
		return errors.New("未知进程职责")
	}
}

func emit(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }

func statsRequest(ctx context.Context, address, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+path, nil)
	if err != nil {
		return err
	}
	res, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("监测接口状态 %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(dst)
}

func diagnosticMongo(m *hostingLoadMongoMetrics) {
	persistence.HostingLoadCommandMonitor = m.Monitor()
	persistence.HostingLoadObserveTx = m.RecordTransaction
}
