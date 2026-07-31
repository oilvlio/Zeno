package api

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
)

func TestLatencySeriesBuildersMatchPreRefactor(t *testing.T) {
	finite := latencySeriesFloat(12.345)
	negativeZero := math.Copysign(0, -1)

	nodeCases := []struct {
		name   string
		points []LatencyPoint
	}{
		{name: "nil input", points: nil},
		{name: "empty input", points: []LatencyPoint{}},
		{
			name: "normal interleaved input",
			points: []LatencyPoint{
				{TS: "2026-08-01T00:00:00Z", TargetID: "target-b", TargetName: "B", MedianMS: latencySeriesFloat(12), AvgMS: latencySeriesFloat(12.345), LossPercent: 1.234},
				{TS: "2026-08-01T00:00:01Z", TargetID: "target-a", TargetName: "A", MedianMS: latencySeriesFloat(20), AvgMS: latencySeriesFloat(20.005), LossPercent: 2.005},
				{TS: "2026-07-31T23:59:59Z", TargetID: "target-b", TargetName: "renamed-but-first-name-wins", MedianMS: latencySeriesFloat(13), AvgMS: latencySeriesFloat(13.999), LossPercent: 3.999},
			},
		},
		{
			name: "nil and boundary values",
			points: []LatencyPoint{
				{TS: "1969-12-31T23:59:59.999Z", TargetID: "boundary", TargetName: "Boundary", AvgMS: nil, LossPercent: negativeZero},
				{TS: "1970-01-01T00:00:00Z", TargetID: "boundary", TargetName: "Boundary", AvgMS: finite, LossPercent: 100},
				{TS: "9999-12-31T23:59:59Z", TargetID: "boundary", TargetName: "Boundary", AvgMS: latencySeriesFloat(-0.004), LossPercent: -12.345},
			},
		},
		{
			name: "invalid timestamp and non-finite numbers",
			points: []LatencyPoint{
				{TS: "not-a-time", TargetID: "odd", TargetName: "Odd", AvgMS: latencySeriesFloat(math.NaN()), LossPercent: math.Inf(1)},
				{TS: "", TargetID: "odd", TargetName: "Odd", AvgMS: latencySeriesFloat(math.Inf(-1)), LossPercent: math.NaN()},
			},
		},
	}
	for _, testCase := range nodeCases {
		t.Run("node/"+testCase.name, func(t *testing.T) {
			got := latencySeriesFromPoints(testCase.points)
			want := referenceLatencySeriesFromPoints(testCase.points)
			assertLatencySeriesListEqual(t, got, want)
		})
	}

	serviceCases := []struct {
		name   string
		points []ServiceLatencyPoint
	}{
		{name: "nil input", points: nil},
		{name: "empty input", points: []ServiceLatencyPoint{}},
		{
			name: "normal interleaved input",
			points: []ServiceLatencyPoint{
				{TS: "2026-08-01T00:00:02Z", NodeID: "node-b", NodeName: "B", MedianMS: latencySeriesFloat(32), AvgMS: latencySeriesFloat(32.345), LossPercent: 4.444},
				{TS: "2026-08-01T00:00:01Z", NodeID: "node-a", NodeName: "A", MedianMS: latencySeriesFloat(40), AvgMS: latencySeriesFloat(40.005), LossPercent: 5.005},
				{TS: "2026-08-01T00:00:00Z", NodeID: "node-b", NodeName: "renamed-but-first-name-wins", MedianMS: latencySeriesFloat(33), AvgMS: latencySeriesFloat(33.999), LossPercent: 6.999},
			},
		},
		{
			name: "nil and boundary values",
			points: []ServiceLatencyPoint{
				{TS: "1969-12-31T23:59:59.999Z", NodeID: "boundary", NodeName: "Boundary", AvgMS: nil, LossPercent: negativeZero},
				{TS: "1970-01-01T00:00:00Z", NodeID: "boundary", NodeName: "Boundary", AvgMS: finite, LossPercent: 100},
				{TS: "9999-12-31T23:59:59Z", NodeID: "boundary", NodeName: "Boundary", AvgMS: latencySeriesFloat(-0.004), LossPercent: -12.345},
			},
		},
		{
			name: "invalid timestamp and non-finite numbers",
			points: []ServiceLatencyPoint{
				{TS: "not-a-time", NodeID: "odd", NodeName: "Odd", AvgMS: latencySeriesFloat(math.NaN()), LossPercent: math.Inf(1)},
				{TS: "", NodeID: "odd", NodeName: "Odd", AvgMS: latencySeriesFloat(math.Inf(-1)), LossPercent: math.NaN()},
			},
		},
	}
	for _, testCase := range serviceCases {
		t.Run("service/"+testCase.name, func(t *testing.T) {
			got := serviceLatencySeriesFromPoints(testCase.points)
			want := referenceServiceLatencySeriesFromPoints(testCase.points)
			assertServiceLatencySeriesListEqual(t, got, want)
		})
	}
}

