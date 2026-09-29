// Команда server — точка входа для Docker-образа из задачи 10.
//
// Сборка локально:
//
//	go build -trimpath -ldflags "-s -w -X main.version=1.2.3 -X main.commit=$(git rev-parse --short HEAD)" \
//	    -o server ./modules/09-advanced/tasks/10_docker/cmd/server
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	dockerapp "gocourse/modules/09-advanced/tasks/10_docker"
)

// Заполняются при сборке: -ldflags "-X main.version=... -X main.commit=...".
// Должны быть переменными (не константами) типа string.
var (
	version = ""
	commit  = ""
)

func main() {
	addr := flag.String("addr", ":8080", "адрес HTTP-сервера")
	healthcheck := flag.Bool("healthcheck", false, "проверить /healthz и выйти (для HEALTHCHECK)")
	flag.Parse()

	if *healthcheck {
		_, port, _ := net.SplitHostPort(*addr)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := dockerapp.Probe(ctx, "http://127.0.0.1:"+port+"/healthz"); err != nil {
			os.Exit(1)
		}
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// PID 1 в контейнере: SIGTERM от `docker stop` приходит нам напрямую,
	// только если ENTRYPOINT в exec-форме (без /bin/sh -c).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if data, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		if n, err := dockerapp.MaxProcsFromCgroup(string(data)); err == nil && n > 0 && n < runtime.GOMAXPROCS(0) {
			runtime.GOMAXPROCS(n) // до Go 1.25 рантайм не учитывает CPU-лимит cgroup
		}
	}

	info := dockerapp.ReadInfo(version, commit)
	log.Info("старт", "version", info.Version, "commit", info.Commit, "go", info.GoVersion,
		"gomaxprocs", runtime.GOMAXPROCS(0), "addr", *addr)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	if err := dockerapp.Run(ctx, ln, dockerapp.NewHandler(info, nil), 10*time.Second); err != nil {
		log.Error("сервер остановлен с ошибкой", "err", err)
		os.Exit(1)
	}
	log.Info("остановлен")
}
