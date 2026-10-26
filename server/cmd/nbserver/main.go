package main

import (
	"flag"
	"log/slog"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"os"
)

func main() {
	config := flag.String("config", "../conf/dev/server.json", "开发服务配置")
	mode := flag.String("mode", "supervisor", "account/game/gm/supervisor")
	runID := flag.String("run-id", "manual", "本次开发装配运行标识")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Info("装配启动", "mode", *mode, "run_id", *runID)
	cfg, err := devconfig.Load(*config)
	if err != nil {
		slog.Error("配置未通过", "error", err)
		os.Exit(2)
	}
	ctx, stop := bootstrap.MainContext()
	defer stop()
	if err = bootstrap.Run(ctx, cfg, *mode); err != nil {
		slog.Error("服务退出", "mode", *mode, "error", err)
		os.Exit(1)
	}
}
