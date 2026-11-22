package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"minimir/internal/devconfig"
	"minimir/internal/game"
	"minimir/internal/persistence"
	"minimir/internal/transport"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

func NodeKey() (string, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	dir := filepath.Join(home, ".local/share/minimir-toolchain/game-dev")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(dir, "node.key")
	if info, e := os.Lstat(path); e == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("拒绝节点密钥符号链接")
		}
		b, e := os.ReadFile(path)
		if e == nil && len(b) != 64 {
			return "", errors.New("节点密钥尚未完整写入")
		}
		return string(b), e
	}
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return "", e
	}
	text := hex.EncodeToString(b)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(e) {
		b, e := os.ReadFile(path)
		if e == nil && len(b) != 64 {
			return "", errors.New("节点密钥尚未完整写入")
		}
		return string(b), e
	}
	if e != nil {
		return "", e
	}
	_, e = f.WriteString(text)
	f.Close()
	return text, e
}
func Run(ctx context.Context, cfg devconfig.Config, mode string) error {
	if mode == "supervisor" {
		return supervise(ctx, cfg)
	}
	if mode != "account" && mode != "game" && mode != "gm" {
		return errors.New("mode必须为account/game/gm/supervisor")
	}
	parent := ctx
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	store, e := persistence.Open(ctx, cfg)
	if e != nil {
		return e
	}
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), nodeLeaseTimeout)
		defer stop()
		store.Close(closeCtx)
	}()
	key, e := NodeKey()
	if e != nil {
		return e
	}
	if mode == "game" {
		body, _ := json.Marshal(map[string]any{"node_id": cfg.NodeID, "instance_id": store.Instance, "run_id": cfg.RunID})
		client := &http.Client{Timeout: 5 * time.Second}
		var last error
		for attempt := 0; attempt < 30; attempt++ {
			request, _ := http.NewRequestWithContext(ctx, "POST", "http://"+cfg.AccountAddress+"/internal/register-game", bytes.NewReader(body))
			request.Header.Set("X-Dev-Node-Key", key)
			response, err := client.Do(request)
			if err == nil {
				if response.StatusCode == 200 {
					var grant persistence.Run
					err = json.NewDecoder(response.Body).Decode(&grant)
					response.Body.Close()
					if err == nil && grant.Owner == store.Instance {
						store.Epoch = grant.Epoch
						last = nil
						break
					}
				} else {
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
					err = errors.New("account尚未批准运行所有权")
				}
			}
			last = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		if store.Epoch == 0 {
			return fmt.Errorf("游戏注册失败: %w", last)
		}
	} else {
		if e = store.Singleton(ctx, mode); e != nil {
			return e
		}
	}
	defer releaseNodeLease(func(releaseCtx context.Context) error { return store.Release(releaseCtx, mode) })
	renew := func(attempt context.Context) error { return store.Renew(attempt, mode) }
	confirmed, e := confirmInitialNodeLease(ctx, renew)
	if e != nil {
		return fmt.Errorf("初始节点续租未获确认：%w", e)
	}
	server := &transport.Server{Store: store, Mode: mode, NodeKey: key}
	var engine *game.Engine
	leaseContext := ctx
	cancelEngine := context.CancelCauseFunc(func(error) {})
	if mode == "game" {
		// 主动停止先结清原流水；租约失败直接取消房间执行与结清等待。
		leaseContext, cancelEngine = context.WithCancelCause(context.WithoutCancel(ctx))
		defer cancelEngine(nil)
		engine = game.New(leaseContext, store)
		server.Engine = engine
		defer engine.Stop()
	}
	address := cfg.AccountAddress
	if mode == "game" {
		address = cfg.GameAddress
	} else if mode == "gm" {
		address = cfg.GMAddress
	}
	httpServer := &http.Server{Addr: address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		MaxHeaderBytes: 16 * 1024, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- httpServer.ListenAndServe() }()
	leaseEvents := make(chan nodeLeaseEvent, 4)
	go logNodeLeaseEvents(leaseContext, mode, leaseEvents)
	go func() {
		if err := watchNodeLease(leaseContext, confirmed, renew, leaseEvents); err != nil && leaseContext.Err() == nil {
			cancelEngine(err)
			cancel(err)
			slog.Error("节点租约失败，隔离本进程", "mode", mode, "error", err)
		}
	}()
	slog.Info("开发节点就绪", "mode", mode, "address", address, "database", cfg.Database, "instance", store.Instance, "runtime_ready", false)
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			cancelEngine(err)
			cancel(err)
			return err
		}
	case <-ctx.Done():
	}
	shutdown, stop := context.WithTimeout(context.Background(), 8*time.Second)
	defer stop()
	var drainErr error
	if engine != nil && parent.Err() != nil {
		drainErr = engine.DrainSettlements(shutdown)
	}
	e = httpServer.Shutdown(shutdown)
	if parent.Err() == nil {
		return errors.Join(e, context.Cause(ctx))
	}
	return errors.Join(e, drainErr)
}
func supervise(ctx context.Context, cfg devconfig.Config) error {
	if !filepath.IsAbs(cfg.SourcePath) {
		return errors.New("监督进程缺少实际加载的配置文件绝对路径")
	}
	binary, e := os.Executable()
	if e != nil {
		return e
	}
	if _, e := NodeKey(); e != nil {
		return e
	}
	childExit := make(chan error, 3)
	modes := []string{"account", "game", "gm"}
	var children []*exec.Cmd
	var wg sync.WaitGroup
	for _, mode := range modes {
		cmd := supervisorChild(binary, cfg, mode)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		prepareChild(cmd)
		if e = cmd.Start(); e != nil {
			return e
		}
		children = append(children, cmd)
		wg.Go(func() {
			err := cmd.Wait()
			childExit <- fmt.Errorf("%s节点退出: %v", mode, err)
		})
	}
	var failure error
	select {
	case <-ctx.Done():
	case failure = <-childExit:
	}
	for _, child := range children {
		if child.Process != nil {
			stopChild(child)
		}
	}
	wg.Wait()
	return failure
}

func supervisorChild(binary string, cfg devconfig.Config, mode string) *exec.Cmd {
	// 保留调用者选定并已校验的原文件；不能把候选装配切回默认数据库或端口。
	return exec.Command(binary, "-config", cfg.SourcePath, "-mode", mode)
}
func MainContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
