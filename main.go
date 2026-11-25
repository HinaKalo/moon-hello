// moon 应用模板：HTTP 服务 + 共享 Postgres + 共享 Valkey。
// 连接信息由 moon-deployd 注入：DATABASE_URL、REDIS_URL。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit")
	flag.Parse()
	addr := getenv("LISTEN_ADDR", ":8080")
	if *healthcheck {
		os.Exit(probe("http://127.0.0.1" + addr + "/healthz"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var db *pgxpool.Pool
	if url := os.Getenv("DATABASE_URL"); url != "" {
		cfg, err := pgxpool.ParseConfig(url)
		if err != nil {
			log.Fatalf("DATABASE_URL: %v", err)
		}
		cfg.MaxConns = 5
		if db, err = pgxpool.NewWithConfig(ctx, cfg); err != nil {
			log.Fatalf("postgres: %v", err)
		}
		defer db.Close()
		if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS visits (id bigserial PRIMARY KEY, at timestamptz NOT NULL DEFAULT now())`); err != nil {
			log.Fatalf("migrate: %v", err)
		}
	}

	var rdb *redis.Client
	if url := os.Getenv("REDIS_URL"); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			log.Fatalf("REDIS_URL: %v", err)
		}
		rdb = redis.NewClient(opt)
		defer rdb.Close()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if db != nil {
			if err := db.Ping(c); err != nil {
				http.Error(w, "postgres: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		if rdb != nil {
			if err := rdb.Ping(c).Err(); err != nil {
				http.Error(w, "redis: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		out := map[string]any{"app": getenv("APP_NAME", "moon-app"), "time": time.Now().UTC()}
		if db != nil {
			var n int64
			err := db.QueryRow(r.Context(), `WITH ins AS (INSERT INTO visits DEFAULT VALUES RETURNING 1) SELECT count(*) + 1 FROM visits`).Scan(&n)
			out["postgres_visits"], out["postgres_error"] = n, errString(err)
		}
		if rdb != nil {
			n, err := rdb.Incr(r.Context(), "hits").Result()
			out["redis_hits"], out["redis_error"] = n, errString(err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func probe(url string) int {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func errString(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}
