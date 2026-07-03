//   Фаза 2: Адаптеры}
//   Embedded режим недоступен без CGO.}

//go:build !cgo

package main

import (
	"context"

	st "github.com/steveyegge/beads/internal/storage/store"
)

//   В non-cgo сборке поддерживает только proxied-server режим.
//   Server режим возвращает nil (DoltStore создаётся отдельно).
//   Embedded режим недоступен.
func NewStoreFromConfig(ctx context.Context, beadsDir string) st.Store {
	if usesProxiedServer() {
		if uowProvider != nil {
			return NewStoreFromUOW(ctx, uowProvider)
		}
		return nil
	}

	// Server mode: DoltStore создаётся отдельно, ServerAdapter не используется
	// Embedded mode: недоступен без CGO
	return nil
}
