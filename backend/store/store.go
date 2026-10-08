package store

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"devcourse-finder/catalog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var Schema string

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*DB, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	return &DB{p}, nil
}
func (d *DB) Ping(ctx context.Context) error    { return d.Pool.Ping(ctx) }
func (d *DB) Migrate(ctx context.Context) error { _, e := d.Pool.Exec(ctx, Schema); return e }

type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func load(ctx context.Context, q Querier) ([]catalog.Course, error) {
	rows, e := q.Query(ctx, "SELECT id,slug,title,provider,language,direction,summary,audience,goals,topics,source,checked_at,status,demo FROM courses ORDER BY id")
	if e != nil {
		return nil, e
	}
	cs := []catalog.Course{}
	idx := map[string]int{}
	for rows.Next() {
		var c catalog.Course
		e = rows.Scan(&c.ID, &c.Slug, &c.Title, &c.Provider, &c.Language, &c.Direction, &c.Summary, &c.Audience, &c.Goals, &c.Topics, &c.Source, &c.CheckedAt, &c.Status, &c.Demo)
		if e != nil {
			rows.Close()
			return nil, e
		}
		c.Offers = []catalog.Offer{}
		idx[c.ID] = len(cs)
		cs = append(cs, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	rows, e = q.Query(ctx, "SELECT id,course_id,name,price,price_kind,free,price_checked_at,valid_until,hours,weeks,review,mentor,schedule,enrollment,url FROM offers ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var o catalog.Offer
		var cid string
		if e = rows.Scan(&o.ID, &cid, &o.Name, &o.Price, &o.PriceKind, &o.Free, &o.PriceCheckedAt, &o.ValidUntil, &o.Hours, &o.Weeks, &o.Review, &o.Mentor, &o.Schedule, &o.Enrollment, &o.URL); e != nil {
			return nil, e
		}
		if i, ok := idx[cid]; ok {
			cs[i].Offers = append(cs[i].Offers, o)
		}
	}
	return cs, rows.Err()
}
func (d *DB) Load(ctx context.Context) ([]catalog.Course, error) {
	tx, e := d.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	v, e := load(ctx, tx)
	if e != nil {
		return nil, e
	}
	return v, tx.Commit(ctx)
}
func (d *DB) Import(ctx context.Context, data catalog.Dataset, operator string, dry bool) error {
	tx, e := d.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829104)"); e != nil {
		return e
	}
	old, e := load(ctx, tx)
	if e != nil {
		return e
	}
	existing := map[string]bool{}
	for _, c := range old {
		existing[c.ID] = true
	}
	for _, c := range data.Courses {
		action := "add"
		if existing[c.ID] {
			action = "update"
		}
		fmt.Printf("%s %s (%s)\n", action, c.ID, c.Status)
	}
	if dry {
		return nil
	}
	if e = importTx(ctx, tx, old, data, operator); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// UpdateVerified merges observations against the current catalog while holding
// the same publication lock as manual imports. The callback must not fetch HTTP.
func (d *DB) UpdateVerified(ctx context.Context, domains []string, operator string, merge func(context.Context, pgx.Tx, []catalog.Course) ([]catalog.Course, error)) (int, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829104)"); err != nil {
		return 0, err
	}
	old, err := load(ctx, tx)
	if err != nil {
		return 0, err
	}
	courses, err := merge(ctx, tx, old)
	if err != nil {
		return 0, err
	}
	if len(courses) > 0 {
		// Preserve domains approved by earlier operator imports, including
		// independently added tariffs retained by this merge.
		var previousDomains []byte
		if err = tx.QueryRow(ctx, "SELECT value FROM settings WHERE key='domains'").Scan(&previousDomains); err != nil && err != pgx.ErrNoRows {
			return 0, err
		}
		var approved []string
		if len(previousDomains) > 0 {
			if err = json.Unmarshal(previousDomains, &approved); err != nil {
				return 0, err
			}
		}
		data := catalog.Dataset{Domains: append(approved, domains...), Courses: courses}
		raw, err := json.Marshal(data)
		if err != nil {
			return 0, err
		}
		if _, err = catalog.Decode(bytes.NewReader(raw)); err != nil {
			return 0, err
		}
		if err = importTx(ctx, tx, old, data, operator); err != nil {
			return 0, err
		}
	}
	return len(courses), tx.Commit(ctx)
}

