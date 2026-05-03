package db

import (
	"context"
	"time"
)

func (s *Store) CleanupExpiredCanonicalRecords(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := s.db.ExecContext(ctx, `DELETE FROM canonical_execution_records WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
