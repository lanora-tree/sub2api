package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestBulkUpdateEncryptsCredentialPatches(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	store := testAccountCredentialStore(t)
	repo := &accountRepository{client: client, sql: db, credentialStore: store}

	credentials := map[string]any{"api_key": "old-secret", "base_url": "https://ollama.com"}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT.*FROM "accounts".*FOR UPDATE`).
		WithArgs(int64(17)).
		WillReturnRows(encryptedAccountRows(t, store, 17, credentials))
	mock.ExpectQuery(`(?s)SELECT.*FROM "accounts".*WHERE "accounts"\."id" = \$1.*LIMIT 2`).
		WithArgs(int64(17)).
		WillReturnRows(encryptedAccountRows(t, store, 17, credentials))
	mock.ExpectExec(`(?s)UPDATE accounts.*credentials = '\{\}'::jsonb.*credentials_encrypted = \$1`).
		WithArgs(
			sqlmock.AnyArg(), 1, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			false, `{"base_url":"https://ollama.com"}`, int64(17),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	rows, err := repo.BulkUpdate(context.Background(), []int64{17}, service.AccountBulkUpdate{
		Credentials: map[string]any{"api_key": "rotated-secret"},
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func encryptedAccountRows(t *testing.T, store *accountCredentialStore, id int64, credentials map[string]any) *sqlmock.Rows {
	t.Helper()
	sealed, err := store.seal(credentials, uuid.MustParse("018f6f52-4a41-7c2e-9cc6-8996f7123456"), service.PlatformOpenAI)
	require.NoError(t, err)
	metadata, err := json.Marshal(sealed.Metadata)
	require.NoError(t, err)
	now := time.Now()
	return sqlmock.NewRows(dbaccount.Columns).AddRow(
		id, now, now, nil, "test", nil, service.PlatformOpenAI, service.AccountTypeAPIKey,
		[]byte(`{}`), sealed.Ciphertext, sealed.KeyVersion, sealed.AADID.String(),
		sealed.Fingerprint, sealed.APIKeyDigest, sealed.HasRefreshToken, metadata,
		[]byte(`{}`), nil, nil, 1, nil, 1, 1.0, service.StatusActive, nil, nil, nil,
		false, true, nil, nil, nil, nil, nil, nil, nil, "", nil, service.QuotaDimensionGlobal,
	)
}
