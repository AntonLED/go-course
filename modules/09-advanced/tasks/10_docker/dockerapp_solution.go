//go:build solution

package dockerapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// dockerfilePath — эталонный Dockerfile.
const dockerfilePath = "Dockerfile.solution"

// ReadInfo: значения из -ldflags имеют приоритет, остальное берём из
// информации, которую компилятор вшивает в бинарник (go version -m ./bin).
func ReadInfo(version, commit string) Info {
	info := Info{Version: version, Commit: commit}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = bi.GoVersion
		info.Module = bi.Main.Path
		if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version // go install module@v1.2.3
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.modified":
				info.Dirty = s.Value == "true"
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	return info
}

// NewHandler: liveness, readiness и версия.
func NewHandler(info Info, ready func() bool) http.Handler {
	mux := http.NewServeMux()
	// liveness: процесс жив и обслуживает HTTP. Никаких проверок БД здесь —
	// иначе падение БД приведёт к рестарту всех подов.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	// readiness: готов ли принимать трафик (прогрет кэш, есть соединение с БД...).
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready != nil && !ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ready\n")
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})
	return mux
}

// Run обслуживает HTTP до отмены ctx, затем graceful shutdown.
func Run(ctx context.Context, ln net.Listener, h http.Handler, shutdownTimeout time.Duration) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second, // защита от Slowloris
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		// Serve завершился сам (например, listener закрыт извне).
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	// ctx уже отменён — для Shutdown нужен новый контекст с таймаутом.
	// WithoutCancel сохраняет значения родителя, но не его отмену.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	// Shutdown: закрывает listener, ждёт завершения активных запросов.
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close() // таймаут: рвём оставшиеся соединения
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Probe делает GET url и возвращает ошибку, если ответ не 200.
func Probe(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // дочитываем для переиспользования соединения
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: статус %d", url, resp.StatusCode)
	}
	return nil
}

// MaxProcsFromCgroup — как uber-go/automaxprocs: floor(quota/period), минимум 1.
func MaxProcsFromCgroup(cpuMax string) (int, error) {
	fields := strings.Fields(cpuMax)
	if len(fields) == 0 || len(fields) > 2 {
		return 0, fmt.Errorf("cpu.max: неожиданный формат %q", cpuMax)
	}
	if fields[0] == "max" {
		return 0, nil // лимита нет
	}
	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || quota <= 0 {
		return 0, fmt.Errorf("cpu.max: некорректная квота %q", fields[0])
	}
	period := int64(100000) // значение по умолчанию в ядре
	if len(fields) == 2 {
		period, err = strconv.ParseInt(fields[1], 10, 64)
		if err != nil || period <= 0 {
			return 0, fmt.Errorf("cpu.max: некорректный период %q", fields[1])
		}
	}
	return max(1, int(quota/period)), nil
}
