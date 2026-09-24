// Package identity stores the UUID a player is known by on this proxy's
// backend servers, so that one account keeps a single UUID whether it logs in
// with an authenticated identity or with an offline one.
//
// An account is a username plus the UUID it was first bound to, and a flag
// saying whether that username has ever been logged in with an authenticated
// identity. Every login of the same username - offline, authenticated or
// supplied by a connection type - is assigned the bound UUID, which is what
// lets one login path inherit the state another one accumulated.
//
// Accounts are keyed by the exact username: usernames that differ only in case
// are different accounts with different UUIDs, which keeps two cracked players
// who spell a name differently from sharing an account.
package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	// Pure Go SQLite driver: Gate is built with CGO_ENABLED=0, so a cgo driver
	// such as mattn/go-sqlite3 is not an option.
	_ "modernc.org/sqlite"

	"go.minekube.com/gate/pkg/util/uuid"
)

// DriverName is the database/sql driver registered by modernc.org/sqlite.
const DriverName = "sqlite"

// schemaVersion is the schema this package writes and understands: accounts
// keyed by the exact username, holding the UUID bound at the first login and a
// flag for whether the username has ever been authenticated.
//
// This is the first released schema, so it is version 1. Earlier development
// iterations (a lower-cased username key, then separate offline and
// authenticated UUID columns) were never released and are not versioned: Open
// refuses a store of any other version and says so, and the operator starts
// from a fresh database. Open also refuses a version 1 database whose players
// table is not this shape, which is what a database written by an unreleased
// development iteration of this same version looks like.
const schemaVersion = 1

// Source describes which identity a login resolved to before the store
// canonicalized it.
type Source string

const (
	// SourceOffline is an unauthenticated offline-mode identity; its UUID is
	// the UUID v3 digest of the username.
	SourceOffline Source = "offline"
	// SourcePremium is an identity the proxy authenticated with Mojang. It
	// marks the username as authenticated.
	SourcePremium Source = "premium"
	// SourceConnectAuthenticated is an identity a connection type supplied and
	// vouched for: the ingress declared that its endpoint does not accept
	// offline-mode players (connect.allowOfflineModePlayers false), so the tunnel
	// service only proposes authenticated players to it.
	//
	// It marks the username as authenticated, like SourcePremium does, because
	// the store records whether the account has authenticated - not who verified
	// it. The login log keeps the two apart, so an audit can still tell which
	// logins the proxy verified itself.
	SourceConnectAuthenticated Source = "connect-authenticated"
	// SourceInjected is any other supplied identity, e.g. one a connection
	// type such as Geyser or a Connect tunnel contributed without vouching for
	// it. It says no more than "this UUID is not the username's offline UUID":
	// nothing verifies that the identity belongs to the account, so it never
	// marks the username as authenticated.
	SourceInjected Source = "injected"
)

func (s Source) valid() bool {
	switch s {
	case SourceOffline, SourcePremium, SourceConnectAuthenticated, SourceInjected:
		return true
	}
	return false
}

// Authenticated reports whether an identity from this source proves that the
// account has authenticated: the proxy verified it with Mojang, or an ingress
// the operator trusts vouched for it. Only these sources mark a username as
// authenticated, and only these sources may use an account that is protected.
func (s Source) Authenticated() bool {
	switch s {
	case SourcePremium, SourceConnectAuthenticated:
		return true
	}
	return false
}

// Match reports how a stored account was found for a login.
type Match string

const (
	// MatchNew means the login registered a new account.
	MatchNew Match = "new"
	// MatchName matched the username, the normal case.
	MatchName Match = "name"
	// MatchActual matched the account's bound UUID, which is how a login under a
	// new username (a Mojang rename) finds the account it belongs to.
	MatchActual Match = "actual"
)

