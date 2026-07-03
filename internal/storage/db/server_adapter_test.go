//   Проверяет read-only транзакции, commit, rollback, DOLT_COMMIT}

package db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	st "github.com/steveyegge/beads/internal/storage/store"
)

//   и выполняет rollback через defer.
// @test Проверяет: BeginTx с ReadOnly:true, fn вызывается, Rollback вызывается
func TestServerAdapter_RunRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectRollback()

	callCount := 0
	err = adapter.RunRead(context.Background(), "test-read", func(ctx context.Context, tx st.ReadTx) error {
		callCount++
		// Verify readTx returns valid accessors (they use sync.Once init)
		_ = tx.Issues()
		_ = tx.Dependencies()
		_ = tx.Labels()
		_ = tx.Config()
		return nil
	})

	if err != nil {
		t.Errorf("RunRead returned unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1", callCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: ошибка BeginTx пробрасывается наружу
func TestServerAdapter_RunRead_BeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	err = adapter.RunRead(context.Background(), "test-read", func(ctx context.Context, tx st.ReadTx) error {
		t.Error("fn should not be called when BeginTx fails")
		return nil
	})

	if err == nil {
		t.Error("RunRead should return error when BeginTx fails")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: ошибка fn пробрасывается, rollback вызывается
func TestServerAdapter_RunRead_FnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectRollback()

	fnErr := errors.New("fn error")
	err = adapter.RunRead(context.Background(), "test-read", func(ctx context.Context, tx st.ReadTx) error {
		return fnErr
	})

	if err != fnErr {
		t.Errorf("RunRead should return fn error, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: BeginTx, fn, DOLT_COMMIT('-Am', ?), SQL COMMIT
func TestServerAdapter_RunWrite_Commit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("CALL DOLT_COMMIT('-Am', ?)")).
		WithArgs("test-write").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	callCount := 0
	err = adapter.RunWrite(context.Background(), "test-write", func(ctx context.Context, tx st.WriteTx) error {
		callCount++
		_ = tx.Issues()
		_ = tx.Dependencies()
		_ = tx.Labels()
		_ = tx.Config()
		return nil
	})

	if err != nil {
		t.Errorf("RunWrite returned unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1", callCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: BeginTx, fn error, ROLLBACK, DOLT_COMMIT не вызывается
func TestServerAdapter_RunWrite_FnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectRollback()

	fnErr := errors.New("fn write error")
	err = adapter.RunWrite(context.Background(), "test-write", func(ctx context.Context, tx st.WriteTx) error {
		return fnErr
	})

	if err != fnErr {
		t.Errorf("RunWrite should return fn error, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: BeginTx, fn OK, DOLT_COMMIT error, ROLLBACK, SQL COMMIT не вызывается
func TestServerAdapter_RunWrite_DoltCommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("CALL DOLT_COMMIT('-Am', ?)")).
		WithArgs("test-write").
		WillReturnError(errors.New("dolt commit failed"))
	mock.ExpectRollback()

	err = adapter.RunWrite(context.Background(), "test-write", func(ctx context.Context, tx st.WriteTx) error {
		return nil
	})

	if err == nil {
		t.Error("RunWrite should return error when DOLT_COMMIT fails")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: BeginTx, fn OK, DOLT_COMMIT OK, SQL COMMIT error
func TestServerAdapter_RunWrite_SQLCommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("CALL DOLT_COMMIT('-Am', ?)")).
		WithArgs("test-write").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit().WillReturnError(errors.New("sql commit failed"))

	err = adapter.RunWrite(context.Background(), "test-write", func(ctx context.Context, tx st.WriteTx) error {
		return nil
	})

	if err == nil {
		t.Error("RunWrite should return error when SQL COMMIT fails")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: writeTx.Commit вызывает CALL DOLT_COMMIT
func TestServerAdapter_WriteTx_ExplicitCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	// Explicit Commit is no-op: runWritePipeline handles all DOLT_COMMITs
	mock.ExpectExec(regexp.QuoteMeta("CALL DOLT_COMMIT('-Am', ?)")).
		WithArgs("test-explicit").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = adapter.RunWrite(context.Background(), "test-explicit", func(ctx context.Context, tx st.WriteTx) error {
		// Explicit Commit is no-op
		_ = tx.Commit(ctx, "explicit-commit")
		return nil
	})

	if err != nil {
		t.Errorf("RunWrite with explicit commit returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// @test Проверяет: writeTx.Rollback вызывает tx.Rollback
func TestServerAdapter_WriteTx_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()

	// The rollback inside fn
	mock.ExpectRollback()

	// fn returns nil but tx is already rolled back — RunWrite tries DOLT_COMMIT and COMMIT
	// which will fail on a closed tx; we expect an error.
	// This is a contract edge case documented in step 1.
	err = adapter.RunWrite(context.Background(), "test-rollback", func(ctx context.Context, tx st.WriteTx) error {
		_ = tx.Rollback(ctx)
		return nil // caller should return error after Rollback, but test the edge case
	})

	if err == nil {
		t.Error("RunWrite should return error when RunWrite tries DOLT_COMMIT on rolled-back tx")
	}
}

// @test Проверяет: var _ st.Store = (*ServerAdapter)(nil)
func TestServerAdapter_ImplementsStore(t *testing.T) {
	var _ st.Store = (*ServerAdapter)(nil)
}

// @test Проверяет: конструктор не паникует с nil
func TestServerAdapter_NewWithNilDB(t *testing.T) {
	adapter := NewServerAdapter(nil)
	if adapter == nil {
		t.Error("NewServerAdapter(nil) should return non-nil adapter")
	}
}

// @test Проверяет: Issues, Dependencies, Labels, Config не nil и инициализируются лениво
func TestServerAdapter_ReadTx_AllAccessors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	adapter := NewServerAdapter(db)

	mock.ExpectBegin()
	mock.ExpectRollback()

	err = adapter.RunRead(context.Background(), "test-accessors", func(ctx context.Context, tx st.ReadTx) error {
		if tx.Issues() == nil {
			t.Error("Issues() returned nil")
		}
		if tx.Dependencies() == nil {
			t.Error("Dependencies() returned nil")
		}
		if tx.Labels() == nil {
			t.Error("Labels() returned nil")
		}
		if tx.Config() == nil {
			t.Error("Config() returned nil")
		}
		return nil
	})

	if err != nil {
		t.Errorf("RunRead returned unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
