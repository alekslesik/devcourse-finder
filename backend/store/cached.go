package store

import (
	"context"
	"sync"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
)

// CachedDB serves immutable catalog snapshots. Every Load checks the committed
// import revision in PostgreSQL, so separate CLI imports invalidate all API
// processes and a database failure is not masked by cached data. Catalog writes
// must go through Import; direct SQL edits are not a publication mechanism.
type CachedDB struct {
	*DB
	mu       sync.RWMutex
	refresh  sync.Mutex
	revision int64
	loaded   bool
	courses  []catalog.Course
}

func NewCached(db *DB) *CachedDB { return &CachedDB{DB: db} }

// Load returns a read-only snapshot. Callers must not mutate its nested slices.
func (d *CachedDB) Load(ctx context.Context) ([]catalog.Course, error) {
	var revision int64
	if err := d.Pool.QueryRow(ctx, "SELECT COALESCE(max(id),0) FROM imports").Scan(&revision); err != nil {
		return nil, err
	}
	d.mu.RLock()
	courses, hit := d.courses, d.loaded && revision == d.revision
	d.mu.RUnlock()
	if hit {
		return courses, nil
	}
	// Serialize cold/revision-change loads; concurrent searches reuse one snapshot.
	d.refresh.Lock()
	defer d.refresh.Unlock()
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Read revision and catalog in the same snapshot, even if an import commits
	// between the initial revision check and this transaction.
	if err = tx.QueryRow(ctx, "SELECT COALESCE(max(id),0) FROM imports").Scan(&revision); err != nil {
		return nil, err
	}
	d.mu.RLock()
	courses, hit = d.courses, d.loaded && revision == d.revision
	d.mu.RUnlock()
	if !hit {
		courses, err = load(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	if !hit {
		d.mu.Lock()
		d.courses, d.revision, d.loaded = courses, revision, true
		d.mu.Unlock()
	}
	return courses, nil
}
