package identity

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/uuid"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

func resolve(t *testing.T, s *Store, name string, id uuid.UUID, source Source) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := s.Resolve(ctx, name, id, source)
	require.NoError(t, err)
	return result
}

func mustLookup(t *testing.T, s *Store, name string) (Record, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, ok, err := s.Lookup(ctx, name)
	require.NoError(t, err)
	return record, ok
}

// The first login of a username binds the account to the UUID that login
// resolved to, and an unauthenticated one does not mark it as authenticated.
func TestFirstRegistrationBindsUUID(t *testing.T) {
	s := openTestStore(t)
	offlineID := uuid.OfflinePlayerUUID("Steve")

	result := resolve(t, s, "Steve", offlineID, SourceOffline)

	require.True(t, result.Created)
	require.Equal(t, MatchNew, result.Match)
	require.Equal(t, "Steve", result.Name)
	require.Equal(t, offlineID, result.ActualID)
	require.False(t, result.Authenticated)
}

// Every later login of the same username uses the bound UUID: an authenticated
// login inherits the state an offline login accumulated.
func TestAuthenticatedLoginKeepsBoundUUID(t *testing.T) {
	s := openTestStore(t)
	offlineID := uuid.OfflinePlayerUUID("Alex")
	premiumID := uuid.New()

	first := resolve(t, s, "Alex", offlineID, SourceOffline)
	second := resolve(t, s, "Alex", premiumID, SourcePremium)

	require.Equal(t, offlineID, first.ActualID)
	require.Equal(t, offlineID, second.ActualID, "the bound UUID is kept")
	require.False(t, second.Created)
	require.Equal(t, MatchName, second.Match)
	require.True(t, second.Authenticated, "the username is now known to be authenticated")

	record, ok := mustLookup(t, s, "Alex")
	require.True(t, ok)
	require.Equal(t, offlineID, record.ActualID)
	require.True(t, record.Authenticated)
}

// The reverse direction: an authenticated first login binds the account to the
// Mojang UUID, and a later offline login keeps it.
func TestOfflineLoginKeepsBoundUUID(t *testing.T) {
	s := openTestStore(t)
	premiumID := uuid.New()

	first := resolve(t, s, "Alex", premiumID, SourcePremium)
	second := resolve(t, s, "Alex", uuid.OfflinePlayerUUID("Alex"), SourceOffline)

	require.True(t, first.Created)
	require.True(t, first.Authenticated)
	require.Equal(t, premiumID, second.ActualID, "the offline login keeps the bound UUID")
	require.True(t, second.Authenticated, "an offline login does not clear the authenticated mark")
}

// An identity a connection type supplied without vouching for it registers the
// account but never marks it as authenticated.
func TestInjectedLoginDoesNotMarkAuthenticated(t *testing.T) {
	s := openTestStore(t)
	injectedID := uuid.New()

	injected := resolve(t, s, "Steve", injectedID, SourceInjected)
	require.True(t, injected.Created)
	require.Equal(t, injectedID, injected.ActualID)
	require.False(t, injected.Authenticated)

	authenticated := resolve(t, s, "Steve", uuid.New(), SourcePremium)
	require.True(t, authenticated.Authenticated)
	require.Equal(t, injectedID, authenticated.ActualID, "the bound UUID stays")
}

// A connect-authenticated identity is authenticated like a Mojang one, because
// the store records whether the account has authenticated, not who verified it:
// that mark is what makes premium protection sticky for the username.
func TestConnectAuthenticatedLoginMarksAuthenticated(t *testing.T) {
	s := openTestStore(t)
	suppliedID := uuid.New()
	offlineID := uuid.OfflinePlayerUUID("Steve")

	vouched := resolve(t, s, "Steve", suppliedID, SourceConnectAuthenticated)
	require.True(t, vouched.Created)
	require.Equal(t, suppliedID, vouched.ActualID)
	require.True(t, vouched.Authenticated)

	// The mark survives an offline login, so the account stays protected.
	offline := resolve(t, s, "Steve", offlineID, SourceOffline)
	require.True(t, offline.Authenticated, "an offline login does not clear the authenticated mark")
	require.Equal(t, suppliedID, offline.ActualID)
}

