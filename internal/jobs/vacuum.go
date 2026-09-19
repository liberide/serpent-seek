package jobs

import "context"

// Vacuum compacts the SQLite database (no-op on PostgreSQL).
func (m *Manager) Vacuum(ctx context.Context) error {
	if err := m.store.Vacuum(ctx); err != nil {
		return err
	}
	m.log.Info("", "", "vacuum done")
	return nil
}
