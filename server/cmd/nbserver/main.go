// 本文件解析装配角色和配置路径，建立结构化日志和进程取消信号，再启动对应开发节点。

package main

import (
	"flag"
	"log/slog"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"os"
)

// main 加载启动配置，记录装配身份，并按指定模式调用节点入口。
func main() {
	// config 接收从命令行指定的启动配置路径。
	config := flag.String("config", "../conf/dev/server.json", "开发服务配置")
	// mode 接收要运行的监管、账号、游戏或 GM 节点模式。
	mode := flag.String("mode", "supervisor", "account/game/gm/supervisor")
	// runID 标记一次本机装配，使不同进程轮次的日志可区分。
	runID := flag.String("run-id", "manual", "本次开发装配运行标识")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Info("装配启动", "mode", *mode, "run_id", *runID)
	// cfg 保存已校验的启动配置与玩法快照，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load(*config)
	if err != nil {
		slog.Error("配置未通过", "error", err)
		os.Exit(2)
	}
	// ctx 保存由主流程上下文取得的上下文，stop 取消对应上下文或停止计时，释放其关联资源。
	ctx, stop := bootstrap.MainContext()
	defer stop()
	err = bootstrap.Run(ctx, cfg, *mode)
	if *mode == "supervisor" {
		slog.Info("装配停止结果", "mode", *mode, "run_id", *runID, "ok", err == nil, "error", err)
	}
	if err != nil {
		slog.Error("服务退出", "mode", *mode, "error", err)
		os.Exit(1)
	}
}
