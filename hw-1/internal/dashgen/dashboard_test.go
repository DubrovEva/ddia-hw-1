package dashgen

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ddia/hw1/pkg/golden"
)

type jsonDashboard struct {
	UID        string `json:"uid"`
	Editable   bool   `json:"editable"`
	Templating struct {
		List []struct {
			Name       string `json:"name"`
			Query      string `json:"query"`
			Multi      bool   `json:"multi"`
			IncludeAll bool   `json:"includeAll"`
		} `json:"list"`
	} `json:"templating"`
	Panels []struct {
		Type    string `json:"type"`
		Title   string `json:"title"`
		Targets []struct {
			Expr   string `json:"expr"`
			Legend string `json:"legendFormat"`
		} `json:"targets"`
	} `json:"panels"`
}

var testSpec = Spec{
	UID:           "items-service",
	Title:         "items-service",
	DatasourceUID: "victoriametrics",
	Deployment:    "items-service",
	Handlers: []golden.HandlerInfo{
		{ID: "items_get", Method: "GET", Path: "/items/{id}"},
		{ID: "items_create", Method: "POST", Path: "/items"},
	},
}

func build(t *testing.T, spec Spec) jsonDashboard {
	t.Helper()

	d, err := Build(spec)
	require.NoError(t, err)
	raw, err := json.Marshal(d)
	require.NoError(t, err)

	var out jsonDashboard
	require.NoError(t, json.Unmarshal(raw, &out))

	return out
}

func TestBuildIsReadonly(t *testing.T) {
	d := build(t, testSpec)

	assert.Equal(t, "items-service", d.UID)
	assert.False(t, d.Editable)
}

func TestBuildVariables(t *testing.T) {
	d := build(t, testSpec)

	require.Len(t, d.Templating.List, 2)
	dc, pod := d.Templating.List[0], d.Templating.List[1]
	assert.Equal(t, "datacenter", dc.Name)
	assert.Equal(t, `label_values(golden_requests_total{deployment="items-service"}, datacenter)`, dc.Query)
	assert.Equal(t, "pod", pod.Name)
	assert.Equal(t, `label_values(golden_requests_total{deployment="items-service", datacenter=~"$datacenter"}, pod)`, pod.Query)
	for _, v := range d.Templating.List {
		assert.True(t, v.Multi, v.Name)
		assert.True(t, v.IncludeAll, v.Name)
	}
}

func TestBuildRowPerHandler(t *testing.T) {
	d := build(t, testSpec)

	var rows []string
	for _, p := range d.Panels {
		if p.Type == "row" {
			rows = append(rows, p.Title)
		}
	}
	assert.Equal(t, []string{"Overview", "GET /items/{id} (items_get)", "POST /items (items_create)"}, rows)
}

func TestBuildQueries(t *testing.T) {
	d := build(t, testSpec)

	var exprs []string
	for _, p := range d.Panels {
		for _, tg := range p.Targets {
			exprs = append(exprs, tg.Expr)
		}
	}
	require.NotEmpty(t, exprs)

	for _, e := range exprs {
		assert.Contains(t, e, `deployment="items-service", datacenter=~"$datacenter", pod=~"$pod"`, e)
		assert.Contains(t, e, "[$__rate_interval]", e)
		assert.True(t, strings.HasPrefix(e, "sum") || strings.HasPrefix(e, "histogram_quantile") || strings.HasPrefix(e, "(sum"), e)
		if strings.HasPrefix(e, "histogram_quantile") {
			assert.Regexp(t, `sum by \((handler, )?le\) \(rate\(golden_request_duration_seconds_bucket\{`, e)
		}
	}
}

func TestBuildLatencyPercentiles(t *testing.T) {
	d := build(t, testSpec)

	for _, p := range d.Panels {
		if p.Title != "Latency" {
			continue
		}
		var legends []string
		for _, tg := range p.Targets {
			legends = append(legends, tg.Legend)
			assert.Contains(t, tg.Expr, `handler="`)
		}
		assert.Equal(t, []string{"p50", "p75", "p90", "p95", "p99"}, legends)
	}
}

func TestBuildValidation(t *testing.T) {
	_, err := Build(Spec{Deployment: "items-service"})
	require.Error(t, err)

	spec := testSpec
	spec.Deployment = ""
	_, err = Build(spec)
	require.Error(t, err)
}
