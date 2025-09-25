package main

import (
	"context"
	"embed"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/supabase/auth/cmd"
	"github.com/supabase/auth/internal/observability"
)

//go:embed migrations/*
var embeddedMigrations embed.FS

// 上面注释是把 migrations/ 下的 SQL 脚本嵌入到 Go 程序中。
func init() { // 初始化日志，并设置为json格式
	logrus.SetFormatter(&logrus.JSONFormatter{})
}

func main() {
	// 把嵌入的migrations提供给cmd，让命令行工具可以运行数据库迁移
	cmd.EmbeddedMigrations = embeddedMigrations

	execCtx, execCancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer execCancel()

	go func() {
		// 当收到退出信号时，就打印日志
		<-execCtx.Done() // 实现是一个只读通道
		logrus.Info("received graceful shutdown signal")
	}()
　　
	// 服务启动点
	// command is expected to obey the cancellation signal on execCtx and
	// block while it is running
	if err := cmd.RootCommand().ExecuteContext(execCtx); err != nil {
		logrus.WithError(err).Fatal(err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Minute)
	defer shutdownCancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		// wait for profiler, metrics and trace exporters to shut down gracefully
		observability.WaitForCleanup(shutdownCtx)
	}()

	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		wg.Wait()
	}()

	select {
	case <-shutdownCtx.Done():
		// cleanup timed out
		return

	case <-cleanupDone:
		// cleanup finished before timing out
		return
	}
}
