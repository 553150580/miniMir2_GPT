// 本文件装配账号、游戏和 GM 节点，加载内部密钥、登记租约并管理受控退出与监管子进程。

package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	"syscall"
	"time"
)

// NodeKey 读取或首次生成同机节点登记密钥，校验存量文件的类型和完整长度。
func NodeKey() (string, error) {
	// home 提供当前系统用户的主目录，用于定位私有数据，e 接收“user”归属方向或目录供后续步骤使用。
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	// dir 保存本机开发节点私有文件的目录。
	dir := filepath.Join(home, ".local/share/minimir-toolchain/game-dev")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	// path 定位同一装配下各进程共用的密钥文件。
	path := filepath.Join(dir, "node.key")
	// info 用于检查存量密钥是否为符号链接，e 接收“lstat”供后续步骤使用。
	if info, e := os.Lstat(path); e == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("拒绝节点密钥符号链接")
		}
		// b 暂存待读取或编码的密钥字节，e 接收读取文件供后续步骤使用。
		b, e := os.ReadFile(path)
		if e == nil && len(b) != 64 {
			return "", errors.New("节点密钥尚未完整写入")
		}
		return string(b), e
	}
	// b 暂存待读取或编码的密钥字节。
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return "", e
	}
	// text 保存可写入文件及内部请求头的十六进制密钥。
	text := hex.EncodeToString(b)
	// 独占创建保证并行启动的进程共用先写入的密钥。
	// f 持有首次独占创建成功的密钥文件句柄。
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(e) {
		// b 暂存待读取或编码的密钥字节，e 接收读取文件供后续步骤使用。
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

// Run 完成节点登记和首个租约确认，启动 HTTP 服务并协调退场清算。
// ctx 向下传递当前请求的截止时间和取消信号，cfg 记录服务配置，mode 记录模式，使用文本表示。
func Run(ctx context.Context, cfg devconfig.Config, mode string) error {
	if mode == "supervisor" {
		return supervise(ctx, cfg)
	}
	if mode != "account" && mode != "game" && mode != "gm" {
		return errors.New("mode必须为account/game/gm/supervisor")
	}
	// parent 保留外部生命周期，区分主动停服和节点内部失权退出。
	parent := ctx
	// cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	// store 保存由打开取得的持久层存储，e 接收打开供后续步骤使用。
	store, e := persistence.Open(ctx, cfg)
	if e != nil {
		return e
	}
	defer func() {
		// closeCtx 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		closeCtx, stop := context.WithTimeout(context.Background(), nodeLeaseTimeout)
		defer stop()
		store.Close(closeCtx)
	}()
	// key 保存由节点键取得的键。
	key, e := NodeKey()
	if e != nil {
		return e
	}
	// gameStores 独立绑定各运行服与其所有权代次，数据库连接池仍由根 store 管理。
	var gameStores []*persistence.Store
	var hostedRunIDs []uint64
	if mode == "game" {
		hostedRunIDs, e = cfg.HostedRunIDs()
		if e != nil {
			return e
		}
		// 对外监听前确认 account 一次批准全部运行服，再安装续租与停服使用的完整授权集。
		grants, err := registerGameRuns(ctx, cfg, key, store.Instance, hostedRunIDs)
		if err != nil {
			return err
		}
		gameStores, e = store.GameRunStores(grants)
		if e != nil {
			return e
		}
	} else {
		if e = store.Singleton(ctx, mode); e != nil {
			return e
		}
	}
	// releaseCtx 记录释放上下文，使用请求或事务上下文表示。
	defer releaseNodeLease(func(releaseCtx context.Context) error { return store.Release(releaseCtx, mode) })
	// renew 封装续租对应的后续处理，attempt 记录“attempt”，使用请求或事务上下文表示。
	renew := func(attempt context.Context) error { return store.Renew(attempt, mode) }
	// confirmed 保存由确认初始值节点租约取得的已确认。
	confirmed, e := confirmInitialNodeLease(ctx, renew)
	if e != nil {
		return fmt.Errorf("初始节点续租未获确认：%w", e)
	}
	// server 保存当前处理所用的服务端。
	server := &transport.Server{Store: store, Mode: mode, NodeKey: key, HostedRunIDs: hostedRunIDs}
	// engine 记录游戏引擎。
	var engine *game.Engine
	// leaseContext 维护租约监视和游戏引擎的生命周期，允许主动停服先结清。
	leaseContext := ctx
	// cancelEngine 保存由取消函数原因“func”取得的取消函数游戏引擎。
	cancelEngine := context.CancelCauseFunc(func(error) {})
	if mode == "game" {
		// 主动停止先结清原流水；租约失败直接取消房间执行与结清等待。
		leaseContext, cancelEngine = context.WithCancelCause(context.WithoutCancel(ctx))
		defer cancelEngine(nil)
		engine = game.NewRunEngine(leaseContext, gameStores)
		server.Engine = engine
		defer engine.Stop()
	}
	// address 保存账号地址，用于当前步骤中的地址。
	address := cfg.AccountAddress
	if mode == "game" {
		address = cfg.GameAddress
	} else if mode == "gm" {
		address = cfg.GMAddress
	}
	// httpServer 保存当前处理所用的HTTP服务端。
	httpServer := &http.Server{Addr: address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		MaxHeaderBytes: 16 * 1024, BaseContext: func(net.Listener) context.Context { return ctx }}
	// done 通知装配层 HTTP 监听已经停止或发生错误。
	done := make(chan error, 1)
	go func() { done <- httpServer.ListenAndServe() }()
	// leaseEvents 创建用于维护租约事件集合的容器或通道。
	leaseEvents := make(chan nodeLeaseEvent, 4)
	go logNodeLeaseEvents(leaseContext, mode, leaseEvents)
	// 游戏房间和 HTTP 接入都依赖续租结果，监视器失权时同时取消二者。
	go func() {
		// err 接收监视节点租约过程中可能产生的错误，供后续失败分支处理。
		if err := watchNodeLease(leaseContext, confirmed, renew, leaseEvents); err != nil && leaseContext.Err() == nil {
			cancelEngine(err)
			cancel(err)
			slog.Error("节点租约失败，隔离本进程", "mode", mode, "error", err)
		}
	}()
	slog.Info("开发节点就绪", "mode", mode, "address", address, "database", cfg.Database, "instance", store.Instance, "runtime_ready", false)
	select {
	// err 保存当前操作的失败原因，供调用方检查或返回。
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			cancelEngine(err)
			cancel(err)
			return err
		}
	case <-ctx.Done():
	}
	// shutdown 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
	shutdown, stop := context.WithTimeout(context.Background(), 8*time.Second)
	defer stop()
	// drainErr 记录收束错误。
	var drainErr error
	// 只有上级主动停服才启动结清。失权退出由原数据库围栏拦截过时代次的事务。
	if engine != nil && parent.Err() != nil {
		drainErr = engine.DrainSettlements(shutdown)
	}
	e = httpServer.Shutdown(shutdown)
	if parent.Err() == nil {
		return errors.Join(e, context.Cause(ctx))
	}
	return errors.Join(e, drainErr)
}

