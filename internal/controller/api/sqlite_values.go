package api

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

func publicNodeStatus(status string, lastSeenAt sql.NullInt64, now time.Time) string {
	return publicNodeStatusAfter(status, lastSeenAt, now, nodeHeartbeatOfflineAfter)
}

func publicNodeStatusAfter(status string, lastSeenAt sql.NullInt64, now time.Time, offlineAfter time.Duration) string {
	if status == "" {
		status = "no_data"
	}
	offlineAfter = normalizeNodeOfflineAfter(offlineAfter)
	if (status == "online" || status == "warning") && (!lastSeenAt.Valid || now.Sub(time.Unix(lastSeenAt.Int64, 0).UTC()) >= offlineAfter) {
		return "offline"
	}
	return status
}

func normalizeNodeOfflineAfter(offlineAfter time.Duration) time.Duration {
	if offlineAfter <= 0 {
		return nodeHeartbeatOfflineAfter
	}
	return offlineAfter
}

func nodeOfflineAfterFromSeconds(seconds sql.NullInt64) time.Duration {
	if !seconds.Valid || seconds.Int64 <= 0 {
		return nodeHeartbeatOfflineAfter
	}
	return time.Duration(seconds.Int64) * time.Second
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullStringOr(value sql.NullString, fallback string) string {
	if value.Valid {
		return value.String
	}
	return fallback
}

func nullFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	parsed := value.Float64
	return &parsed
}

func monthlyRenewalCostCNY(amount, cnyRate sql.NullFloat64, billingCycle sql.NullString, permanent bool) *float64 {
	months := billingCycleMonths(billingCycle)
	if permanent || months <= 0 || !amount.Valid || amount.Float64 <= 0 || !cnyRate.Valid || cnyRate.Float64 <= 0 {
		return nil
	}
	monthly := math.Round((amount.Float64*cnyRate.Float64/float64(months))*100) / 100
	return &monthly
}

func expiryLabelValue(expiryDate, billingCycle sql.NullString, permanent bool, now time.Time) string {
	if permanent {
		return "永久"
	}
	rawDate := strings.TrimSpace(nullStringOr(expiryDate, ""))
	if rawDate == "" {
		return ""
	}
	cycleMonths := billingCycleMonths(billingCycle)
	if cycleMonths <= 0 {
		return rawDate
	}
	nextDate, ok := nextBillingCycleDate(rawDate, cycleMonths, now)
	if !ok {
		return rawDate
	}
	return formatExpiryDaysLabel(nextDate, now)
}

func billingCycleMonths(value sql.NullString) int {
	if !value.Valid {
		return 0
	}
	cycle := strings.TrimSpace(value.String)
	if cycle == "" {
		return 0
	}
	if strings.Contains(cycle, "五年") || strings.Contains(cycle, "5年") || strings.Contains(cycle, "5 年") {
		return 60
	}
	if strings.Contains(cycle, "三年") || strings.Contains(cycle, "3年") || strings.Contains(cycle, "3 年") {
		return 36
	}
	if strings.Contains(cycle, "两年") || strings.Contains(cycle, "二年") || strings.Contains(cycle, "2年") || strings.Contains(cycle, "2 年") {
		return 24
	}
	if strings.Contains(cycle, "半年") || strings.Contains(cycle, "半 年") {
		return 6
	}
	if strings.Contains(cycle, "季") {
		return 3
	}
	if strings.Contains(cycle, "年") {
		return 12
	}
	if strings.Contains(cycle, "月") {
		return 1
	}
	return 0
}

func nextBillingCycleDate(rawDate string, cycleMonths int, now time.Time) (time.Time, bool) {
	anchorDate, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(rawDate), time.UTC)
	if err != nil || cycleMonths <= 0 {
		return time.Time{}, false
	}
	today := dateOnlyUTC(now)
	anchorDate = dateOnlyUTC(anchorDate)

	yearsApart := today.Year() - anchorDate.Year()
	monthsApart := yearsApart*12 + int(today.Month()-anchorDate.Month())
	offsetMonths := 0
	if monthsApart > 0 {
		offsetMonths = (monthsApart / cycleMonths) * cycleMonths
	}
	nextDate := addMonthsFromAnchorClampedUTC(anchorDate, offsetMonths)
	for nextDate.Before(today) {
		offsetMonths += cycleMonths
		nextDate = addMonthsFromAnchorClampedUTC(anchorDate, offsetMonths)
	}
	for {
		previousOffset := offsetMonths - cycleMonths
		previous := addMonthsFromAnchorClampedUTC(anchorDate, previousOffset)
		if previous.Before(today) {
			break
		}
		offsetMonths = previousOffset
		nextDate = previous
	}
	return nextDate, true
}

func formatExpiryDaysLabel(date, now time.Time) string {
	today := dateOnlyUTC(now)
	due := dateOnlyUTC(date)
	days := int(due.Sub(today).Hours() / 24)
	if days < 0 {
		return "已过期"
	}
	if days == 0 {
		return "今天到期"
	}
	return fmt.Sprintf("余 %d 天", days)
}

func dateOnlyUTC(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func addMonthsClampedUTC(value time.Time, months int) time.Time {
	value = dateOnlyUTC(value)
	return addMonthsFromAnchorClampedUTC(value, months)
}

func addMonthsFromAnchorClampedUTC(anchor time.Time, months int) time.Time {
	anchor = dateOnlyUTC(anchor)
	year, month, day := anchor.Date()
	totalMonths := year*12 + int(month) - 1 + months
	newYear := totalMonths / 12
	newMonth := time.Month(totalMonths%12 + 1)
	if totalMonths < 0 && totalMonths%12 != 0 {
		newYear--
		newMonth = time.Month(totalMonths%12 + 13)
	}
	if maxDay := daysInMonth(newYear, newMonth); day > maxDay {
		day = maxDay
	}
	return time.Date(newYear, newMonth, day, 0, 0, 0, 0, time.UTC)
}

func sqliteBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intPtr(value sql.NullInt64) *float64 {
	if !value.Valid {
		return nil
	}
	converted := float64(value.Int64)
	return &converted
}

func intSQLPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	converted := value.Int64
	return &converted
}

func unixStringPtr(value sql.NullInt64) *string {
	if !value.Valid || value.Int64 <= 0 {
		return nil
	}
	formatted := time.Unix(value.Int64, 0).UTC().Format(time.RFC3339)
	return &formatted
}

func unixStringOr(value sql.NullInt64, fallback time.Time) string {
	if !value.Valid || value.Int64 <= 0 {
		return fallback.UTC().Format(time.RFC3339)
	}
	return time.Unix(value.Int64, 0).UTC().Format(time.RFC3339)
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	converted := value.Float64
	return &converted
}

func nullFloatOr(value sql.NullFloat64, fallback float64) float64 {
	if !value.Valid {
		return fallback
	}
	return value.Float64
}
