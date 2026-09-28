package index

import (
	"database/sql"
	"errors"
	"time"
)

// requiem: embedding/unreachable-endpoints-remembered
// EndpointDownUntil reports whether endpoint failed to connect recently
// enough to be skipped, and until when.
func (ix *Index) EndpointDownUntil(endpoint string) (time.Time, bool, error) {
	var until string
	err := ix.db.QueryRow(`SELECT down_until FROM endpoint_status WHERE endpoint = ?`, endpoint).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse(timeFormat, until)
	if err != nil || !time.Now().Before(t) {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

// MarkEndpointDown records that endpoint failed to connect.
func (ix *Index) MarkEndpointDown(endpoint string, until time.Time) error {
	_, err := ix.db.Exec(`INSERT INTO endpoint_status (endpoint, down_until) VALUES (?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET down_until = excluded.down_until`, endpoint, until.UTC().Format(timeFormat))
	return err
}

// ClearEndpoint forgets an outage once the endpoint answers again.
func (ix *Index) ClearEndpoint(endpoint string) error {
	_, err := ix.db.Exec(`DELETE FROM endpoint_status WHERE endpoint = ?`, endpoint)
	return err
}