// supervise 从同一可执行文件拉起三个节点，接到退出信号后通知并等待所有子进程。
// ctx 向下传递当前请求的截止时间和取消信号，cfg 记录服务配置。
func supervise(ctx context.Context, cfg devconfig.Config) error {
	if !filepath.IsAbs(cfg.SourcePath) {
		return errors.New("监督进程缺少实际加载的配置文件绝对路径")
	}
	// binary 保存由“executable”取得的“binary”，e 接收“executable”供后续步骤使用。
	binary, e := os.Executable()
	if e != nil {
		return e
	}
	// e 接收节点键供后续步骤使用。
	if _, e := NodeKey(); e != nil {
		return e
	}
	childExit := make(chan supervisedChildExit, 3)
	// modes 初始化用作模式集合的文本列表。
	modes := []string{"account", "game", "gm"}
	// children 记录“children”，使用“cmd”列表表示。
	var children []*exec.Cmd
	stop := func() {
		for _, child := range children {
			stopChild(child)
		}
	}
	// mode 接收遍历模式集合时当前的元素。
	for _, mode := range modes {
		// cmd 保存由进程监管器子进程取得的“cmd”。
		cmd := supervisorChild(binary, cfg, mode)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		prepareChild(cmd)
		if e = cmd.Start(); e != nil {
			stop()
			return errors.Join(fmt.Errorf("启动%s节点: %w", mode, e), collectChildExits(childExit, len(children)))
		}
		children = append(children, cmd)
		go func() { childExit <- supervisedChildExit{mode, cmd.Wait()} }()
	}
	return waitForChildren(ctx, childExit, len(children), stop)
}

type supervisedChildExit struct {
	mode string
	err  error
}

// 收到停服请求后仍核对所有节点的退出结果，包括游戏节点的结清错误。
func waitForChildren(ctx context.Context, exits <-chan supervisedChildExit, remaining int, stop func()) error {
	var failure error
	select {
	case <-ctx.Done():
	case exit := <-exits:
		remaining--
		failure = exit.failure()
		if failure == nil && ctx.Err() == nil {
			failure = fmt.Errorf("%s节点在请求停服前已退出", exit.mode)
		}
	}
	stop()
	return errors.Join(failure, collectChildExits(exits, remaining))
}

func collectChildExits(exits <-chan supervisedChildExit, remaining int) error {
	var failure error
	for range remaining {
		if err := (<-exits).failure(); err != nil {
			failure = errors.Join(failure, err)
		}
	}
	return failure
}

func (exit supervisedChildExit) failure() error {
	if exit.err == nil {
		return nil
	}
	return fmt.Errorf("%s节点退出: %w", exit.mode, exit.err)
}

// supervisorChild 返回进程监管器子进程供调用方使用。
// binary 记录“binary”，使用文本表示，cfg 记录服务配置，mode 记录模式，使用文本表示。
func supervisorChild(binary string, cfg devconfig.Config, mode string) *exec.Cmd {
	// 保留调用者选定并已校验的原文件；不能把候选装配切回默认数据库或端口。
	return exec.Command(binary, "-config", cfg.SourcePath, "-mode", mode)
}

// MainContext 将操作系统的中断和终止信号转换为装配取消信号。
func MainContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
