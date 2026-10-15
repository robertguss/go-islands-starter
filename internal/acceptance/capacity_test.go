//go:build acceptance

package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robertguss/go-islands-starter/internal/auth"
	"github.com/robertguss/go-islands-starter/internal/db"
	"github.com/robertguss/go-islands-starter/internal/exports"
	"github.com/robertguss/go-islands-starter/internal/testdb"
)

func TestProductionCapacityAndRestore(t *testing.T) {
	image := os.Getenv("ISLANDS_TEST_IMAGE")
	if image == "" {
		t.Skip("set ISLANDS_TEST_IMAGE to the locally built production image")
	}
	st, fixture := testdb.New(t)
	ctx := context.Background()
	name := fixture.Config().ConnConfig.Database
	container := "islands-acceptance-" + name
	command := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %s failed: %s", args[0], out)
		}
		return out
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", container).Run()
	})
	if _, err := fixture.Exec(ctx, "INSERT INTO app.requests(owner_id,title,description) SELECT $1, 'Synthetic request ' || n, 'Capacity fixture' FROM generate_series(1,1000) n", testdb.Alice); err != nil {
		t.Fatal(err)
	}
	const token = "synthetic-capacity-session-token"
	if err := st.Auth.CreateSession(ctx, db.CreateSessionParams{TokenHash: auth.Hash(token), UserID: testdb.Alice}); err != nil {
		t.Fatal(err)
	}
	jobs, err := exports.New(st)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pool.Config().MaxConns != 8 {
		t.Fatal("unexpected runtime pool limit")
	}
	runtimeURL := "postgres://islands_app:local-runtime-only@postgres:5432/" + name + "?sslmode=disable"
	command("run", "-d", "--name", container, "--network", "go-islands-starter_default", "--cpus=0.5", "--memory=512m", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "-p", "127.0.0.1::8080", "-e", "DATABASE_URL="+runtimeURL, "-e", "APP_ORIGIN=https://capacity.example.test", "-e", "GOOGLE_CLIENT_ID=synthetic", "-e", "GOOGLE_CLIENT_SECRET=synthetic", "-e", "GOOGLE_WORKSPACE_DOMAINS=example.test", image)
	port := strings.TrimSpace(string(command("port", container, "8080")))
	base := "http://" + port
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	waitHealthy := func() {
		t.Helper()
		deadline := time.Now().Add(35 * time.Second)
		for time.Now().Before(deadline) {
			resp, e := client.Get(base + "/healthz")
			if e == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode == 200 {
					return
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("container failed to start: %s", command("logs", "--tail", "20", container))
	}
	waitHealthy()
	readContext, stopReaders := context.WithCancel(ctx)
	t.Cleanup(stopReaders)
	read := func(path string) (time.Duration, error) {
		start := time.Now()
		req, e := http.NewRequestWithContext(readContext, "GET", base+path, nil)
		if e != nil {
			return 0, e
		}
		req.AddCookie(&http.Cookie{Name: "__Host-islands", Value: token})
		resp, e := client.Do(req)
		if e != nil {
			return 0, e
		}
		defer func() { _ = resp.Body.Close() }()
		_, e = io.Copy(io.Discard, resp.Body)
		if e != nil {
			return 0, e
		}
		if resp.StatusCode != 200 {
			return 0, fmt.Errorf("status %d", resp.StatusCode)
		}
		return time.Since(start), nil
	}
	// Production must not expose the local preview's login endpoint.
	resp, err := client.PostForm(base+"/preview/login", url.Values{"account": {testdb.Admin}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("preview route present in production")
	}
	if _, err = read("/dashboard"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var durations []time.Duration
	failures := 0
	var workers, reading sync.WaitGroup
	reading.Add(10)
	finish := time.Now().Add(60 * time.Second)
	for i := range 10 {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			path := "/requests"
			if i%2 == 0 {
				path = "/dashboard"
			}
			first := true
			for time.Now().Before(finish) && readContext.Err() == nil {
				elapsed, e := read(path)
				mu.Lock()
				if e != nil {
					failures++
				} else {
					durations = append(durations, elapsed)
				}
				mu.Unlock()
				if first {
					reading.Done()
					first = false
				}
			}
		}(i)
	}
	t.Cleanup(func() { stopReaders(); workers.Wait() })
	reading.Wait()
	// Enqueue after all readers start, guaranteeing overlap with production workers.
	for range 20 {
		if _, err = jobs.Request(ctx, testdb.Alice, "capacity"); err != nil {
			t.Fatal(err)
		}
	}
	st.Pool.Close() // Exclude the fixture producer from app connection counts.
	var peakMiB float64
	maxListeners, maxTotal := 0, 0
	for time.Now().Before(finish) {
		raw := strings.TrimSpace(string(command("stats", "--no-stream", "--format", "{{.MemUsage}}", container)))
		part := strings.Fields(raw)[0]
		multiplier := 1.0
		if strings.HasSuffix(part, "GiB") {
			multiplier = 1024
		}
		value, e := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(part, "MiB"), "GiB"), 64)
		if e != nil {
			t.Fatal(e)
		}
		peakMiB = max(peakMiB, value*multiplier)
		var connections, listeners int
		if err = fixture.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE query LIKE 'LISTEN %') FROM pg_stat_activity WHERE datname=current_database() AND usename='islands_app'").Scan(&connections, &listeners); err != nil {
			t.Fatal(err)
		}
		maxListeners = max(maxListeners, listeners)
		maxTotal = max(maxTotal, connections)
	}
	workers.Wait()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	if len(durations) == 0 {
		t.Fatal("no completed reads")
	}
	p95 := durations[(len(durations)-1)*95/100]
	var completed int
	if err = fixture.QueryRow(ctx, "SELECT count(*) FROM app.exports WHERE state='complete'").Scan(&completed); err != nil {
		t.Fatal(err)
	}
	t.Logf("reads=%d failures=%d p95=%s peak_memory_mib=%.2f configured_pool_limit=8 observed_listeners=%d max_total_connections=%d exports_complete=%d/20 elapsed=60s", len(durations), failures, p95, peakMiB, maxListeners, maxTotal, completed)
	if failures != 0 || p95 >= time.Second || peakMiB >= 450 || maxListeners != 1 || maxTotal > 9 || completed != 20 {
		t.Fatal("capacity thresholds not met")
	}
	command("stop", "-t", "25", container)
	var state struct {
		ExitCode  int
		OOMKilled bool
	}
	if err = json.Unmarshal(command("inspect", "--format", "{{json .State}}", container), &state); err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 0 || state.OOMKilled {
		t.Fatalf("unclean shutdown: %+v", state)
	}
	command("start", container)
	base = "http://" + strings.TrimSpace(string(command("port", container, "8080")))
	waitHealthy()
	if _, err = read("/requests"); err != nil {
		t.Fatal("session or data lost on restart", err)
	}
	command("stop", "-t", "25", container)
	// Restore into a separate disposable database and compare retained records.
	backup := command("exec", "go-islands-starter-postgres-1", "pg_dump", "-U", "postgres", "-Fc", name)
	restored := name + "_restore"
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := fixture.Exec(cleanup, "DROP DATABASE IF EXISTS "+pgx.Identifier{restored}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	if _, err = fixture.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{restored}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	restoreCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	restore := exec.CommandContext(restoreCtx, "docker", "exec", "-i", "go-islands-starter-postgres-1", "pg_restore", "-U", "postgres", "-d", restored)
	restore.Stdin = strings.NewReader(string(backup))
	if out, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore: %v %s", err, out)
	}
	counts := strings.TrimSpace(string(command("exec", "go-islands-starter-postgres-1", "psql", "-U", "postgres", "-d", restored, "-Atc", "SELECT (SELECT count(*) FROM app.requests), (SELECT count(*) FROM app.exports WHERE state='complete'), (SELECT count(*) FROM app.audit_events), (SELECT count(*) FROM app.sessions)")))
	if counts != "1000|20|40|1" {
		t.Fatalf("restored counts=%s", counts)
	}
	t.Log("graceful shutdown, restart, session persistence, and restore counts passed")
}
