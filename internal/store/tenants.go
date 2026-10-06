package store

import "context"

// Query di file ini tidak di-scope per toko: hanya dipanggil oleh menu master
// dan middleware penentu toko aktif.

type TenantInput struct {
	Name, Info string
	Active     bool
}

const tenantSelectSQL = `
SELECT s.id, s.name, s.info, s.active, s.created_at,
       (SELECT COUNT(*) FROM users u WHERE u.store_id = s.id) AS admin_count
FROM stores s`

func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.db.Query(ctx, tenantSelectSQL+` ORDER BY s.active DESC, s.name`)
	return collect[Tenant](rows, err)
}

func (s *Store) TenantByID(ctx context.Context, id int64) (Tenant, error) {
	rows, err := s.db.Query(ctx, tenantSelectSQL+` WHERE s.id = $1`, id)
	return collectOne[Tenant](rows, err)
}

func (s *Store) CreateTenant(ctx context.Context, in TenantInput) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO stores (name, info, active) VALUES ($1, $2, $3) RETURNING id`,
		in.Name, in.Info, in.Active).Scan(&id)
	return id, err
}

func (s *Store) UpdateTenant(ctx context.Context, id int64, in TenantInput) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE stores SET name = $2, info = $3, active = $4 WHERE id = $1`, id, in.Name, in.Info, in.Active)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) ToggleTenant(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `UPDATE stores SET active = NOT active WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
