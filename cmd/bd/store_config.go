//   Фаза 2: Адаптеры}
//   соответствующую реализацию store.Store. Только для cgo-сборок.}

//go:build cgo

package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/db"
	"github.com/steveyegge/beads/internal/storage/dolt"
	st "github.com/steveyegge/beads/internal/storage/store"
)

//   В proxied-server режиме возвращает UoWAdapter через uowProvider.
//   В embedded режиме возвращает EmbeddedAdapter.
//   В server режиме возвращает ServerAdapter через dolt.DoltStore.DB().
func NewStoreFromConfig(ctx context.Context, beadsDir string) st.Store {
	if usesProxiedServer() {
		if uowProvider != nil {
			return NewStoreFromUOW(ctx, uowProvider)
		}
		return nil
	}

	if usesSQLServer() {
		// Server mode: создаём DoltStore через cf и извлекаем *sql.DB
		cfg, err := configfile.Load(beadsDir)
		if err != nil || cfg == nil {
			return nil
		}
		ds, err := dolt.NewFromConfig(ctx, beadsDir)
		if err != nil {
			return nil
		}
		return db.NewServerAdapter(ds.DB())
	}

	// Embedded mode
	cfg, err := configfile.Load(beadsDir)
	database := configfile.DefaultDoltDatabase
	if err == nil && cfg != nil {
		database = cfg.GetDoltDatabase()
	}
	if sanitized := sanitizeDBName(database); sanitized != database {
		database = sanitized
	}
	dataDir := filepath.Join(beadsDir, "embeddeddolt")

	// Validate that the embedded dolt directory exists
	// (NewEmbeddedAdapter will use OpenSQL which creates it)

	_ = fmt.Sprintf("NewStoreFromConfig: embedded mode dataDir=%s db=%s", dataDir, database)

	return db.NewEmbeddedAdapter(dataDir, database, "main")
}