func TestLatencySeriesJSONMatchesPreRefactor(t *testing.T) {
	nodeResponses := []struct {
		name     string
		response LatencyResponse
	}{
		{name: "nil points", response: LatencyResponse{NodeID: "node-nil", Range: "1h"}},
		{name: "empty points", response: LatencyResponse{NodeID: "node-empty", Range: "1d", Points: []LatencyPoint{}}},
		{
			name: "unordered points with nil value",
			response: LatencyResponse{NodeID: "node", Range: "7d", Points: []LatencyPoint{
				{TS: "2026-08-01T00:00:02Z", TargetID: "b", TargetName: "B", AvgMS: nil, LossPercent: 1.236},
				{TS: "2026-08-01T00:00:01Z", TargetID: "a", TargetName: "A", AvgMS: latencySeriesFloat(2.345), LossPercent: 2.345},
				{TS: "2026-08-01T00:00:00Z", TargetID: "b", TargetName: "B", AvgMS: latencySeriesFloat(3.456), LossPercent: 3.456},
			}},
		},
	}
	for _, testCase := range nodeResponses {
		t.Run("marshal/node/"+testCase.name, func(t *testing.T) {
			got, gotErr := json.Marshal(testCase.response)
			want, wantErr := referenceMarshalLatencyResponse(testCase.response)
			assertJSONResultEqual(t, got, gotErr, want, wantErr)
		})
	}

	serviceResponses := []struct {
		name     string
		response ServiceTargetLatencyResponse
	}{
		{name: "nil points", response: ServiceTargetLatencyResponse{Target: ServiceTarget{ID: "target-nil"}, Range: "1h"}},
		{name: "empty points", response: ServiceTargetLatencyResponse{Target: ServiceTarget{ID: "target-empty"}, Range: "1d", Points: []ServiceLatencyPoint{}}},
		{
			name: "unordered points with nil value",
			response: ServiceTargetLatencyResponse{Target: ServiceTarget{ID: "target"}, Range: "30d", Points: []ServiceLatencyPoint{
				{TS: "2026-08-01T00:00:02Z", NodeID: "b", NodeName: "B", AvgMS: nil, LossPercent: 1.236},
				{TS: "2026-08-01T00:00:01Z", NodeID: "a", NodeName: "A", AvgMS: latencySeriesFloat(2.345), LossPercent: 2.345},
				{TS: "2026-08-01T00:00:00Z", NodeID: "b", NodeName: "B", AvgMS: latencySeriesFloat(3.456), LossPercent: 3.456},
			}},
		},
	}
	for _, testCase := range serviceResponses {
		t.Run("marshal/service/"+testCase.name, func(t *testing.T) {
			got, gotErr := json.Marshal(testCase.response)
			want, wantErr := referenceMarshalServiceLatencyResponse(testCase.response)
			assertJSONResultEqual(t, got, gotErr, want, wantErr)
		})
	}

	t.Run("marshal errors on non-finite values", func(t *testing.T) {
		nodeResponse := LatencyResponse{Points: []LatencyPoint{{TargetID: "odd", AvgMS: latencySeriesFloat(math.Inf(1))}}}
		got, gotErr := json.Marshal(nodeResponse)
		want, wantErr := referenceMarshalLatencyResponse(nodeResponse)
		assertJSONResultEqual(t, got, gotErr, want, wantErr)

		serviceResponse := ServiceTargetLatencyResponse{Points: []ServiceLatencyPoint{{NodeID: "odd", LossPercent: math.NaN()}}}
		got, gotErr = json.Marshal(serviceResponse)
		want, wantErr = referenceMarshalServiceLatencyResponse(serviceResponse)
		assertJSONResultEqual(t, got, gotErr, want, wantErr)
	})
}

