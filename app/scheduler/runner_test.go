//go:build windows || darwin

package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ollama/ollama/app/store"
)

func setupTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "scheduler-test-*")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "test.db")
	s := &store.Store{DBPath: dbPath}

	cleanup := func() {
		_ = s.Close()
		_ = os.RemoveAll(dir)
	}

	return s, cleanup
}

func TestRunnerCreation(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	runner := NewRunner(s, nil)
	if runner == nil {
		t.Fatal("expected runner to be created")
	}

	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	runner.Stop()
}