// Record is one account as stored: a username, the UUID it is bound to, and
// whether it has ever been authenticated.
type Record struct {
	Name          string    // the exact username, the account's key
	ActualID      uuid.UUID // the UUID the backend server uses, set by the first login
	Authenticated bool      // whether an authenticated login was ever seen
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Result reports what a Resolve call did.
type Result struct {
	Record
	Match   Match
	Created bool
}

// Store is a SQLite-backed player identity store.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (and creates if necessary) the store at path and applies the
// schema. The parent directory is created when missing.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("identity store path is empty")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create identity store directory: %w", err)
		}
	}
	// WAL keeps readers from blocking the single writer; busy_timeout makes
	// concurrent writers wait instead of failing. One connection per process
	// serializes the read-modify-write Resolve performs.
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open identity store: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	s := &Store{db: db, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping identity store %q: %w", path, err)
	}
	if err = s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate identity store %q: %w", path, err)
	}
	return s, nil
}

// Path returns the configured database file path.
func (s *Store) Path() string { return s.path }

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
)`); err != nil {
		return err
	}
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'schema_version'`).Scan(&version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err = s.createPlayersTable(ctx); err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES ('schema_version', ?)`, schemaVersion)
		return err
	case err != nil:
		return err
	case version != schemaVersion:
		return fmt.Errorf("identity store %q has schema version %d, this Gate writes version %d; "+
			"remove the file to start from a fresh database", s.path, version, schemaVersion)
	}
	return s.checkPlayersTable(ctx)
}

func (s *Store) createPlayersTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS players (
	name          TEXT PRIMARY KEY,
	actual_uid    BLOB NOT NULL,
	authenticated INTEGER NOT NULL,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_players_actual_uid ON players(actual_uid);`)
	return err
}

// checkPlayersTable verifies that an existing version 1 database has the table
// shape this version writes. A database left behind by an unreleased
// development iteration carries the same version but different columns, and
// without this check it would fail on every login instead of at startup.
func (s *Store) checkPlayersTable(ctx context.Context) error {
	columns, err := s.db.QueryContext(ctx, `SELECT name FROM pragma_table_info('players')`)
	if err != nil {
		return err
	}
	defer func() { _ = columns.Close() }()

	got := make(map[string]struct{}, 5)
	for columns.Next() {
		var name string
		if err = columns.Scan(&name); err != nil {
			return err
		}
		got[name] = struct{}{}
	}
	if err = columns.Err(); err != nil {
		return err
	}
	for _, want := range []string{"name", "actual_uid", "authenticated", "created_at", "updated_at"} {
		if _, ok := got[want]; !ok {
			return fmt.Errorf("identity store %q has no players.%s column; "+
				"remove the file to start from a fresh database", s.path, want)
		}
	}
	return nil
}

