//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/crypto"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// GetUserCredentials returns per-user credential overrides for a CLI binary.
// Returns (nil, nil) if no per-user credentials exist.
func (s *SQLiteSecureCLIStore) GetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) (*store.SecureCLIUserCredential, error) {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}

	var uc store.SecureCLIUserCredential
	var env []byte
	var createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx,
		`SELECT id, binary_id, user_id, encrypted_env, COALESCE(metadata, '{}'), created_at, updated_at
		 FROM secure_cli_user_credentials
		 WHERE binary_id = ? AND user_id = ? AND tenant_id = ?`,
		binaryID, userID, tid,
	).Scan(&uc.ID, &uc.BinaryID, &uc.UserID, &env, &uc.Metadata, &createdAt, &updatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	uc.CreatedAt = createdAt
	uc.UpdatedAt = updatedAt

	// Decrypt env
	if len(env) > 0 && s.encKey != "" {
		if decrypted, err := crypto.Decrypt(string(env), s.encKey); err == nil {
			uc.EncryptedEnv = []byte(decrypted)
		}
	} else {
		uc.EncryptedEnv = env
	}

	return &uc, nil
}

// SetUserCredentials creates or updates per-user encrypted env overrides (upsert).
// Encrypts the env bytes before storing. metadata may be nil/empty; defaults to "{}".
func (s *SQLiteSecureCLIStore) SetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string, encryptedEnv []byte, metadata json.RawMessage) error {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}

	var envBytes []byte
	if len(encryptedEnv) > 0 && s.encKey != "" {
		encrypted, err := crypto.Encrypt(string(encryptedEnv), s.encKey)
		if err != nil {
			return fmt.Errorf("encrypt env: %w", err)
		}
		envBytes = []byte(encrypted)
	} else {
		envBytes = encryptedEnv
	}
	if len(metadata) == 0 {
		metadata = json.RawMessage("{}")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := store.GenNewID()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO secure_cli_user_credentials (id, binary_id, user_id, encrypted_env, metadata, tenant_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (binary_id, user_id, tenant_id) DO UPDATE SET
		   encrypted_env = excluded.encrypted_env,
		   metadata = excluded.metadata,
		   updated_at = excluded.updated_at`,
		id, binaryID, userID, envBytes, string(metadata), tid, now, now,
	)
	return err
}

// DeleteUserCredentials removes per-user credentials for a binary.
func (s *SQLiteSecureCLIStore) DeleteUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) error {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM secure_cli_user_credentials WHERE binary_id = ? AND user_id = ? AND tenant_id = ?`,
		binaryID, userID, tid,
	)
	return err
}

// ListUserCredentials returns all per-user credentials for a binary (tenant-scoped).
func (s *SQLiteSecureCLIStore) ListUserCredentials(ctx context.Context, binaryID uuid.UUID) ([]store.SecureCLIUserCredential, error) {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, binary_id, user_id, encrypted_env, COALESCE(metadata, '{}'), created_at, updated_at
		 FROM secure_cli_user_credentials
		 WHERE binary_id = ? AND tenant_id = ?
		 ORDER BY created_at`, binaryID, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []store.SecureCLIUserCredential
	for rows.Next() {
		var uc store.SecureCLIUserCredential
		var env []byte
		var createdAt, updatedAt string

		if err := rows.Scan(&uc.ID, &uc.BinaryID, &uc.UserID, &env, &uc.Metadata, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		uc.CreatedAt = createdAt
		uc.UpdatedAt = updatedAt

		if len(env) > 0 && s.encKey != "" {
			if decrypted, err := crypto.Decrypt(string(env), s.encKey); err == nil {
				uc.EncryptedEnv = []byte(decrypted)
			}
		} else {
			uc.EncryptedEnv = env
		}

		result = append(result, uc)
	}
	return result, rows.Err()
}

// ListUserCredentialsByBinaryName lists user-credential rows JOINed with
// their binary across ALL tenants (caller MUST switch tenant ctx per-row
// before writes). Used by the B3-01 refresh worker.
func (s *SQLiteSecureCLIStore) ListUserCredentialsByBinaryName(ctx context.Context, binaryName string) ([]store.SecureCLIUserCredentialWithBinary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uc.id, uc.binary_id, uc.user_id, uc.encrypted_env, COALESCE(uc.metadata, '{}'), uc.created_at, uc.updated_at,
		        b.binary_name, b.tenant_id
		 FROM secure_cli_user_credentials uc
		 JOIN secure_cli_binaries b ON b.id = uc.binary_id
		 WHERE LOWER(b.binary_name) = LOWER(?)
		 ORDER BY uc.updated_at DESC`, binaryName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []store.SecureCLIUserCredentialWithBinary
	for rows.Next() {
		var row store.SecureCLIUserCredentialWithBinary
		var env []byte
		if err := rows.Scan(
			&row.ID, &row.BinaryID, &row.UserID, &env, &row.Metadata, &row.CreatedAt, &row.UpdatedAt,
			&row.BinaryName, &row.TenantID,
		); err != nil {
			return nil, err
		}
		if len(env) > 0 && s.encKey != "" {
			if decrypted, err := crypto.Decrypt(string(env), s.encKey); err == nil {
				row.EncryptedEnv = []byte(decrypted)
			}
		} else {
			row.EncryptedEnv = env
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
