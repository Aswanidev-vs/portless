package broker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"gotunnel/internal/protocol"
)

type Store interface {
	SaveLease(context.Context, protocol.Lease) error
	GetLease(context.Context, string) (protocol.Lease, error)
	SaveRelay(context.Context, protocol.RelayRegistration) error
	ListRelays(context.Context) ([]protocol.RelayRegistration, error)
}

type postgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	store := &postgresStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *postgresStore) migrate(ctx context.Context) error {
	stmts := []string{
		`create table if not exists leases (id text primary key, payload jsonb not null, created_at timestamptz not null default now())`,
		`create table if not exists relays (id text primary key, payload jsonb not null, updated_at timestamptz not null default now())`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *postgresStore) SaveLease(ctx context.Context, lease protocol.Lease) error {
	payload, _ := json.Marshal(lease)
	_, err := s.db.ExecContext(ctx, `insert into leases (id, payload, created_at) values ($1, $2, $3) on conflict (id) do update set payload = excluded.payload`, lease.ID, payload, lease.CreatedAt)
	return err
}

func (s *postgresStore) GetLease(ctx context.Context, id string) (protocol.Lease, error) {
	var payload []byte
	if err := s.db.QueryRowContext(ctx, `select payload from leases where id = $1`, id).Scan(&payload); err != nil {
		return protocol.Lease{}, err
	}
	var lease protocol.Lease
	if err := json.Unmarshal(payload, &lease); err != nil {
		return protocol.Lease{}, err
	}
	return lease, nil
}

func (s *postgresStore) SaveRelay(ctx context.Context, relay protocol.RelayRegistration) error {
	payload, _ := json.Marshal(relay)
	_, err := s.db.ExecContext(ctx, `insert into relays (id, payload, updated_at) values ($1, $2, $3) on conflict (id) do update set payload = excluded.payload, updated_at = excluded.updated_at`, relay.ID, payload, time.Now().UTC())
	return err
}

func (s *postgresStore) ListRelays(ctx context.Context) ([]protocol.RelayRegistration, error) {
	rows, err := s.db.QueryContext(ctx, `select payload from relays`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []protocol.RelayRegistration{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var relay protocol.RelayRegistration
		if err := json.Unmarshal(payload, &relay); err != nil {
			return nil, err
		}
		out = append(out, relay)
	}
	return out, rows.Err()
}

type memoryStore struct{}

func NewMemoryStore() Store { return &memoryStore{} }

func (m *memoryStore) SaveLease(context.Context, protocol.Lease) error { return nil }
func (m *memoryStore) GetLease(context.Context, string) (protocol.Lease, error) {
	return protocol.Lease{}, fmt.Errorf("not found")
}
func (m *memoryStore) SaveRelay(context.Context, protocol.RelayRegistration) error { return nil }
func (m *memoryStore) ListRelays(context.Context) ([]protocol.RelayRegistration, error) {
	return nil, nil
}
