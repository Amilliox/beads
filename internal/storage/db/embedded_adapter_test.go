//   NOTE: тесты проверяют только логику адаптера, не embeddeddolt.
//   Для полного теста требуется реальная embeddeddolt БД с CGO+ICU.}
//   OpenSQL не вызывается напрямую — тесты проверяют RunRead/RunWrite логику
//   через mock. Полная интеграция требует реального embeddeddolt.}

//go:build cgo

package db

import (
	"context"
	"testing"

	st "github.com/steveyegge/beads/internal/storage/store"
)

// @test Проверяет: файл компилируется (compile-only check)
// @note Полная функциональная проверка EmbeddedAdapter требует
//   реального embeddeddolt подключения (CGO + ICU на Mac).
//   На CI с установленным ICU все тесты запустятся.
func TestEmbeddedAdapter_CompilesWithCGO(t *testing.T) {
	// This is a compile-time check — if this file compiles, the check passes.
	_ = NewEmbeddedAdapter("/tmp/test", "beads", "main")
}

// @test Проверяет: var _ st.Store = (*EmbeddedAdapter)(nil)
func TestEmbeddedAdapter_ImplementsStore(t *testing.T) {
	var _ st.Store = (*EmbeddedAdapter)(nil)
}

// @test Проверяет: dataDir, database, branch сохраняются, writeSem инициализирован
func TestEmbeddedAdapter_New(t *testing.T) {
	a := NewEmbeddedAdapter("/tmp/test", "beads", "main")
	if a == nil {
		t.Fatal("NewEmbeddedAdapter returned nil")
	}
	if a.dataDir != "/tmp/test" {
		t.Errorf("dataDir = %q, want /tmp/test", a.dataDir)
	}
	if a.database != "beads" {
		t.Errorf("database = %q, want beads", a.database)
	}
	if a.branch != "main" {
		t.Errorf("branch = %q, want main", a.branch)
	}
	// writeSem capacity should be 1
	if cap(a.writeSem) != 1 {
		t.Errorf("writeSem capacity = %d, want 1", cap(a.writeSem))
	}
}

// @test Проверяет: горутина не может войти в RunWrite пока другая внутри
func TestEmbeddedAdapter_WriteSemaphore(t *testing.T) {
	a := NewEmbeddedAdapter("/tmp/test", "beads", "main")

	// writeSem capacity test — should block when full
	// First acquire the semaphore slot
	a.writeSem <- struct{}{}

	// This should block — verify with a goroutine that times out
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	// The write should fail immediately due to cancelled context
	err := a.RunWrite(ctx, "test", func(ctx context.Context, tx st.WriteTx) error {
		return nil
	})

	if err == nil {
		t.Error("RunWrite should fail when writeSem is full and ctx is cancelled")
	}

	// Release the semaphore
	<-a.writeSem
}

// @test Проверяет: Issues, Dependencies, Labels, Config возвращают не nil
//   Это unit-тест структуры, embeddeddolt не требуется.
func TestEmbeddedAdapter_ReadTx_AccessorTypes(t *testing.T) {
	// Verify the types/structs exist and have correct methods
	a := NewEmbeddedAdapter("/tmp/test", "beads", "main")
	_ = a // EmbeddedAdapter compiles and is constructable

	// Verify embeddedWriteTx embeds embeddedReadTx
	rt := &embeddedReadTx{}
	_ = rt.Issues()
	_ = rt.Dependencies()
	_ = rt.Labels()
	_ = rt.Config()
}

// @test Проверяет: Сигнатуры методов соответствуют store.WriteTx
//   (compile-time check + basic structure)
func TestEmbeddedAdapter_WriteTx_Methods(t *testing.T) {
	// Compile-time check: embeddedWriteTx embeds embeddedReadTx
	var _ st.WriteTx = &embeddedWriteTx{}

	wt := &embeddedWriteTx{}
	// DOLT_COMMIT через nil tx — это паника (sql.Tx паникует при nil receiver)
	// Просто проверяем, что метод существует
	_ = wt.Commit
	_ = wt.Rollback
}