// Accounts are keyed by the exact username: two usernames that differ only in
// case are different accounts with their own UUIDs.
func TestUsernameIsCaseSensitive(t *testing.T) {
	s := openTestStore(t)
	offlineID := uuid.OfflinePlayerUUID("Steve")
	otherOfflineID := uuid.OfflinePlayerUUID("sTeVe")

	first := resolve(t, s, "Steve", offlineID, SourceOffline)
	second := resolve(t, s, "sTeVe", otherOfflineID, SourceOffline)

	require.True(t, first.Created)
	require.True(t, second.Created, "a differently cased username is its own account")
	require.Equal(t, offlineID, first.ActualID)
	require.Equal(t, otherOfflineID, second.ActualID)

	count, err := s.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

// A Mojang rename changes the username but not the Mojang UUID, so the login
// finds the account by its bound UUID and keeps it - including the
// authenticated mark.
func TestRenameResolvesByBoundUUID(t *testing.T) {
	s := openTestStore(t)
	premiumID := uuid.New()

	resolve(t, s, "Alex", premiumID, SourcePremium)
	renamed := resolve(t, s, "Alexandra", premiumID, SourcePremium)

	require.False(t, renamed.Created)
	require.Equal(t, MatchActual, renamed.Match)
	require.Equal(t, premiumID, renamed.ActualID)
	require.True(t, renamed.Authenticated)
	require.Equal(t, "Alexandra", renamed.Name)

	_, ok := mustLookup(t, s, "Alex")
	require.False(t, ok, "the account moved to the new username")
	count, err := s.Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count, "a rename must not create a second account")
}

// Two logins racing for the same username must agree on one UUID.
func TestConcurrentFirstRegistration(t *testing.T) {
	s := openTestStore(t)
	const name = "Race"
	offlineID := uuid.OfflinePlayerUUID(name)
	premiumID := uuid.New()

	type outcome struct {
		result Result
		err    error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	go func() {
		<-start
		r, err := s.Resolve(context.Background(), name, offlineID, SourceOffline)
		results <- outcome{r, err}
	}()
	go func() {
		<-start
		r, err := s.Resolve(context.Background(), name, premiumID, SourcePremium)
		results <- outcome{r, err}
	}()
	close(start)

	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, first.result.ActualID, second.result.ActualID,
		"both logins must be assigned the same UUID")
	require.NotEqual(t, first.result.Created, second.result.Created,
		"exactly one of them registers the account")
}

// The store must survive a restart: identities live on disk, not in memory.
func TestPersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identities.db")
	offlineID := uuid.OfflinePlayerUUID("Steve")

	store, err := Open(path)
	require.NoError(t, err)
	resolve(t, store, "Steve", offlineID, SourceOffline)
	require.NoError(t, store.Close())

	reopened, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	record, ok := mustLookup(t, reopened, "Steve")
	require.True(t, ok)
	require.Equal(t, offlineID, record.ActualID)

	after := resolve(t, reopened, "Steve", uuid.New(), SourcePremium)
	require.Equal(t, offlineID, after.ActualID)
	require.True(t, after.Authenticated)
}

func TestResolveRejectsInvalidArguments(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	_, err := s.Resolve(ctx, "", uuid.New(), SourceOffline)
	require.Error(t, err)
	_, err = s.Resolve(ctx, "Steve", uuid.Nil, SourceOffline)
	require.Error(t, err)
	_, err = s.Resolve(ctx, "Steve", uuid.New(), Source("bogus"))
	require.Error(t, err)

	_, err = Open("")
	require.Error(t, err)
}

// A database written by another Gate must not be silently reinterpreted: the
// store refuses to open it and says to start from a fresh database.
func TestRejectsForeignSchemaVersion(t *testing.T) {
	for name, version := range map[string]int{"older": schemaVersion - 1, "newer": schemaVersion + 1} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "identities.db")
			store, err := Open(path)
			require.NoError(t, err)
			require.NoError(t, store.Close())

			db, err := sql.Open(DriverName, "file:"+path)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE meta SET value = ? WHERE key = 'schema_version'`, version)
			require.NoError(t, err)
			require.NoError(t, db.Close())

			_, err = Open(path)
			require.ErrorContains(t, err, "remove the file to start from a fresh database")
		})
	}
}

// A database left behind by an unreleased development iteration carries the same
// schema version with different columns. Open must refuse it at startup rather
// than let every login fail on a missing column.
func TestRejectsVersion1WithForeignShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identities.db")
	db, err := sql.Open(DriverName, "file:"+path)
	require.NoError(t, err)
	_, err = db.Exec(`
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE players (
	name       TEXT PRIMARY KEY,
	actual_uid BLOB NOT NULL,
	premium    INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
INSERT INTO meta (key, value) VALUES ('schema_version', 1);`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = Open(path)
	require.ErrorContains(t, err, "no players.authenticated column")
	require.ErrorContains(t, err, "remove the file to start from a fresh database")
}