func TestLatencySeriesUnmarshalMatchesPreRefactor(t *testing.T) {
	nodePayloads := []struct {
		name string
		data string
	}{
		{name: "invalid json", data: `{"node_id":`},
		{name: "series omitted keeps nil points", data: `{"node_id":"node","range":"1h"}`},
		{name: "empty series creates empty points", data: `{"node_id":"node","range":"1h","series":[]}`},
		{name: "legacy points win", data: `{"node_id":"node","range":"1h","points":[{"ts":"2026-08-01T00:00:00Z","target_id":"legacy","target_name":"Legacy","median_ms":null,"avg_ms":1.25,"loss_percent":2.5}],"series":[{"target_id":"ignored","created_at":[1]}]}`},
		{name: "shared and own timelines with missing values", data: `{"node_id":"node","range":"7d","created_at":[0,-1,1000],"series":[{"target_id":"shared","target_name":"Shared","median_ms":[null,2.5],"avg_ms":[1.5],"loss_percent":[3]},{"target_id":"own","target_name":"Own","created_at":[2000],"median_ms":[4.5],"avg_ms":[5.5],"loss_percent":[6.5]}]}`},
	}
	for _, testCase := range nodePayloads {
		t.Run("node/"+testCase.name, func(t *testing.T) {
			var got LatencyResponse
			gotErr := json.Unmarshal([]byte(testCase.data), &got)
			want, wantErr := referenceUnmarshalLatencyResponse([]byte(testCase.data))
			assertErrorEqual(t, gotErr, wantErr)
			assertLatencyResponseEqual(t, got, want)
		})
	}

	servicePayloads := []struct {
		name string
		data string
	}{
		{name: "invalid json", data: `{"target":`},
		{name: "series omitted keeps nil points", data: `{"target":{"id":"target"},"range":"1h"}`},
		{name: "empty series creates empty points", data: `{"target":{"id":"target"},"range":"1h","series":[]}`},
		{name: "legacy points win", data: `{"target":{"id":"target"},"range":"1h","points":[{"ts":"2026-08-01T00:00:00Z","node_id":"legacy","node_name":"Legacy","median_ms":null,"avg_ms":1.25,"loss_percent":2.5}],"series":[{"node_id":"ignored","created_at":[1]}]}`},
		{name: "shared and own timelines with missing values", data: `{"target":{"id":"target","name":"Target"},"range":"30d","created_at":[0,-1,1000],"series":[{"node_id":"shared","node_name":"Shared","median_ms":[null,2.5],"avg_ms":[1.5],"loss_percent":[3]},{"node_id":"own","node_name":"Own","created_at":[2000],"median_ms":[4.5],"avg_ms":[5.5],"loss_percent":[6.5]}]}`},
	}
	for _, testCase := range servicePayloads {
		t.Run("service/"+testCase.name, func(t *testing.T) {
			var got ServiceTargetLatencyResponse
			gotErr := json.Unmarshal([]byte(testCase.data), &got)
			want, wantErr := referenceUnmarshalServiceLatencyResponse([]byte(testCase.data))
			assertErrorEqual(t, gotErr, wantErr)
			assertServiceLatencyResponseEqual(t, got, want)
		})
	}
}

func referenceLatencySeriesFromPoints(points []LatencyPoint) []LatencySeries {
	order := make([]string, 0)
	byTarget := make(map[string]*LatencySeries)
	for _, point := range points {
		series := byTarget[point.TargetID]
		if series == nil {
			series = &LatencySeries{TargetID: point.TargetID, TargetName: point.TargetName}
			byTarget[point.TargetID] = series
			order = append(order, point.TargetID)
		}
		series.CreatedAt = append(series.CreatedAt, latencyTimestampMillis(point.TS))
		series.AvgMS = append(series.AvgMS, compactLatencyValue(point.AvgMS))
		series.LossPercent = append(series.LossPercent, compactLatencyNumber(point.LossPercent))
	}
	seriesList := make([]LatencySeries, 0, len(order))
	for _, targetID := range order {
		seriesList = append(seriesList, *byTarget[targetID])
	}
	return seriesList
}

