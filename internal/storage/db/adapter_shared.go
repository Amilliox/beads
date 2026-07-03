//   Оба адаптера (embedded и server) встраивают readTxBase.
//   runWritePipeline выносит общий DOLT_COMMIT → COMMIT → verify.}

package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/steveyegge/beads/internal/storage/domain"
	"github.com/steveyegge/beads/internal/storage/domain/db"
)

// readTxBase реализует ленивую инициализацию use-cases.
// Встраивается в embeddedReadTx и serverReadTx.
type readTxBase struct {
	tx       *sql.Tx
	issueUC  domain.IssueUseCase
	depUC    domain.DependencyUseCase
	labelUC  domain.LabelUseCase
	configUC domain.ConfigUseCase
	initOnce sync.Once
}

// init создаёт domain use-cases для текущей транзакции один раз.
func (r *readTxBase) init() {
	r.initOnce.Do(func() {
		runner := db.Runner(r.tx)
		depRepo := db.NewDependencySQLRepository(runner)
		labelRepo := db.NewLabelSQLRepository(runner)
		r.depUC = domain.NewDependencyUseCase(depRepo)
		r.labelUC = domain.NewLabelUseCase(labelRepo)
		r.configUC = domain.NewConfigUseCase(db.NewConfigSQLRepository(runner))
		r.issueUC = domain.NewIssueUseCase(
			db.NewIssueSQLRepository(runner), depRepo, labelRepo,
			db.NewChildCounterSQLRepository(runner),
			db.NewCommentSQLRepository(runner),
			db.NewConfigSQLRepository(runner),
			db.NewEventsSQLRepository(runner),
			r.labelUC, r.depUC,
		)
	})
}

func (r *readTxBase) Issues() domain.IssueUseCase           { r.init(); return r.issueUC }
func (r *readTxBase) Dependencies() domain.DependencyUseCase { r.init(); return r.depUC }
func (r *readTxBase) Labels() domain.LabelUseCase             { r.init(); return r.labelUC }
func (r *readTxBase) Config() domain.ConfigUseCase             { r.init(); return r.configUC }

// runWritePipeline выполняет общий pipeline: DOLT_COMMIT → SQL COMMIT → verifyPostCommit.
// prefix используется для форматирования сообщений об ошибках ("embedded" или "server").
func runWritePipeline(ctx context.Context, tx *sql.Tx, op, prefix string, dbForVerify *sql.DB) error {
	if _, err := tx.ExecContext(ctx, "CALL DOLT_COMMIT('-Am', ?)", op); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("%s RunWrite(%s): dolt commit: %w", prefix, op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s RunWrite(%s): commit: %w", prefix, op, err)
	}
	if err := verifyPostCommit(ctx, dbForVerify); err != nil {
		return fmt.Errorf("%s RunWrite(%s): post-commit verify: %w", prefix, op, err)
	}
	return nil
}
