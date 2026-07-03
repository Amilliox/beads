package store

import (
	"context"
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/domain"
	"github.com/steveyegge/beads/internal/storage/uow"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubUOW implements uow.UnitOfWork for testing. Only Commit and Close are used;
// all use-case accessors return nil stubs (the tests check they can be called
// without panicking, not that they return meaningful data).
type stubUOW struct {
	commitErr    error
	closed       bool
	issueUC      domain.IssueUseCase
	dependencyUC domain.DependencyUseCase
	labelUC      domain.LabelUseCase
	configUC     domain.ConfigUseCase
}

func (u *stubUOW) Close(_ context.Context)                     { u.closed = true }
func (u *stubUOW) Commit(_ context.Context, _ string) error    { return u.commitErr }
func (u *stubUOW) ConfigUseCase() domain.ConfigUseCase         { return u.configUC }
func (u *stubUOW) DoltRemoteUseCase() domain.DoltRemoteUseCase { panic("not implemented") }
func (u *stubUOW) BootstrapUseCase() domain.BootstrapUseCase   { panic("not implemented") }
func (u *stubUOW) IssueUseCase() domain.IssueUseCase           { return u.issueUC }
func (u *stubUOW) DependencyUseCase() domain.DependencyUseCase { return u.dependencyUC }
func (u *stubUOW) LabelUseCase() domain.LabelUseCase           { return u.labelUC }
func (u *stubUOW) CommentUseCase() domain.CommentUseCase       { panic("not implemented") }

// stubProvider lets tests inject NewUOW errors.
type stubProvider struct {
	newUOWErr error
	uow       uow.UnitOfWork
}

func (p *stubProvider) Close(_ context.Context) error { return nil }
func (p *stubProvider) NewUOW(_ context.Context) (uow.UnitOfWork, error) {
	if p.newUOWErr != nil {
		return nil, p.newUOWErr
	}
	return p.uow, nil
}

// stubIssueUC implements domain.IssueUseCase for use-case accessor verification.
type stubIssueUC struct{ domain.IssueUseCase }

// stubDepUC implements domain.DependencyUseCase for use-case accessor verification.
type stubDepUC struct{ domain.DependencyUseCase }

// stubLabelUC implements domain.LabelUseCase for use-case accessor verification.
type stubLabelUC struct{ domain.LabelUseCase }

// stubConfigUC implements domain.ConfigUseCase for use-case accessor verification.
type stubConfigUC struct{ domain.ConfigUseCase }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestStore_RunRead_ReadOnly(t *testing.T) {
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunRead(ctx, "test-read", func(ctx context.Context, tx ReadTx) error {
		_ = tx.Issues()
		_ = tx.Dependencies()
		_ = tx.Labels()
		_ = tx.Config()
		return nil
	})
	if err != nil {
		t.Fatalf("RunRead returned unexpected error: %v", err)
	}
	if !su.closed {
		t.Error("RunRead did not close the UoW")
	}
}

func TestStore_RunRead_NewUOWError(t *testing.T) {
	expectedErr := errors.New("connection failed")
	store := NewStore(&stubProvider{newUOWErr: expectedErr})

	ctx := context.Background()
	err := store.RunRead(ctx, "test-read", func(ctx context.Context, tx ReadTx) error {
		t.Error("fn should not be called when NewUOW fails")
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStore_RunWrite_Commit(t *testing.T) {
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunWrite(ctx, "test-write", func(ctx context.Context, tx WriteTx) error {
		_ = tx.Issues()
		_ = tx.Dependencies()
		_ = tx.Labels()
		_ = tx.Config()
		return nil
	})
	if err != nil {
		t.Fatalf("RunWrite returned unexpected error: %v", err)
	}
	if !su.closed {
		t.Error("RunWrite did not close the UoW")
	}
}

func TestStore_RunWrite_CommitError(t *testing.T) {
	commitErr := errors.New("commit failed")
	su := &stubUOW{commitErr: commitErr}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunWrite(ctx, "test-write", func(ctx context.Context, tx WriteTx) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected commit error, got nil")
	}
	if !su.closed {
		t.Error("RunWrite did not close the UoW after commit failure")
	}
}

func TestStore_RunWrite_FnError(t *testing.T) {
	fnErr := errors.New("fn failed")
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunWrite(ctx, "test-write", func(ctx context.Context, tx WriteTx) error {
		return fnErr
	})
	if err == nil {
		t.Fatal("expected fn error, got nil")
	}
	if !su.closed {
		t.Error("RunWrite did not close the UoW after fn error")
	}
}

func TestStore_RunWrite_CommitAndClose(t *testing.T) {
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunWrite(ctx, "test-commit", func(ctx context.Context, tx WriteTx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunWrite returned unexpected error: %v", err)
	}
	if !su.closed {
		t.Error("RunWrite did not close the UoW after successful commit")
	}
}

func TestStore_RunWrite_ErrStateDiverged(t *testing.T) {
	su := &stubUOW{commitErr: storage.ErrStateDiverged}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunWrite(ctx, "test-diverged", func(ctx context.Context, tx WriteTx) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected ErrStateDiverged, got nil")
	}
	if !errors.Is(err, storage.ErrStateDiverged) {
		t.Fatalf("expected ErrStateDiverged, got: %v", err)
	}
}

func TestStore_RunRead_FnError(t *testing.T) {
	fnErr := errors.New("read operation failed")
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunRead(ctx, "test-read", func(ctx context.Context, tx ReadTx) error {
		return fnErr
	})
	if err == nil {
		t.Fatal("expected fn error, got nil")
	}
	if !errors.Is(err, fnErr) {
		t.Fatalf("expected fnErr, got: %v", err)
	}
	if !su.closed {
		t.Error("RunRead did not close the UoW after fn error")
	}
}

func TestStore_RunWrite_ExplicitCommit(t *testing.T) {
	su := &stubUOW{}
	tx := &writeTx{uow: su}

	ctx := context.Background()
	err := tx.Commit(ctx, "explicit-commit")
	if err != nil {
		t.Fatalf("writeTx.Commit failed: %v", err)
	}

	err = tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("writeTx.Rollback failed: %v", err)
	}
	if !su.closed {
		t.Error("Rollback did not close the UoW")
	}
}

func TestUoWAdapter_Lifecycle(t *testing.T) {
	su := &stubUOW{}
	provider := &stubProvider{uow: su}
	adapter := NewStore(provider)

	if adapter == nil {
		t.Fatal("NewStore returned nil")
	}

	ctx := context.Background()

	err := adapter.RunRead(ctx, "lifecycle-read", func(ctx context.Context, tx ReadTx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunRead failed: %v", err)
	}

	err = adapter.RunWrite(ctx, "lifecycle-write", func(ctx context.Context, tx WriteTx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunWrite failed: %v", err)
	}
}

func TestUoWAdapter_NewStoreNilProvider(t *testing.T) {
	adapter := NewStore(nil)
	if adapter == nil {
		t.Fatal("NewStore(nil) returned nil")
	}
}

func TestWriteTx_ImplementsReadTx(t *testing.T) {
	var _ ReadTx = (*writeTx)(nil)
}

// ---------------------------------------------------------------------------
// Adversarial / edge-case tests
// ---------------------------------------------------------------------------

func TestStore_DoubleCloseSafe(t *testing.T) {
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunWrite(ctx, "double-close", func(ctx context.Context, tx WriteTx) error {
		_ = tx.Rollback(ctx)
		return nil
	})
	if err != nil {
		t.Logf("RunWrite returned error (expected if Commit after Rollback fails): %v", err)
	}
	if !su.closed {
		t.Error("UoW should have been closed at least once")
	}
}

func TestStore_RunRead_PropagatesContext(t *testing.T) {
	su := &stubUOW{}
	store := NewStore(&stubProvider{uow: su})

	ctx := context.Background()
	err := store.RunRead(ctx, "context-test", func(gotCtx context.Context, tx ReadTx) error {
		if gotCtx != ctx {
			t.Error("fn received a different context than the one passed to RunRead")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunRead returned unexpected error: %v", err)
	}
}

func TestStore_RunWrite_RollbackIdempotent(t *testing.T) {
	su := &stubUOW{}
	stx := &writeTx{uow: su}

	ctx := context.Background()
	err1 := stx.Rollback(ctx)
	err2 := stx.Rollback(ctx)

	if err1 != nil {
		t.Fatalf("first Rollback failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second Rollback failed: %v", err2)
	}
	if !su.closed {
		t.Error("UoW should be closed after Rollback")
	}
}

func TestReadTx_AllUseCasesAccessible(t *testing.T) {
	issueUC := &stubIssueUC{}
	depUC := &stubDepUC{}
	labelUC := &stubLabelUC{}
	configUC := &stubConfigUC{}

	su := &stubUOW{
		issueUC:      issueUC,
		dependencyUC: depUC,
		labelUC:      labelUC,
		configUC:     configUC,
	}
	tx := &readTx{uow: su}

	gotIssues := tx.Issues()
	gotDeps := tx.Dependencies()
	gotLabels := tx.Labels()
	gotConfig := tx.Config()

	if gotIssues != issueUC {
		t.Error("Issues() returned unexpected value")
	}
	if gotDeps != depUC {
		t.Error("Dependencies() returned unexpected value")
	}
	if gotLabels != labelUC {
		t.Error("Labels() returned unexpected value")
	}
	if gotConfig != configUC {
		t.Error("Config() returned unexpected value")
	}
}