func referenceServiceLatencySeriesFromPoints(points []ServiceLatencyPoint) []ServiceLatencySeries {
	order := make([]string, 0)
	byNode := make(map[string]*ServiceLatencySeries)
	for _, point := range points {
		series := byNode[point.NodeID]
		if series == nil {
			series = &ServiceLatencySeries{NodeID: point.NodeID, NodeName: point.NodeName}
			byNode[point.NodeID] = series
			order = append(order, point.NodeID)
		}
		series.CreatedAt = append(series.CreatedAt, latencyTimestampMillis(point.TS))
		series.AvgMS = append(series.AvgMS, compactLatencyValue(point.AvgMS))
		series.LossPercent = append(series.LossPercent, compactLatencyNumber(point.LossPercent))
	}
	seriesList := make([]ServiceLatencySeries, 0, len(order))
	for _, nodeID := range order {
		seriesList = append(seriesList, *byNode[nodeID])
	}
	return seriesList
}

func referenceLatencyPointsFromSeries(seriesList []LatencySeries, sharedCreatedAt []int64) []LatencyPoint {
	points := make([]LatencyPoint, 0)
	for _, series := range seriesList {
		createdAtValues := series.CreatedAt
		if len(createdAtValues) == 0 {
			createdAtValues = sharedCreatedAt
		}
		for index, createdAt := range createdAtValues {
			points = append(points, LatencyPoint{
				TS:          latencyTimestampString(createdAt),
				TargetID:    series.TargetID,
				TargetName:  series.TargetName,
				MedianMS:    floatSliceValue(series.MedianMS, index),
				AvgMS:       floatSliceValue(series.AvgMS, index),
				LossPercent: floatValue(series.LossPercent, index),
			})
		}
	}
	return points
}

func referenceServiceLatencyPointsFromSeries(seriesList []ServiceLatencySeries, sharedCreatedAt []int64) []ServiceLatencyPoint {
	points := make([]ServiceLatencyPoint, 0)
	for _, series := range seriesList {
		createdAtValues := series.CreatedAt
		if len(createdAtValues) == 0 {
			createdAtValues = sharedCreatedAt
		}
		for index, createdAt := range createdAtValues {
			points = append(points, ServiceLatencyPoint{
				TS:          latencyTimestampString(createdAt),
				NodeID:      series.NodeID,
				NodeName:    series.NodeName,
				MedianMS:    floatSliceValue(series.MedianMS, index),
				AvgMS:       floatSliceValue(series.AvgMS, index),
				LossPercent: floatValue(series.LossPercent, index),
			})
		}
	}
	return points
}

func referenceMarshalLatencyResponse(response LatencyResponse) ([]byte, error) {
	type latencyResponseJSON struct {
		NodeID          string          `json:"node_id"`
		Range           string          `json:"range"`
		SharedCreatedAt []int64         `json:"created_at,omitempty"`
		Series          []LatencySeries `json:"series"`
	}
	series := referenceLatencySeriesFromPoints(response.Points)
	shared := sharedLatencyCreatedAt(series)
	if len(shared) > 0 {
		for index := range series {
			series[index].CreatedAt = nil
		}
	}
	return json.Marshal(latencyResponseJSON{NodeID: response.NodeID, Range: response.Range, SharedCreatedAt: shared, Series: series})
}

func referenceMarshalServiceLatencyResponse(response ServiceTargetLatencyResponse) ([]byte, error) {
	type serviceLatencyResponseJSON struct {
		Target          ServiceTarget          `json:"target"`
		Range           string                 `json:"range"`
		SharedCreatedAt []int64                `json:"created_at,omitempty"`
		Series          []ServiceLatencySeries `json:"series"`
	}
	series := referenceServiceLatencySeriesFromPoints(response.Points)
	shared := sharedServiceLatencyCreatedAt(series)
	if len(shared) > 0 {
		for index := range series {
			series[index].CreatedAt = nil
		}
	}
	return json.Marshal(serviceLatencyResponseJSON{Target: response.Target, Range: response.Range, SharedCreatedAt: shared, Series: series})
}

