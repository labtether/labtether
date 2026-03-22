package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

type Point struct {
	TS    int64   `json:"ts"`
	Value float64 `json:"value"`
}

type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPrometheusClient(baseURL string) *PrometheusClient {
	return &PrometheusClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

func (c *PrometheusClient) Enabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *PrometheusClient) QuerySingleValue(ctx context.Context, expr string, at time.Time) (*float64, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("prometheus client is not configured")
	}

	endpoint, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, err
	}

	params := endpoint.Query()
	params.Set("query", expr)
	params.Set("time", strconv.FormatFloat(float64(at.Unix()), 'f', 0, 64))
	endpoint.RawQuery = params.Encode()

	var payload promQueryResponse
	if err := c.fetchJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", payload.Error)
	}
	if len(payload.Data.Result) == 0 || len(payload.Data.Result[0].Value) < 2 {
		return nil, nil
	}

	value, err := parsePromSampleValue(payload.Data.Result[0].Value[1])
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *PrometheusClient) QueryRange(ctx context.Context, expr string, start, end time.Time, step time.Duration) ([]Point, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("prometheus client is not configured")
	}

	endpoint, err := url.Parse(c.baseURL + "/api/v1/query_range")
	if err != nil {
		return nil, err
	}

	params := endpoint.Query()
	params.Set("query", expr)
	params.Set("start", strconv.FormatFloat(float64(start.Unix()), 'f', 0, 64))
	params.Set("end", strconv.FormatFloat(float64(end.Unix()), 'f', 0, 64))
	params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
	endpoint.RawQuery = params.Encode()

	var payload promRangeResponse
	if err := c.fetchJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("prometheus range query failed: %s", payload.Error)
	}
	if len(payload.Data.Result) == 0 {
		return nil, nil
	}

	series := payload.Data.Result[0]
	points := make([]Point, 0, len(series.Values))
	for _, sample := range series.Values {
		if len(sample) < 2 {
			continue
		}
		ts, err := parsePromSampleTS(sample[0])
		if err != nil {
			continue
		}
		value, err := parsePromSampleValue(sample[1])
		if err != nil {
			continue
		}
		points = append(points, Point{TS: ts, Value: value})
	}

	return points, nil
}

func (c *PrometheusClient) fetchJSON(ctx context.Context, endpoint string, out any) error {
	req, err := securityruntime.NewOutboundRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := securityruntime.DoOutboundRequest(c.httpClient, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func parsePromSampleValue(raw any) (float64, error) {
	switch typed := raw.(type) {
	case string:
		return strconv.ParseFloat(typed, 64)
	case float64:
		return typed, nil
	default:
		return 0, fmt.Errorf("unsupported prom value type %T", raw)
	}
}

func parsePromSampleTS(raw any) (int64, error) {
	switch typed := raw.(type) {
	case float64:
		return int64(typed), nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return 0, err
		}
		return int64(parsed), nil
	default:
		return 0, fmt.Errorf("unsupported prom timestamp type %T", raw)
	}
}

type promQueryResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Value []any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type promRangeResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Values [][]any `json:"values"`
		} `json:"result"`
	} `json:"data"`
}
