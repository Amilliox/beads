//   Фаза 2: Адаптеры}
//   Создаёт доменные use-cases через shared readTxBase из adapter_shared.go.
//   Dolt SQL server сам управляет concurrency, поэтому семафор не требуется.
//   DOLT_COMMIT вызывается через shared runWritePipeline.}

package db

import (
	"context"
	"database/sql"
	"fmt"

	st "github.com/steveyegge/beads/internal/storage/store"
)

//   Используется в server режиме (подключение к внешнему dolt sql-server).
type ServerAdapter struct {
	db *sql.DB
}

func NewServerAdapter(db *sql.DB) *ServerAdapter {
	return &ServerAdapter{db: db}
}

//   Транзакция всегда rollback'ится после завершения fn (read-only).
func (a *ServerAdapter) RunRead(ctx context.Context, op string, fn func(context.Context, st.ReadTx) error) error {
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("server RunRead(%s): begin tx: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()

	return fn(ctx, &serverReadTx{readTxBase: readTxBase{tx: tx}})
}

//   При успешном fn вызывается shared runWritePipeline (DOLT_COMMIT + SQL COMMIT + verify).
//   При ошибке fn вызывается ROLLBACK.
func (a *ServerAdapter) RunWrite(ctx context.Context, op string, fn func(context.Context, st.WriteTx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("server RunWrite(%s): begin tx: %w", op, err)
	}

	if fnErr := fn(ctx, &serverWriteTx{serverReadTx: serverReadTx{readTxBase: readTxBase{tx: tx}}}); fnErr != nil {
		_ = tx.Rollback()
		return fnErr
	}

	return runWritePipeline(ctx, tx, op, "server", a.db)
}

// --- Внутренние типы ---

// serverReadTx реализует store.ReadTx через read-only SQL транзакцию.
// Use-cases создаются лениво через shared readTxBase.
type serverReadTx struct {
	readTxBase
}

// serverWriteTx реализует store.WriteTx через SQL транзакцию с Dolt-коммитом.
type serverWriteTx struct {
	serverReadTx
}

func (w *serverWriteTx) Commit(ctx context.Context, message string) error {
	// No-op: real commit happens in RunWrite via runWritePipeline
	return nil
}
func (w *serverWriteTx) Rollback(ctx context.Context) error {
	return w.tx.Rollback()
}