func referenceUnmarshalLatencyResponse(data []byte) (LatencyResponse, error) {
	type latencyResponseJSON struct {
		NodeID          string          `json:"node_id"`
		Range           string          `json:"range"`
		SharedCreatedAt []int64         `json:"created_at"`
		Points          []LatencyPoint  `json:"points"`
		Series          []LatencySeries `json:"series"`
	}
	var raw latencyResponseJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return LatencyResponse{}, err
	}
	response := LatencyResponse{NodeID: raw.NodeID, Range: raw.Range}
	if len(raw.Points) > 0 || raw.Series == nil {
		response.Points = raw.Points
		return response, nil
	}
	response.Points = referenceLatencyPointsFromSeries(raw.Series, raw.SharedCreatedAt)
	return response, nil
}

func referenceUnmarshalServiceLatencyResponse(data []byte) (ServiceTargetLatencyResponse, error) {
	type serviceLatencyResponseJSON struct {
		Target          ServiceTarget          `json:"target"`
		Range           string                 `json:"range"`
		SharedCreatedAt []int64                `json:"created_at"`
		Points          []ServiceLatencyPoint  `json:"points"`
		Series          []ServiceLatencySeries `json:"series"`
	}
	var raw serviceLatencyResponseJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return ServiceTargetLatencyResponse{}, err
	}
	response := ServiceTargetLatencyResponse{Target: raw.Target, Range: raw.Range}
	if len(raw.Points) > 0 || raw.Series == nil {
		response.Points = raw.Points
		return response, nil
	}
	response.Points = referenceServiceLatencyPointsFromSeries(raw.Series, raw.SharedCreatedAt)
	return response, nil
}

func assertLatencySeriesListEqual(t *testing.T, got, want []LatencySeries) {
	t.Helper()
	if (got == nil) != (want == nil) || len(got) != len(want) {
		t.Fatalf("series shape mismatch: got nil=%t len=%d, want nil=%t len=%d", got == nil, len(got), want == nil, len(want))
	}
	for index := range want {
		if got[index].TargetID != want[index].TargetID || got[index].TargetName != want[index].TargetName {
			t.Fatalf("series[%d] identity mismatch: got %#v, want %#v", index, got[index], want[index])
		}
		assertInt64SliceEqual(t, "created_at", got[index].CreatedAt, want[index].CreatedAt)
		assertFloatPointerSliceEqual(t, "median_ms", got[index].MedianMS, want[index].MedianMS)
		assertFloatPointerSliceEqual(t, "avg_ms", got[index].AvgMS, want[index].AvgMS)
		assertFloatSliceEqual(t, "loss_percent", got[index].LossPercent, want[index].LossPercent)
	}
}

func assertServiceLatencySeriesListEqual(t *testing.T, got, want []ServiceLatencySeries) {
	t.Helper()
	if (got == nil) != (want == nil) || len(got) != len(want) {
		t.Fatalf("series shape mismatch: got nil=%t len=%d, want nil=%t len=%d", got == nil, len(got), want == nil, len(want))
	}
	for index := range want {
		if got[index].NodeID != want[index].NodeID || got[index].NodeName != want[index].NodeName {
			t.Fatalf("series[%d] identity mismatch: got %#v, want %#v", index, got[index], want[index])
		}
		assertInt64SliceEqual(t, "created_at", got[index].CreatedAt, want[index].CreatedAt)
		assertFloatPointerSliceEqual(t, "median_ms", got[index].MedianMS, want[index].MedianMS)
		assertFloatPointerSliceEqual(t, "avg_ms", got[index].AvgMS, want[index].AvgMS)
		assertFloatSliceEqual(t, "loss_percent", got[index].LossPercent, want[index].LossPercent)
	}
}

