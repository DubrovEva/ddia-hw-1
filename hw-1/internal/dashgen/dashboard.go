package dashgen

import (
	"errors"
	"fmt"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
	"github.com/grafana/grafana-foundation-sdk/go/units"

	"ddia/hw1/pkg/golden"
)

const (
	varDatacenter = "datacenter"
	varPod        = "pod"
	rateWindow    = "$__rate_interval"
	panelHeight   = 8
	panelWidth    = 6
)

var quantiles = []float64{0.5, 0.75, 0.9, 0.95, 0.99}

type Spec struct {
	UID           string
	Title         string
	DatasourceUID string
	Deployment    string
	Handlers      []golden.HandlerInfo
}

func Build(spec Spec) (dashboard.Dashboard, error) {
	if spec.Deployment == "" {
		return dashboard.Dashboard{}, errors.New("dashgen: deployment is required")
	}
	if len(spec.Handlers) == 0 {
		return dashboard.Dashboard{}, errors.New("dashgen: no handlers to render")
	}

	ds := common.DataSourceRef{Type: ptr("prometheus"), Uid: ptr(spec.DatasourceUID)}
	q := queries{deployment: spec.Deployment}

	b := dashboard.NewDashboardBuilder(spec.Title).
		Uid(spec.UID).
		Tags([]string{"generated", spec.Deployment}).
		Readonly().
		Time("now-15m", "now").
		Refresh("10s").
		Timezone(common.TimeZoneBrowser).
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(variable(ds, varDatacenter, "Datacenter",
			fmt.Sprintf("label_values(%s{%s}, %s)", golden.MetricRequests, q.deploymentMatcher(), golden.LabelDatacenter))).
		WithVariable(variable(ds, varPod, "Pod",
			fmt.Sprintf("label_values(%s{%s, %s}, %s)", golden.MetricRequests, q.deploymentMatcher(), q.datacenterMatcher(), golden.LabelPod)))

	b.WithRow(dashboard.NewRowBuilder("Overview"))
	b.WithPanel(panel(ds, "RPS by datacenter", units.RequestsPerSecond).
		WithTarget(target("A", q.sumRate(golden.MetricRequests, "", golden.LabelDatacenter), "{{datacenter}}")))
	b.WithPanel(panel(ds, "RPS by handler", units.RequestsPerSecond).
		WithTarget(target("A", q.sumRate(golden.MetricRequests, "", golden.LabelHandler), "{{handler}}")))
	b.WithPanel(panel(ds, "p99 latency by handler", units.Seconds).
		WithTarget(target("A", q.quantile(0.99, "", golden.LabelHandler), "{{handler}}")))
	b.WithPanel(panel(ds, "Error ratio by handler", units.PercentUnit).
		WithTarget(target("A", q.errorRatio("", golden.LabelHandler), "{{handler}}")))

	for _, h := range spec.Handlers {
		b.WithRow(dashboard.NewRowBuilder(fmt.Sprintf("%s %s (%s)", h.Method, h.Path, h.ID)))

		b.WithPanel(panel(ds, "RPS", units.RequestsPerSecond).
			WithTarget(target("A", q.sumRate(golden.MetricRequests, h.ID, golden.LabelCode), "{{code}}")))

		latency := panel(ds, "Latency", units.Seconds)
		for i, qv := range quantiles {
			latency.WithTarget(target(refID(i), q.quantile(qv, h.ID), fmt.Sprintf("p%g", qv*100)))
		}
		b.WithPanel(latency)

		b.WithPanel(panel(ds, "Errors by code", units.RequestsPerSecond).
			WithTarget(target("A", q.sumRate(golden.MetricErrors, h.ID, golden.LabelCode), "{{code}}")))
		b.WithPanel(panel(ds, "Error ratio", units.PercentUnit).
			WithTarget(target("A", q.errorRatio(h.ID), "errors")))
	}

	return b.Build()
}

func variable(ds common.DataSourceRef, name, label, query string) *dashboard.QueryVariableBuilder {
	return dashboard.NewQueryVariableBuilder(name).
		Label(label).
		Datasource(ds).
		Query(dashboard.StringOrMap{String: ptr(query)}).
		Definition(query).
		Multi(true).
		IncludeAll(true).
		AllValue(".*").
		Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
		Sort(dashboard.VariableSortAlphabeticalAsc)
}

func panel(ds common.DataSourceRef, title, unit string) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Title(title).
		Datasource(ds).
		Unit(unit).
		Min(0).
		Height(panelHeight).
		Span(panelWidth).
		FillOpacity(10).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom).
			ShowLegend(true))
}

func target(ref, expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().
		RefId(ref).
		Expr(expr).
		LegendFormat(legend).
		Range()
}

func refID(i int) string {
	return string(rune('A' + i))
}

type queries struct {
	deployment string
}

func (q queries) deploymentMatcher() string {
	return fmt.Sprintf("%s=%q", golden.LabelDeployment, q.deployment)
}

func (q queries) datacenterMatcher() string {
	return fmt.Sprintf("%s=~\"$%s\"", golden.LabelDatacenter, varDatacenter)
}

func (q queries) selector(handlerID string) string {
	matchers := []string{
		q.deploymentMatcher(),
		q.datacenterMatcher(),
		fmt.Sprintf("%s=~\"$%s\"", golden.LabelPod, varPod),
	}
	if handlerID != "" {
		matchers = append(matchers, fmt.Sprintf("%s=%q", golden.LabelHandler, handlerID))
	}

	return strings.Join(matchers, ", ")
}

func (q queries) rate(metric, handlerID string) string {
	return fmt.Sprintf("rate(%s{%s}[%s])", metric, q.selector(handlerID), rateWindow)
}

func (q queries) sumRate(metric, handlerID string, by ...string) string {
	return fmt.Sprintf("sum%s (%s)", byClause(by), q.rate(metric, handlerID))
}

func (q queries) quantile(quantile float64, handlerID string, by ...string) string {
	return fmt.Sprintf("histogram_quantile(%g, sum by (%s) (%s))",
		quantile, strings.Join(append(by, "le"), ", "), q.rate(golden.MetricLatency+"_bucket", handlerID))
}

func (q queries) errorRatio(handlerID string, by ...string) string {
	return fmt.Sprintf("(%s or %s) / %s",
		q.sumRate(golden.MetricErrors, handlerID, by...),
		zeroLike(q.sumRate(golden.MetricRequests, handlerID, by...)),
		q.sumRate(golden.MetricRequests, handlerID, by...))
}

func zeroLike(expr string) string {
	return expr + " * 0"
}

func byClause(by []string) string {
	if len(by) == 0 {
		return ""
	}

	return fmt.Sprintf(" by (%s)", strings.Join(by, ", "))
}

func ptr[T any](v T) *T {
	return &v
}
