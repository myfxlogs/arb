package store

import (
"context"
"fmt"
)

type BrokerAccountRecord struct {
Name     string
Platform int32
Host     string
Server   string
Port     int32
Login    int64
Password string
Company  string
}

func (s *Store) SaveBrokerAccount(ctx context.Context, r BrokerAccountRecord) error {
_, err := s.pool.Exec(ctx,
`INSERT INTO broker_accounts (name, platform, host, server, port, login, password, company)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT (name) DO UPDATE SET
   platform=EXCLUDED.platform, host=EXCLUDED.host, server=EXCLUDED.server,
   port=EXCLUDED.port, login=EXCLUDED.login, password=EXCLUDED.password, company=EXCLUDED.company`,
r.Name, r.Platform, r.Host, r.Server, r.Port, r.Login, r.Password, r.Company)
if err != nil {
return fmt.Errorf("save broker account: %w", err)
}
return nil
}

func (s *Store) DeleteBrokerAccount(ctx context.Context, name string) error {
_, err := s.pool.Exec(ctx, `DELETE FROM broker_accounts WHERE name=$1`, name)
if err != nil {
return fmt.Errorf("delete broker account: %w", err)
}
return nil
}

func (s *Store) ListBrokerAccounts(ctx context.Context) ([]BrokerAccountRecord, error) {
rows, err := s.pool.Query(ctx,
`SELECT name, platform, host, server, port, login, password, company FROM broker_accounts ORDER BY name`)
if err != nil {
return nil, fmt.Errorf("list broker accounts: %w", err)
}
defer rows.Close()
var results []BrokerAccountRecord
for rows.Next() {
var r BrokerAccountRecord
if err := rows.Scan(&r.Name, &r.Platform, &r.Host, &r.Server, &r.Port, &r.Login, &r.Password, &r.Company); err != nil {
return nil, err
}
results = append(results, r)
}
return results, rows.Err()
}
