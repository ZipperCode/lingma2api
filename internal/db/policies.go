package db

import (
	"context"
	"encoding/json"
	"time"
)

type PolicyMatch struct {
	Protocol       string `json:"protocol,omitempty"`
	RequestedModel string `json:"requested_model,omitempty"`
	Stream         *bool  `json:"stream,omitempty"`
	HasTools       *bool  `json:"has_tools,omitempty"`
	HasReasoning   *bool  `json:"has_reasoning,omitempty"`
	SessionPresent *bool  `json:"session_present,omitempty"`
	ClientName     string `json:"client_name,omitempty"`
	IngressTag     string `json:"ingress_tag,omitempty"`
}

type PolicyActions struct {
	RewriteModel *string  `json:"rewrite_model,omitempty"`
	SetReasoning *bool    `json:"set_reasoning,omitempty"`
	AllowTools   *bool    `json:"allow_tools,omitempty"`
	AddTags      []string `json:"add_tags,omitempty"`
}

type PolicyRule struct {
	ID        int           `json:"id"`
	Priority  int           `json:"priority"`
	Name      string        `json:"name"`
	Enabled   bool          `json:"enabled"`
	Match     PolicyMatch   `json:"match"`
	Actions   PolicyActions `json:"actions"`
	Source    string        `json:"source"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func (s *Store) ListPolicies(ctx context.Context) ([]PolicyRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,priority,name,enabled,match_json,actions_json,source,created_at,updated_at
		 FROM policy_rules ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PolicyRule
	for rows.Next() {
		policy, err := scanPolicyRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, policy)
	}
	if items == nil {
		items = []PolicyRule{}
	}
	return items, rows.Err()
}

func (s *Store) GetEnabledPolicies(ctx context.Context) ([]PolicyRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,priority,name,enabled,match_json,actions_json,source,created_at,updated_at
		 FROM policy_rules WHERE enabled=1 ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PolicyRule
	for rows.Next() {
		policy, err := scanPolicyRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, policy)
	}
	if items == nil {
		items = []PolicyRule{}
	}
	return items, rows.Err()
}

func (s *Store) CreatePolicy(ctx context.Context, policy *PolicyRule) error {
	now := time.Now()
	matchJSON, err := json.Marshal(policy.Match)
	if err != nil {
		return err
	}
	actionsJSON, err := json.Marshal(policy.Actions)
	if err != nil {
		return err
	}
	source := policy.Source
	if source == "" {
		source = "native"
	}
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO policy_rules (priority,name,enabled,match_json,actions_json,source,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		policy.Priority, policy.Name, boolToInt(policy.Enabled), string(matchJSON), string(actionsJSON), source, now, now,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	policy.ID = int(id)
	policy.Source = source
	policy.CreatedAt = now
	policy.UpdatedAt = now
	return nil
}

func (s *Store) UpdatePolicy(ctx context.Context, policy *PolicyRule) error {
	matchJSON, err := json.Marshal(policy.Match)
	if err != nil {
		return err
	}
	actionsJSON, err := json.Marshal(policy.Actions)
	if err != nil {
		return err
	}
	source := policy.Source
	if source == "" {
		source = "native"
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE policy_rules
		 SET priority=?,name=?,enabled=?,match_json=?,actions_json=?,source=?,updated_at=?
		 WHERE id=?`,
		policy.Priority, policy.Name, boolToInt(policy.Enabled), string(matchJSON), string(actionsJSON), source, time.Now(), policy.ID,
	)
	return err
}

func (s *Store) DeletePolicy(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM policy_rules WHERE id=?`, id)
	return err
}

func scanPolicyRow(scanner interface {
	Scan(dest ...any) error
}) (PolicyRule, error) {
	var (
		policy      PolicyRule
		enabled     int
		matchJSON   string
		actionsJSON string
	)
	if err := scanner.Scan(
		&policy.ID,
		&policy.Priority,
		&policy.Name,
		&enabled,
		&matchJSON,
		&actionsJSON,
		&policy.Source,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	); err != nil {
		return PolicyRule{}, err
	}
	policy.Enabled = enabled != 0
	if matchJSON != "" {
		if err := json.Unmarshal([]byte(matchJSON), &policy.Match); err != nil {
			return PolicyRule{}, err
		}
	}
	if actionsJSON != "" {
		if err := json.Unmarshal([]byte(actionsJSON), &policy.Actions); err != nil {
			return PolicyRule{}, err
		}
	}
	return policy, nil
}
