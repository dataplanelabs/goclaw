package pg

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

func (s *PGSecureCLIStore) GetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) (*store.SecureCLIUserCredential, error) {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}
	var uc store.SecureCLIUserCredential
	var env []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, binary_id, user_id, encrypted_env, metadata, created_at, updated_at
		 FROM secure_cli_user_credentials
		 WHERE binary_id = $1 AND user_id = $2 AND tenant_id = $3`,
		binaryID, userID, tid,
	).Scan(&uc.ID, &uc.BinaryID, &uc.UserID, &env, &uc.Metadata, &uc.CreatedAt, &uc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
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

func (s *PGSecureCLIStore) SetUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string, encryptedEnv []byte, metadata json.RawMessage) error {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}
	// Encrypt env
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
	// metadata defaults to "{}" when caller passes nil/empty (back-compat with
	// the existing PUT handler that doesn't carry metadata).
	if len(metadata) == 0 {
		metadata = json.RawMessage("{}")
	}

	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO secure_cli_user_credentials (binary_id, user_id, encrypted_env, metadata, tenant_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)
		 ON CONFLICT (binary_id, user_id, tenant_id) DO UPDATE SET
		   encrypted_env = EXCLUDED.encrypted_env,
		   metadata = EXCLUDED.metadata,
		   updated_at = EXCLUDED.updated_at`,
		binaryID, userID, envBytes, []byte(metadata), tid, now,
	)
	return err
}

func (s *PGSecureCLIStore) DeleteUserCredentials(ctx context.Context, binaryID uuid.UUID, userID string) error {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM secure_cli_user_credentials WHERE binary_id = $1 AND user_id = $2 AND tenant_id = $3`,
		binaryID, userID, tid,
	)
	return err
}

func (s *PGSecureCLIStore) ListUserCredentials(ctx context.Context, binaryID uuid.UUID) ([]store.SecureCLIUserCredential, error) {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, binary_id, user_id, encrypted_env, metadata, created_at, updated_at
		 FROM secure_cli_user_credentials
		 WHERE binary_id = $1 AND tenant_id = $2
		 ORDER BY created_at`, binaryID, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []store.SecureCLIUserCredential
	for rows.Next() {
		var uc store.SecureCLIUserCredential
		var env []byte
		if err := rows.Scan(&uc.ID, &uc.BinaryID, &uc.UserID, &env, &uc.Metadata, &uc.CreatedAt, &uc.UpdatedAt); err != nil {
			return nil, err
		}
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
// their binary across ALL tenants. Caller (e.g. refresh worker) is
// expected to switch tenant ctx per-row before writes.
func (s *PGSecureCLIStore) ListUserCredentialsByBinaryName(ctx context.Context, binaryName string) ([]store.SecureCLIUserCredentialWithBinary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uc.id, uc.binary_id, uc.user_id, uc.encrypted_env, uc.metadata, uc.created_at, uc.updated_at,
		        b.binary_name, b.tenant_id
		 FROM secure_cli_user_credentials uc
		 JOIN secure_cli_binaries b ON b.id = uc.binary_id
		 WHERE LOWER(b.binary_name) = LOWER($1)
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
