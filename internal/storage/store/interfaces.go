//   Фаза 1: Store — RunRead/RunWrite интерфейсы}
//   Sub-package store чтобы избежать import cycle: storage -> domain -> storage}

package store

import (
	"context"

	"github.com/steveyegge/beads/internal/storage/domain"
)

//   RunRead создаёт read-only транзакцию, fn получает ReadTx.
//   RunWrite создаёт транзакцию с коммитом, fn получает WriteTx.
type Store interface {
	RunRead(ctx context.Context, op string, fn func(context.Context, ReadTx) error) error
	RunWrite(ctx context.Context, op string, fn func(context.Context, WriteTx) error) error
}

type ReadTx interface {
	Issues() domain.IssueUseCase
	Dependencies() domain.DependencyUseCase
	Labels() domain.LabelUseCase
	Config() domain.ConfigUseCase
}

type WriteTx interface {
	ReadTx
	Commit(ctx context.Context, message string) error
	Rollback(ctx context.Context) error
}
