package postgrestest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

const (
	defaultImage = "postgres:16-alpine"
	defaultUser  = "harness"
	defaultPass  = "harness"
	defaultDB    = "harness_test"
)

type Instance struct {
	DSN           string
	containerName string
}

func Start(t testing.TB) *Instance {
	t.Helper()

	if dsn := firstEnv("HARNESS_POSTGRES_TEST_DSN", "HARNESS_POSTGRES_DSN"); dsn != "" {
		inst := &Instance{DSN: dsn}
		inst.prepareDatabase(t)
		return inst
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Postgres integration tests unless HARNESS_POSTGRES_TEST_DSN is set")
	}

	containerName := "harness-pg-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cmd := exec.Command(
		"docker", "run", "--rm", "-d",
		"-e", "POSTGRES_USER="+defaultUser,
		"-e", "POSTGRES_PASSWORD="+defaultPass,
		"-e", "POSTGRES_DB="+defaultDB,
		"-p", "127.0.0.1::5432",
		"--name", containerName,
		defaultImage,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start postgres container: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		stop := exec.Command("docker", "stop", "-t", "1", containerName)
		_, _ = stop.CombinedOutput()
	})

	hostPort := waitForDockerPort(t, containerName)
	inst := &Instance{
		DSN:           fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", defaultUser, defaultPass, hostPort, defaultDB),
		containerName: containerName,
	}
	inst.prepareDatabase(t)
	return inst
}

func (i *Instance) OpenService(t testing.TB, opts hruntime.Options) (*hruntime.Service, *sql.DB) {
	t.Helper()
	rt, db, err := hpostgres.OpenService(context.Background(), i.DSN, opts)
	if err != nil {
		t.Fatalf("open postgres runtime service: %v", err)
	}
	return rt, db
}

func (i *Instance) prepareDatabase(t testing.TB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var db *sql.DB
	var err error
	deadline := time.Now().Add(30 * time.Second)
	for {
		db, err = hpostgres.OpenDB(ctx, i.DSN)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for postgres readiness: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	defer db.Close()

	if err := hpostgres.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply postgres migrations: %v", err)
	}
	if err := resetDatabase(ctx, db); err != nil {
		t.Fatalf("reset postgres database: %v", err)
	}
}

func resetDatabase(ctx context.Context, db *sql.DB) error {
	statements := []string{
		"DELETE FROM runtime_handles",
		"DELETE FROM artifacts",
		"DELETE FROM verification_records",
		"DELETE FROM action_records",
		"DELETE FROM attempts",
		"DELETE FROM planning_records",
		"DELETE FROM context_summaries",
		"DELETE FROM capability_snapshots",
		"DELETE FROM approvals",
		"DELETE FROM plan_steps",
		"DELETE FROM plans",
		"DELETE FROM tasks",
		"DELETE FROM sessions",
		"DELETE FROM audit_events",
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func waitForDockerPort(t testing.TB, containerName string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := exec.Command("docker", "port", containerName, "5432/tcp").CombinedOutput()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "[::]") {
					continue
				}
				line = strings.TrimPrefix(line, "0.0.0.0:")
				if strings.Count(line, ":") == 0 {
					continue
				}
				if !strings.Contains(line, ":") {
					continue
				}
				if !strings.HasPrefix(line, "127.0.0.1:") {
					line = "127.0.0.1:" + line[strings.LastIndex(line, ":")+1:]
				}
				return line
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for docker port mapping for %s: %v %s", containerName, err, strings.TrimSpace(string(out)))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
