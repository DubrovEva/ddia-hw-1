package grafana

import (
	"context"
	"net/http"
)

type DashboardResult struct {
	UID     string `json:"uid"`
	URL     string `json:"url"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

func (c *Client) UpsertDashboard(ctx context.Context, dashboard any, message string) (DashboardResult, error) {
	req := map[string]any{
		"dashboard": dashboard,
		"overwrite": true,
		"message":   message,
	}
	var res DashboardResult
	err := c.do(ctx, http.MethodPost, "/api/dashboards/db", req, &res)

	return res, err
}
