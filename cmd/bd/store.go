//   Фаза 1: Store — RunRead/RunWrite интерфейсы}
//   Импорт store пакета с алиасом st, чтобы избежать конфликта с var store storage.DoltStorage}

package main

import (
	"context"

	st "github.com/steveyegge/beads/internal/storage/store"
	"github.com/steveyegge/beads/internal/storage/uow"
)

//   В proxied-server режиме возвращает UoWAdapter, иначе nil.
func NewStoreFromUOW(ctx context.Context, provider uow.UnitOfWorkProvider) *st.UoWAdapter {
	if proxiedServerMode {
		return st.NewStore(provider)
	}
	// legacy mode — возвращаем nil (используется DoltStorage)
	return nil
}
