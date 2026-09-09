package api

import (
	"context"
	"database/sql"
	"time"
)

// upsertCalendarTraffic accumulates raw counter deltas into the node's current
// natural-month row. Unlike billing periods, the calendar month is global, so
// each node keeps exactly one row: when a sample lands in a new UTC calendar
// month the measured aggregates reset to that sample's delta while the counter
// baselines carry over untouched (raw counters are continuous across the
// boundary, so nothing is skipped). A whole sample delta is always attributed
// to the sample's own calendar month; boundary error is bounded by one report
// interval. No history is retained by design - only the running month exists.
// Corrections never apply here: calendar months report measured usage, while
// billing alignment lives on the traffic_monthly snapshot.
func upsertCalendarTraffic(ctx context.Context, tx *sql.Tx, nodeID string, inTotal, outTotal int64, counterSource string, sampleTS time.Time, now int64) error {
	calendarMonth := sampleTS.UTC().Format("2006-01")
	var rowMonth string
	var aggregateIn, aggregateOut int64
	var previousIn, previousOut, lastSampleTS sql.NullInt64
	var previousSource string
	err := tx.QueryRowContext(ctx, `
		SELECT month, in_bytes, out_bytes,
		       last_in_total_bytes, last_out_total_bytes, counter_source, last_sample_ts
		FROM traffic_calendar_monthly
		WHERE node_id = ?
	`, nodeID).Scan(&rowMonth, &aggregateIn, &aggregateOut, &previousIn, &previousOut, &previousSource, &lastSampleTS)
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO traffic_calendar_monthly (node_id, month, in_bytes, out_bytes, last_in_total_bytes, last_out_total_bytes, counter_source, last_sample_ts, updated_at)
			VALUES (?, ?, 0, 0, ?, ?, ?, ?, ?)
		`, nodeID, calendarMonth, inTotal, outTotal, counterSource, sampleTS.Unix(), now)
		return err
	}
	if err != nil {
		return err
	}
	if lastSampleTS.Valid && sampleTS.Unix() <= lastSampleTS.Int64 {
		return nil
	}
	if counterSource != "" && counterSource != previousSource {
		_, err = tx.ExecContext(ctx, `
			UPDATE traffic_calendar_monthly
			SET month = ?, in_bytes = ?, out_bytes = ?,
			    last_in_total_bytes = ?, last_out_total_bytes = ?, counter_source = ?, last_sample_ts = ?, updated_at = ?
			WHERE node_id = ?
		`, calendarMonth, calendarAggregateOnSourceChange(rowMonth, calendarMonth, aggregateIn), calendarAggregateOnSourceChange(rowMonth, calendarMonth, aggregateOut), inTotal, outTotal, counterSource, sampleTS.Unix(), now, nodeID)
		return err
	}
	effectiveSource := counterSource
	if effectiveSource == "" && previousSource != "" {
		effectiveSource = previousSource
	}

	deltaIn := nonNegativeDelta(previousIn, inTotal)
	deltaOut := nonNegativeDelta(previousOut, outTotal)
	if rowMonth != calendarMonth {
		aggregateIn = deltaIn
		aggregateOut = deltaOut
	} else {
		aggregateIn = saturatingAddNonNegativeInt64(aggregateIn, deltaIn)
		aggregateOut = saturatingAddNonNegativeInt64(aggregateOut, deltaOut)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE traffic_calendar_monthly
		SET month = ?,
		    in_bytes = ?,
		    out_bytes = ?,
		    last_in_total_bytes = ?,
		    last_out_total_bytes = ?,
		    counter_source = ?,
		    last_sample_ts = ?,
		    updated_at = ?
		WHERE node_id = ?
	`, calendarMonth, aggregateIn, aggregateOut, inTotal, outTotal, effectiveSource, sampleTS.Unix(), now, nodeID)
	return err
}

// calendarAggregateOnSourceChange keeps the running month's aggregates across
// a counter-source transition, but starts over when the transition coincides
// with a month rollover.
func calendarAggregateOnSourceChange(rowMonth, calendarMonth string, aggregate int64) int64 {
	if rowMonth != calendarMonth {
		return 0
	}
	return aggregate
}
