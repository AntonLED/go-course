// Команда kvservice — точка входа «Задания 9».
//
//	go run -tags solution ./modules/09-advanced/tasks/11_project_service/cmd/kvservice -addr :8080
//	curl -X PUT -d hello localhost:8080/kv/greeting
//	curl localhost:8080/kv/greeting
//	curl localhost:8080/metrics
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	service "gocourse/modules/09-advanced/tasks/11_project_service"
)

// version задаётся при сборке: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "kvservice:", err)
		os.Exit(1)
	}
}

// run отделён от main, чтобы defer отрабатывали до os.Exit.
func run() error {
	cfg, err := service.LoadConfig(os.Args[1:], os.LookupEnv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := service.New(cfg)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "kvservice %s слушает %s\n", version, ln.Addr())
	return app.Run(ctx, ln)
}
