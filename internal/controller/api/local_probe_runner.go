package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shui1iao/zeno/internal/shared/probe"
)

type ProbeRunner func(ctx context.Context, target ProbeTarget) ([]probe.Sample, error)

var (
	errProbeHTTPSDowngrade = errors.New("probe redirect from https to http refused")
	errProbeRedirectLimit  = errors.New("probe redirect limit exceeded")
	errProbeURLPolicy      = errors.New("probe URL violates transport policy")
)

const (
	localDrawableLatencyCap           = 5 * time.Second
	maxProbeErrorBytes                = 512
	maxProbeErrorBytesPerRound        = 4 << 10
	maxAgentProbeErrorBytesPerRequest = 32 << 10
)

type LocalProbeCollectorOptions struct {
	NodeID      string
	Now         func() time.Time
	ProbeRunner ProbeRunner
}

type LocalProbeCollector struct {
	store       *SQLiteStore
	nodeID      string
	now         func() time.Time
	probeRunner ProbeRunner
}

func NewLocalProbeCollector(store *SQLiteStore, options LocalProbeCollectorOptions) *LocalProbeCollector {
	nodeID := options.NodeID
	if nodeID == "" {
		nodeID = "example-node-a"
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	probeRunner := options.ProbeRunner
	if probeRunner == nil {
		probeRunner = RunLocalProbe
	}
	return &LocalProbeCollector{store: store, nodeID: nodeID, now: now, probeRunner: probeRunner}
}

func (c *LocalProbeCollector) CollectOnce(ctx context.Context) error {
	if c.store == nil {
		return errors.New("local probe collector requires a SQLite store")
	}
	targets, err := c.store.EnabledProbeTargets(ctx, c.nodeID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	ts := c.now().UTC()
	var errs []error
	for _, target := range targets {
		samples, err := c.probeRunner(ctx, target)
		if err != nil {
			errs = append(errs, fmt.Errorf("probe %s: %w", target.ID, err))
		}
		if len(samples) == 0 {
			samples = failedProbeSamples(target.Count, "probe_error")
		}
		if err := c.store.InsertProbeRound(ctx, c.nodeID, target, ts, samples); err != nil {
			errs = append(errs, fmt.Errorf("store %s: %w", target.ID, err))
		}
	}
	return errors.Join(errs...)
}

func RunLocalProbe(ctx context.Context, target ProbeTarget) ([]probe.Sample, error) {
	switch normalizeProbeTargetForExecution(target).Type {
	case "tcping":
		return RunTCPProbe(ctx, target)
	case "ping":
		return RunPingProbe(ctx, target)
	case "http_get":
		return RunHTTPProbe(ctx, target)
	default:
		return nil, fmt.Errorf("unsupported probe target type %q", target.Type)
	}
}

func RunTCPProbe(ctx context.Context, target ProbeTarget) ([]probe.Sample, error) {
	if target.Port == nil {
		return nil, fmt.Errorf("target %s has no TCP port", target.ID)
	}
	target = normalizeProbeTargetForExecution(target)
	count := target.Count
	if count <= 0 {
		count = 1
	}
	timeout := normalizedLocalProbeTimeout(target.TimeoutMS)
	observationTimeout := localLatencyObservationTimeout(timeout)

	address := net.JoinHostPort(target.Address, strconv.Itoa(*target.Port))
	samples := make([]probe.Sample, 0, count)
	for seq := 1; seq <= count; seq++ {
		select {
		case <-ctx.Done():
			return samples, ctx.Err()
		default:
		}

		dialCtx, cancel := context.WithTimeout(ctx, observationTimeout)
		start := time.Now()
		conn, err := (&net.Dialer{Timeout: observationTimeout}).DialContext(dialCtx, "tcp", address)
		elapsedMS := float64(time.Since(start).Microseconds()) / 1000
		cancel()
		if err != nil {
			samples = append(samples, failedMeasuredLocalProbeSample(seq, elapsedMS, classifyProbeError(err)))
			continue
		}
		_ = conn.Close()
		samples = append(samples, measuredLocalProbeSample(seq, elapsedMS, timeout))
	}
	return samples, nil
}

func RunHTTPProbe(ctx context.Context, target ProbeTarget) ([]probe.Sample, error) {
	target = normalizeProbeTargetForExecution(target)
	if target.Type != "http_get" {
		return nil, fmt.Errorf("target %s is not http_get", target.ID)
	}
	initialURL, err := url.ParseRequestURI(strings.TrimSpace(target.Address))
	if err != nil || !validHTTPProbeURL(initialURL) {
		return nil, errProbeURLPolicy
	}
	count := target.Count
	if count <= 0 {
		count = 1
	}
	timeout := normalizedLocalProbeTimeout(target.TimeoutMS)
	observationTimeout := localLatencyObservationTimeout(timeout)
	client := &http.Client{
		Timeout: observationTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			if len(via) >= 10 {
				return errProbeRedirectLimit
			}
			previous := via[len(via)-1].URL
			if previous != nil && strings.EqualFold(previous.Scheme, "https") && !strings.EqualFold(request.URL.Scheme, "https") {
				return errProbeHTTPSDowngrade
			}
			if !validHTTPProbeURL(request.URL) {
				return errProbeURLPolicy
			}
			if !sameHTTPProbeOrigin(previous, request.URL) {
				request.Header = make(http.Header)
				request.Header.Set("User-Agent", "Zeno-Controller")
			}
			return nil
		},
	}
	samples := make([]probe.Sample, 0, count)
	for seq := 1; seq <= count; seq++ {
		select {
		case <-ctx.Done():
			return samples, ctx.Err()
		default:
		}
		requestCtx, cancel := context.WithTimeout(ctx, observationTimeout)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target.Address, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		request.Header.Set("User-Agent", "Zeno-Controller")
		start := time.Now()
		response, err := client.Do(request)
		elapsedMS := float64(time.Since(start).Microseconds()) / 1000
		cancel()
		if err != nil {
			samples = append(samples, failedMeasuredLocalProbeSample(seq, elapsedMS, classifyProbeError(err)))
			continue
		}
		if response.Request == nil || !validHTTPProbeURL(response.Request.URL) {
			_ = response.Body.Close()
			samples = append(samples, failedMeasuredLocalProbeSample(seq, elapsedMS, "url_policy"))
			continue
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 400 {
			samples = append(samples, failedMeasuredLocalProbeSample(seq, elapsedMS, "http_status"))
			continue
		}
		samples = append(samples, measuredLocalProbeSample(seq, elapsedMS, timeout))
	}
	return samples, nil
}

func sameHTTPProbeOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectiveHTTPProbePort(left) == effectiveHTTPProbePort(right)
}

func effectiveHTTPProbePort(value *url.URL) string {
	if value == nil {
		return ""
	}
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func RunPingProbe(ctx context.Context, target ProbeTarget) ([]probe.Sample, error) {
	target = normalizeProbeTargetForExecution(target)
	if target.Type != "ping" {
		return nil, fmt.Errorf("target %s is not ping", target.ID)
	}
	count := target.Count
	if count <= 0 {
		count = 1
	}
	timeout := normalizedLocalProbeTimeout(target.TimeoutMS)
	observationTimeout := localLatencyObservationTimeout(timeout)
	samples := make([]probe.Sample, 0, count)
	for seq := 1; seq <= count; seq++ {
		select {
		case <-ctx.Done():
			return samples, ctx.Err()
		default:
		}
		pingCtx, cancel := context.WithTimeout(ctx, observationTimeout)
		start := time.Now()
		output, err := exec.CommandContext(pingCtx, "ping", "-n", "-c", "1", "-W", pingTimeoutSeconds(observationTimeout), "--", target.Address).CombinedOutput()
		elapsedMS := float64(time.Since(start).Microseconds()) / 1000
		cancel()
		if err != nil {
			samples = append(samples, failedMeasuredLocalProbeSample(seq, elapsedMS, classifyProbeError(err)))
			continue
		}
		latency := parsePingLatencyMS(string(output))
		if latency == nil {
			latencyValue := cappedLocalDrawableLatencyMS(elapsedMS)
			latency = &latencyValue
		}
		if time.Duration(*latency*float64(time.Millisecond)) > timeout {
			samples = append(samples, probe.Sample{Seq: seq, Success: false, Error: "timeout"})
			continue
		}
		capped := cappedLocalDrawableLatencyMS(*latency)
		samples = append(samples, probe.Sample{Seq: seq, Success: true, LatencyMS: &capped})
	}
	return samples, nil
}

func pingTimeoutSeconds(timeout time.Duration) string {
	seconds := int(math.Ceil(timeout.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

func parsePingLatencyMS(output string) *float64 {
	marker := "time="
	index := strings.Index(output, marker)
	if index < 0 {
		return nil
	}
	valueStart := index + len(marker)
	valueEnd := valueStart
	for valueEnd < len(output) {
		c := output[valueEnd]
		if (c >= '0' && c <= '9') || c == '.' {
			valueEnd++
			continue
		}
		break
	}
	if valueEnd == valueStart {
		return nil
	}
	value, err := strconv.ParseFloat(output[valueStart:valueEnd], 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return nil
	}
	return &value
}

func normalizedLocalProbeTimeout(timeoutMS int) time.Duration {
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if timeout <= 0 {
		return time.Second
	}
	return timeout
}

func localLatencyObservationTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return time.Second
	}
	if timeout > localDrawableLatencyCap {
		return localDrawableLatencyCap
	}
	return timeout
}

func measuredLocalProbeSample(seq int, elapsedMS float64, timeout time.Duration) probe.Sample {
	latency := cappedLocalDrawableLatencyMS(elapsedMS)
	if time.Duration(elapsedMS*float64(time.Millisecond)) > timeout {
		return probe.Sample{Seq: seq, Success: false, Error: "timeout"}
	}
	return probe.Sample{Seq: seq, Success: true, LatencyMS: &latency}
}

func failedMeasuredLocalProbeSample(seq int, elapsedMS float64, errText string) probe.Sample {
	return probe.Sample{Seq: seq, Success: false, Error: errText}
}

func cappedLocalDrawableLatencyMS(elapsedMS float64) float64 {
	if elapsedMS < 0 {
		return 0
	}
	capMS := float64(localDrawableLatencyCap / time.Millisecond)
	if elapsedMS > capMS {
		return capMS
	}
	return elapsedMS
}

func failedProbeSamples(count int, errText string) []probe.Sample {
	if count <= 0 {
		count = 1
	}
	samples := make([]probe.Sample, 0, count)
	for seq := 1; seq <= count; seq++ {
		samples = append(samples, probe.Sample{Seq: seq, Success: false, Error: errText})
	}
	return samples
}

func classifyProbeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	if errors.Is(err, errProbeHTTPSDowngrade) {
		return "redirect_downgrade"
	}
	if errors.Is(err, errProbeRedirectLimit) {
		return "redirect_limit"
	}
	if errors.Is(err, errProbeURLPolicy) {
		return "url_policy"
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "timeout") || strings.Contains(message, "deadline") || strings.Contains(message, "i/o timeout") {
		return "timeout"
	}
	if strings.Contains(message, "no such host") {
		return "dns_error"
	}
	return "connect_error"
}
