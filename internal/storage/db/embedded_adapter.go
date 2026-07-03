//   Фаза 2: Адаптеры}
//   Создаёт доменные use-cases через shared readTxBase из adapter_shared.go.
//   RunWrite защищён семафором cap=1 для последовательных записей.
//   Для Dolt-коммита используется shared runWritePipeline.}

//go:build cgo

package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	st "github.com/steveyegge/beads/internal/storage/store"
)

//   Используется в embedded режиме (без внешнего dolt sql-server).
type EmbeddedAdapter struct {
	dataDir  string
	database string
	branch   string
	writeSem chan struct{}
}

//   Сохраняет dataDir, database, branch для открытия SQL-соединений при каждом вызове.
func NewEmbeddedAdapter(dataDir, database, branch string) *EmbeddedAdapter {
	return &EmbeddedAdapter{
		dataDir:  dataDir,
		database: database,
		branch:   branch,
		writeSem: make(chan struct{}, 1),
	}
}

//   Транзакция всегда rollback'ится после завершения fn (read-only).
func (a *EmbeddedAdapter) RunRead(ctx context.Context, op string, fn func(context.Context, st.ReadTx) error) error {
	openDB, cleanup, err := embeddeddolt.OpenSQL(ctx, a.dataDir, a.database, a.branch)
	if err != nil {
		return fmt.Errorf("embedded RunRead(%s): %w", op, err)
	}
	defer func() { _ = cleanup() }()

	tx, err := openDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("embedded RunRead(%s): begin tx: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()

	return fn(ctx, &embeddedReadTx{readTxBase: readTxBase{tx: tx}})
}

//   writeSem гарантирует, что только одна запись выполняется одновременно.
//   При успешном fn вызывается runWritePipeline, иначе ROLLBACK.
func (a *EmbeddedAdapter) RunWrite(ctx context.Context, op string, fn func(context.Context, st.WriteTx) error) error {
	select {
	case a.writeSem <- struct{}{}:
		defer func() { <-a.writeSem }()
	case <-ctx.Done():
		return fmt.Errorf("embedded lock timeout: %w", ctx.Err())
	}

	openDB, cleanup, err := embeddeddolt.OpenSQL(ctx, a.dataDir, a.database, a.branch)
	if err != nil {
		return fmt.Errorf("embedded RunWrite(%s): %w", op, err)
	}
	defer func() { _ = cleanup() }()

	tx, err := openDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("embedded RunWrite(%s): begin tx: %w", op, err)
	}

	wt := &embeddedWriteTx{embeddedReadTx: embeddedReadTx{readTxBase: readTxBase{tx: tx}}}
	if fnErr := fn(ctx, wt); fnErr != nil {
		_ = tx.Rollback()
		return fnErr
	}

	return runWritePipeline(ctx, tx, op, "embedded", openDB)
}

// --- Внутренние типы ---

// embeddedReadTx реализует store.ReadTx через read-only SQL transaction.
// Use-cases создаются лениво через shared readTxBase.
type embeddedReadTx struct {
	readTxBase
}

// embeddedWriteTx реализует store.WriteTx через SQL транзакцию с Dolt-коммитом.
type embeddedWriteTx struct {
	embeddedReadTx
}

func (w *embeddedWriteTx) Commit(ctx context.Context, message string) error {
	// No-op: real commit happens in RunWrite via runWritePipeline
	return nil
}
func (w *embeddedWriteTx) Rollback(ctx context.Context) error {
	return w.tx.Rollback()
}
