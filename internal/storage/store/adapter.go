//   Фаза 1: Store — RunRead/RunWrite интерфейсы}
//   Очистка транзакции через defer Close() в RunRead/RunWrite}

package store

import (
	"context"
	"fmt"

	"github.com/steveyegge/beads/internal/storage/domain"
	"github.com/steveyegge/beads/internal/storage/uow"
)

//   Используется в proxied-server режиме.
type UoWAdapter struct {
	provider uow.UnitOfWorkProvider
}

func NewStore(provider uow.UnitOfWorkProvider) *UoWAdapter {
	return &UoWAdapter{provider: provider}
}

//   Транзакция закрывается через defer Close() после завершения fn.
func (a *UoWAdapter) RunRead(ctx context.Context, op string, fn func(context.Context, ReadTx) error) error {
	u, err := a.provider.NewUOW(ctx)
	if err != nil {
		return fmt.Errorf("RunRead(%s): %w", op, err)
	}
	defer u.Close(ctx)
	return fn(ctx, &readTx{uow: u})
}

//   При успешном fn вызывается Commit, иначе Close (rollback) через defer.
func (a *UoWAdapter) RunWrite(ctx context.Context, op string, fn func(context.Context, WriteTx) error) error {
	u, err := a.provider.NewUOW(ctx)
	if err != nil {
		return fmt.Errorf("RunWrite(%s): %w", op, err)
	}
	defer u.Close(ctx)

	stx := &writeTx{uow: u}
	if err := fn(ctx, stx); err != nil {
		return err
	}
	return u.Commit(ctx, op)
}

type readTx struct {
	uow uow.UnitOfWork
}

func (tx *readTx) Issues() domain.IssueUseCase           { return tx.uow.IssueUseCase() }
func (tx *readTx) Dependencies() domain.DependencyUseCase { return tx.uow.DependencyUseCase() }
func (tx *readTx) Labels() domain.LabelUseCase             { return tx.uow.LabelUseCase() }
func (tx *readTx) Config() domain.ConfigUseCase             { return tx.uow.ConfigUseCase() }

type writeTx struct {
	uow uow.UnitOfWork
}

func (tx *writeTx) Issues() domain.IssueUseCase           { return tx.uow.IssueUseCase() }
func (tx *writeTx) Dependencies() domain.DependencyUseCase { return tx.uow.DependencyUseCase() }
func (tx *writeTx) Labels() domain.LabelUseCase             { return tx.uow.LabelUseCase() }
func (tx *writeTx) Config() domain.ConfigUseCase             { return tx.uow.ConfigUseCase() }

func (tx *writeTx) Commit(ctx context.Context, message string) error {
	return tx.uow.Commit(ctx, message)
}

func (tx *writeTx) Rollback(ctx context.Context) error {
	tx.uow.Close(ctx)
	return nil
}
