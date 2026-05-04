package db

import (
	"context"
	"time"
)

type HTTPExchange struct {
	ID         int       `json:"id"`
	LogID      string    `json:"log_id"`
	Direction  string    `json:"direction"` // "downstream" | "upstream"
	Phase      string    `json:"phase"`     // "request" | "response"
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method,omitempty"`
	URL        string    `json:"url,omitempty"`
	Path       string    `json:"path,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	Headers    string    `json:"headers,omitempty"`
	Body       string    `json:"body,omitempty"`
	DurationMs int       `json:"duration_ms,omitempty"`
	Error      string    `json:"error,omitempty"`
	RawStream  string    `json:"raw_stream,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) InsertHTTPExchange(ctx context.Context, e *HTTPExchange) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO http_exchanges (log_id, direction, phase, timestamp, method, url, path, status_code, headers, body, duration_ms, error, raw_stream)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.LogID, e.Direction, e.Phase, e.Timestamp, e.Method, e.URL, e.Path,
		e.StatusCode, e.Headers, e.Body, e.DurationMs, e.Error, e.RawStream,
	)
	return err
}

func (s *Store) GetHTTPExchangesByLogID(ctx context.Context, logID string) ([]HTTPExchange, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, log_id, direction, phase, timestamp, method, url, path, status_code, headers, body, duration_ms, error, raw_stream, created_at
		 FROM http_exchanges WHERE log_id = ? ORDER BY timestamp ASC`,
		logID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HTTPExchange
	for rows.Next() {
		var e HTTPExchange
		err := rows.Scan(
			&e.ID, &e.LogID, &e.Direction, &e.Phase, &e.Timestamp,
			&e.Method, &e.URL, &e.Path, &e.StatusCode, &e.Headers,
			&e.Body, &e.DurationMs, &e.Error, &e.RawStream, &e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	if result == nil {
		result = []HTTPExchange{}
	}
	return result, rows.Err()
}

func (s *Store) CleanupExpiredExchanges(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM http_exchanges WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
