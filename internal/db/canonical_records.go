package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"lingma2api/internal/proxy"
)

type CanonicalExecutionRecordRow struct {
	ID                string                           `json:"id"`
	CreatedAt         time.Time                        `json:"created_at"`
	IngressProtocol   string                           `json:"ingress_protocol"`
	IngressEndpoint   string                           `json:"ingress_endpoint"`
	SessionID         string                           `json:"session_id,omitempty"`
	PrePolicyRequest  proxy.CanonicalRequest           `json:"pre_policy_request"`
	PostPolicyRequest proxy.CanonicalRequest           `json:"post_policy_request"`
	SessionSnapshot   *proxy.CanonicalSessionSnapshot  `json:"session_snapshot,omitempty"`
	SouthboundRequest string                           `json:"southbound_request,omitempty"`
	Sidecar           *proxy.CanonicalExecutionSidecar `json:"sidecar,omitempty"`
}

func (s *Store) InsertCanonicalExecutionRecord(ctx context.Context, record *CanonicalExecutionRecordRow) error {
	preJSON, err := marshalJSON(record.PrePolicyRequest)
	if err != nil {
		return err
	}
	postJSON, err := marshalJSON(record.PostPolicyRequest)
	if err != nil {
		return err
	}
	sessionJSON, err := marshalJSON(record.SessionSnapshot)
	if err != nil {
		return err
	}
	sidecarJSON, err := marshalJSON(record.Sidecar)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO canonical_execution_records (
			id,created_at,ingress_protocol,ingress_endpoint,session_id,
			pre_policy_json,post_policy_json,session_snapshot_json,southbound_request,sidecar_json
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		record.ID, record.CreatedAt, record.IngressProtocol, record.IngressEndpoint, record.SessionID,
		preJSON, postJSON, sessionJSON, record.SouthboundRequest, sidecarJSON,
	)
	return err
}

func (s *Store) GetCanonicalExecutionRecord(ctx context.Context, id string) (CanonicalExecutionRecordRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,created_at,ingress_protocol,ingress_endpoint,session_id,
		 pre_policy_json,post_policy_json,session_snapshot_json,southbound_request,sidecar_json
		 FROM canonical_execution_records WHERE id=?`, id)
	return scanCanonicalExecutionRecord(row)
}

func (s *Store) ListCanonicalExecutionRecords(ctx context.Context, limit int) ([]CanonicalExecutionRecordRow, error) {
	query := `SELECT id,created_at,ingress_protocol,ingress_endpoint,session_id,
		 pre_policy_json,post_policy_json,session_snapshot_json,southbound_request,sidecar_json
		 FROM canonical_execution_records ORDER BY created_at DESC`
	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		rows, err = s.db.QueryContext(ctx, query+` LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CanonicalExecutionRecordRow
	for rows.Next() {
		record, err := scanCanonicalExecutionRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	if items == nil {
		items = []CanonicalExecutionRecordRow{}
	}
	return items, rows.Err()
}

func marshalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type canonicalRecordScanner interface {
	Scan(dest ...any) error
}

func scanCanonicalExecutionRecord(scanner canonicalRecordScanner) (CanonicalExecutionRecordRow, error) {
	var record CanonicalExecutionRecordRow
	var preJSON string
	var postJSON string
	var sessionJSON string
	var sidecarJSON string
	if err := scanner.Scan(
		&record.ID,
		&record.CreatedAt,
		&record.IngressProtocol,
		&record.IngressEndpoint,
		&record.SessionID,
		&preJSON,
		&postJSON,
		&sessionJSON,
		&record.SouthboundRequest,
		&sidecarJSON,
	); err != nil {
		return CanonicalExecutionRecordRow{}, err
	}
	if err := unmarshalOptionalJSON(preJSON, &record.PrePolicyRequest); err != nil {
		return CanonicalExecutionRecordRow{}, err
	}
	if err := unmarshalOptionalJSON(postJSON, &record.PostPolicyRequest); err != nil {
		return CanonicalExecutionRecordRow{}, err
	}
	record.SessionSnapshot = &proxy.CanonicalSessionSnapshot{}
	if err := unmarshalOptionalJSON(sessionJSON, record.SessionSnapshot); err != nil {
		return CanonicalExecutionRecordRow{}, err
	}
	if sessionJSON == "" {
		record.SessionSnapshot = nil
	}
	record.Sidecar = &proxy.CanonicalExecutionSidecar{}
	if err := unmarshalOptionalJSON(sidecarJSON, record.Sidecar); err != nil {
		return CanonicalExecutionRecordRow{}, err
	}
	if sidecarJSON == "" {
		record.Sidecar = nil
	}
	return record, nil
}

func unmarshalOptionalJSON(raw string, target any) error {
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	return nil
}