func assertLatencyResponseEqual(t *testing.T, got, want LatencyResponse) {
	t.Helper()
	if got.NodeID != want.NodeID || got.Range != want.Range {
		t.Fatalf("response metadata mismatch: got %#v, want %#v", got, want)
	}
	if (got.Points == nil) != (want.Points == nil) || len(got.Points) != len(want.Points) {
		t.Fatalf("points shape mismatch: got nil=%t len=%d, want nil=%t len=%d", got.Points == nil, len(got.Points), want.Points == nil, len(want.Points))
	}
	for index := range want.Points {
		gotPoint, wantPoint := got.Points[index], want.Points[index]
		if gotPoint.TS != wantPoint.TS || gotPoint.TargetID != wantPoint.TargetID || gotPoint.TargetName != wantPoint.TargetName {
			t.Fatalf("point[%d] metadata mismatch: got %#v, want %#v", index, gotPoint, wantPoint)
		}
		assertFloatPointerEqual(t, "median_ms", gotPoint.MedianMS, wantPoint.MedianMS)
		assertFloatPointerEqual(t, "avg_ms", gotPoint.AvgMS, wantPoint.AvgMS)
		assertFloatEqual(t, "loss_percent", gotPoint.LossPercent, wantPoint.LossPercent)
	}
}

func assertServiceLatencyResponseEqual(t *testing.T, got, want ServiceTargetLatencyResponse) {
	t.Helper()
	if got.Target != want.Target || got.Range != want.Range {
		t.Fatalf("response metadata mismatch: got %#v, want %#v", got, want)
	}
	if (got.Points == nil) != (want.Points == nil) || len(got.Points) != len(want.Points) {
		t.Fatalf("points shape mismatch: got nil=%t len=%d, want nil=%t len=%d", got.Points == nil, len(got.Points), want.Points == nil, len(want.Points))
	}
	for index := range want.Points {
		gotPoint, wantPoint := got.Points[index], want.Points[index]
		if gotPoint.TS != wantPoint.TS || gotPoint.NodeID != wantPoint.NodeID || gotPoint.NodeName != wantPoint.NodeName {
			t.Fatalf("point[%d] metadata mismatch: got %#v, want %#v", index, gotPoint, wantPoint)
		}
		assertFloatPointerEqual(t, "median_ms", gotPoint.MedianMS, wantPoint.MedianMS)
		assertFloatPointerEqual(t, "avg_ms", gotPoint.AvgMS, wantPoint.AvgMS)
		assertFloatEqual(t, "loss_percent", gotPoint.LossPercent, wantPoint.LossPercent)
	}
}

func assertJSONResultEqual(t *testing.T, got []byte, gotErr error, want []byte, wantErr error) {
	t.Helper()
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("JSON error mismatch: got %v, want %v", gotErr, wantErr)
	}
	if gotErr != nil {
		return
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func assertErrorEqual(t *testing.T, got, want error) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("error mismatch: got %v, want %v", got, want)
	}
	if got != nil && got.Error() != want.Error() {
		t.Fatalf("error text mismatch: got %q, want %q", got.Error(), want.Error())
	}
}

func assertInt64SliceEqual(t *testing.T, field string, got, want []int64) {
	t.Helper()
	if (got == nil) != (want == nil) || len(got) != len(want) {
		t.Fatalf("%s shape mismatch: got %#v, want %#v", field, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s[%d] mismatch: got %d, want %d", field, index, got[index], want[index])
		}
	}
}

func assertFloatPointerSliceEqual(t *testing.T, field string, got, want []*float64) {
	t.Helper()
	if (got == nil) != (want == nil) || len(got) != len(want) {
		t.Fatalf("%s shape mismatch: got %#v, want %#v", field, got, want)
	}
	for index := range want {
		assertFloatPointerEqual(t, field, got[index], want[index])
	}
}

func assertFloatSliceEqual(t *testing.T, field string, got, want []float64) {
	t.Helper()
	if (got == nil) != (want == nil) || len(got) != len(want) {
		t.Fatalf("%s shape mismatch: got %#v, want %#v", field, got, want)
	}
	for index := range want {
		assertFloatEqual(t, field, got[index], want[index])
	}
}

func assertFloatPointerEqual(t *testing.T, field string, got, want *float64) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("%s nil mismatch: got %v, want %v", field, got, want)
	}
	if got != nil {
		assertFloatEqual(t, field, *got, *want)
	}
}

func assertFloatEqual(t *testing.T, field string, got, want float64) {
	t.Helper()
	if math.IsNaN(want) {
		if !math.IsNaN(got) {
			t.Fatalf("%s mismatch: got %v, want NaN", field, got)
		}
		return
	}
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("%s mismatch: got %v (%x), want %v (%x)", field, got, math.Float64bits(got), want, math.Float64bits(want))
	}
}

func latencySeriesFloat(value float64) *float64 {
	return &value
}