// Resolve returns the UUID the player is known by on this proxy's backend
// servers, registering the account on its first login.
//
// resolved is the UUID the normal login flow produced and source says which
// identity that was. The returned ActualID is the account's bound UUID: on the
// first login it is resolved itself, afterwards it never changes. A login whose
// source is authenticated - the proxy verified it with Mojang, or a trusted
// ingress vouched for it - also marks the username as authenticated, for good.
func (s *Store) Resolve(ctx context.Context, name string, resolved uuid.UUID, source Source) (Result, error) {
	if strings.TrimSpace(name) == "" {
		return Result{}, errors.New("player name is empty")
	}
	if resolved == uuid.Nil {
		return Result{}, errors.New("resolved uuid is empty")
	}
	if !source.valid() {
		return Result{}, fmt.Errorf("unknown identity source %q", source)
	}

	now := time.Now().UTC()
	authenticated := source.Authenticated()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin identity transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, match, err := find(ctx, tx, name, resolved)
	if err != nil {
		return Result{}, err
	}

	if match == "" {
		// First registration: this login's UUID is the account's from now on.
		res, err := tx.ExecContext(ctx, `
INSERT INTO players (name, actual_uid, authenticated, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(name) DO NOTHING`,
			name, blob(resolved), boolToInt(authenticated), now.UnixMilli(), now.UnixMilli())
		if err != nil {
			return Result{}, fmt.Errorf("register player identity: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return Result{}, err
		}
		if affected == 0 {
			// Another process registered the same name between our read and
			// write; use its account so both logins agree on one UUID.
			rec, match, err = find(ctx, tx, name, resolved)
			if err != nil {
				return Result{}, err
			}
			if match == "" {
				return Result{}, errors.New("identity registration lost a race and the account is gone")
			}
			if err = tx.Commit(); err != nil {
				return Result{}, err
			}
			return Result{Record: rec, Match: match}, nil
		}
		rec = Record{
			Name: name, ActualID: resolved, Authenticated: authenticated, CreatedAt: now, UpdatedAt: now,
		}
		if err = tx.Commit(); err != nil {
			return Result{}, err
		}
		return Result{Record: rec, Match: MatchNew, Created: true}, nil
	}

	// Known account: keep its bound UUID and only ever add to what it records.
	// An authenticated login is sticky, and a login under a new username (a
	// Mojang rename) moves the account to it.
	authenticated = authenticated || rec.Authenticated
	if _, err = tx.ExecContext(ctx, `
UPDATE players SET name = ?, authenticated = ?, updated_at = ?
WHERE name = ?`,
		name, boolToInt(authenticated), now.UnixMilli(), rec.Name); err != nil {
		return Result{}, fmt.Errorf("update player identity: %w", err)
	}
	rec.Name = name
	rec.Authenticated = authenticated
	rec.UpdatedAt = now
	if err = tx.Commit(); err != nil {
		return Result{}, err
	}
	return Result{Record: rec, Match: match}, nil
}

// Lookup returns the stored account for a username without modifying it.
func (s *Store) Lookup(ctx context.Context, name string) (Record, bool, error) {
	rec, match, err := find(ctx, s.db, strings.TrimSpace(name), uuid.Nil)
	if err != nil {
		return Record{}, false, err
	}
	return rec, match != "", nil
}

// Count returns the number of registered accounts.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM players`).Scan(&n)
	return n, err
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const selectColumns = `SELECT name, actual_uid, authenticated, created_at, updated_at FROM players`

type lookup struct {
	column string
	arg    any
	match  Match
}

// find returns the account for a login: by the exact username, or - when the
// username is new, which is what a Mojang rename looks like - by the bound UUID
// it already resolved to.
func find(ctx context.Context, q querier, name string, resolved uuid.UUID) (Record, Match, error) {
	lookups := []lookup{{column: "name", arg: name, match: MatchName}}
	if resolved != uuid.Nil {
		lookups = append(lookups, lookup{column: "actual_uid", arg: resolved[:], match: MatchActual})
	}
	for _, l := range lookups {
		rec, err := scanRecord(q.QueryRowContext(ctx, selectColumns+` WHERE `+l.column+` = ?`, l.arg))
		if err == nil {
			return rec, l.match, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Record{}, "", fmt.Errorf("look up player identity: %w", err)
		}
	}
	return Record{}, "", nil
}

func scanRecord(row *sql.Row) (Record, error) {
	var (
		rec           Record
		actualID      []byte
		authenticated int64
		created       int64
		updated       int64
	)
	if err := row.Scan(&rec.Name, &actualID, &authenticated, &created, &updated); err != nil {
		return Record{}, err
	}
	id, err := parseID(actualID)
	if err != nil {
		return Record{}, err
	}
	rec.ActualID = id
	rec.Authenticated = authenticated != 0
	rec.CreatedAt = time.UnixMilli(created).UTC()
	rec.UpdatedAt = time.UnixMilli(updated).UTC()
	return rec, nil
}

func blob(id uuid.UUID) []byte { return id[:] }

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseID(b []byte) (uuid.UUID, error) {
	if len(b) == 0 {
		return uuid.Nil, nil
	}
	if len(b) != 16 {
		return uuid.Nil, fmt.Errorf("stored uuid has %d bytes, want 16", len(b))
	}
	var id uuid.UUID
	copy(id[:], b)
	return id, nil
}