func importTx(ctx context.Context, tx pgx.Tx, old []catalog.Course, data catalog.Dataset, operator string) error {
	var e error
	for _, c := range data.Courses {
		_, e = tx.Exec(ctx, `INSERT INTO courses VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT(id) DO UPDATE SET slug=EXCLUDED.slug,title=EXCLUDED.title,provider=EXCLUDED.provider,language=EXCLUDED.language,direction=EXCLUDED.direction,summary=EXCLUDED.summary,audience=EXCLUDED.audience,goals=EXCLUDED.goals,topics=EXCLUDED.topics,source=EXCLUDED.source,checked_at=EXCLUDED.checked_at,status=EXCLUDED.status,demo=EXCLUDED.demo`, c.ID, c.Slug, c.Title, c.Provider, c.Language, c.Direction, c.Summary, c.Audience, c.Goals, c.Topics, c.Source, c.CheckedAt, c.Status, c.Demo)
		if e != nil {
			return e
		}
		// Preserve omitted offers as unavailable so saved comparison URLs remain meaningful.
		if _, e = tx.Exec(ctx, "UPDATE offers SET enrollment='closed' WHERE course_id=$1", c.ID); e != nil {
			return e
		}
		for _, o := range c.Offers {
			_, e = tx.Exec(ctx, `INSERT INTO offers VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,price=EXCLUDED.price,price_kind=EXCLUDED.price_kind,free=EXCLUDED.free,price_checked_at=EXCLUDED.price_checked_at,valid_until=EXCLUDED.valid_until,hours=EXCLUDED.hours,weeks=EXCLUDED.weeks,review=EXCLUDED.review,mentor=EXCLUDED.mentor,schedule=EXCLUDED.schedule,enrollment=EXCLUDED.enrollment,url=EXCLUDED.url WHERE offers.course_id=EXCLUDED.course_id`, o.ID, c.ID, o.Name, o.Price, o.PriceKind, o.Free, o.PriceCheckedAt, o.ValidUntil, o.Hours, o.Weeks, o.Review, o.Mentor, o.Schedule, o.Enrollment, o.URL)
			if e != nil {
				return e
			}
		}
	}
	before, _ := json.Marshal(old)
	after, _ := json.Marshal(data)
	if _, e = tx.Exec(ctx, "INSERT INTO imports(operator,before_data,after_data) VALUES($1,$2,$3)", operator, before, after); e != nil {
		return e
	}
	var prev []string
	var raw []byte
	if e = tx.QueryRow(ctx, "SELECT value FROM settings WHERE key='domains'").Scan(&raw); e == nil {
		json.Unmarshal(raw, &prev)
	}
	domainSet := map[string]bool{}
	for _, v := range append(prev, data.Domains...) {
		domainSet[v] = true
	}
	domains := []string{}
	for v := range domainSet {
		domains = append(domains, v)
	}
	raw, _ = json.Marshal(domains)
	if _, e = tx.Exec(ctx, "INSERT INTO settings VALUES('domains',$1) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value", raw); e != nil {
		return e
	}
	return nil
}
func (d *DB) Domains(ctx context.Context) []string {
	var raw []byte
	var ds []string
	if d.Pool.QueryRow(ctx, "SELECT value FROM settings WHERE key='domains'").Scan(&raw) == nil {
		json.Unmarshal(raw, &ds)
	}
	return ds
}
func (d *DB) Event(ctx context.Context, id, kind, cid, lang, goal string, total int) error {
	tx, e := d.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "INSERT INTO events(id,kind,course_id,language,goal,total) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING", id, kind, cid, lang, goal, total)
	if e != nil {
		return e
	}
	if tag.RowsAffected() > 0 {
		_, e = tx.Exec(ctx, `INSERT INTO daily_stats VALUES(CURRENT_DATE,$1,$2,$3,$4,1) ON CONFLICT(day,kind,course_id,language,goal) DO UPDATE SET count=daily_stats.count+1`, kind, cid, lang, goal)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (d *DB) Cleanup(ctx context.Context) {
	d.Pool.Exec(ctx, "DELETE FROM events WHERE created_at < now()-interval '30 days'")
	d.Pool.Exec(ctx, "DELETE FROM daily_stats WHERE day < CURRENT_DATE-interval '12 months'")
}
