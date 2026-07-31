package api

import (
	"database/sql"
	"strings"
)

// sqlPatch accumulates the SET clause of a partial UPDATE. Optional request
// fields describe themselves through the add* helpers instead of each store
// method repeating the same "if field != nil { append clause; append arg }"
// block, which is what made the admin update paths large and easy to get
// subtly wrong.
//
// A patch is empty when no field was supplied; callers use empty() to decide
// whether an UPDATE is needed at all.
type sqlPatch struct {
	sets []string
	args []any
}

func newSQLPatch(capacity int) *sqlPatch {
	return &sqlPatch{
		sets: make([]string, 0, capacity),
		args: make([]any, 0, capacity+1),
	}
}

// set records a column assignment with its bound argument.
func (p *sqlPatch) set(column string, value any) {
	p.sets = append(p.sets, column+" = ?")
	p.args = append(p.args, value)
}

// setExpression records an assignment whose right-hand side is SQL rather
// than a single placeholder, along with that expression's arguments.
func (p *sqlPatch) setExpression(assignment string, args ...any) {
	p.sets = append(p.sets, assignment)
	p.args = append(p.args, args...)
}

// addString assigns a plain string when supplied.
func (p *sqlPatch) addString(column string, value *string) {
	if value != nil {
		p.set(column, *value)
	}
}

// addNullableString assigns a string, storing NULL for the empty value so
// "cleared" and "never set" share one representation in SQLite.
func (p *sqlPatch) addNullableString(column string, value *string) {
	if value != nil {
		p.set(column, nullIfEmpty(*value))
	}
}

// addBoolInt assigns a boolean using SQLite's integer encoding.
func (p *sqlPatch) addBoolInt(column string, value *bool) {
	if value != nil {
		p.set(column, sqliteBoolInt(*value))
	}
}

// addInt assigns an integer when supplied.
func (p *sqlPatch) addInt(column string, value *int) {
	if value != nil {
		p.set(column, *value)
	}
}

// addOptionalFloat assigns a tri-state float: absent, explicit null, or value.
func (p *sqlPatch) addOptionalFloat(column string, value adminOptionalFloat) {
	if !value.Set {
		return
	}
	if value.Valid {
		p.set(column, value.Value)
		return
	}
	p.set(column, nil)
}

// addOptionalInt64 assigns a tri-state int64: absent, explicit null, or value.
func (p *sqlPatch) addOptionalInt64(column string, value adminOptionalInt64) {
	if !value.Set {
		return
	}
	if value.Valid {
		p.set(column, value.Value)
		return
	}
	p.set(column, nil)
}

func (p *sqlPatch) empty() bool {
	return len(p.sets) == 0
}

// updateStatement renders "UPDATE <table> SET ... WHERE <where>" and returns
// the full argument list. stamp columns (such as updated_at) are appended last
// so every write refreshes them, and whereArgs follow the SET arguments to
// match placeholder order.
func (p *sqlPatch) updateStatement(table, where string, whereArgs ...any) (string, []any) {
	args := make([]any, 0, len(p.args)+len(whereArgs))
	args = append(args, p.args...)
	args = append(args, whereArgs...)
	return "UPDATE " + table + " SET " + strings.Join(p.sets, ", ") + " WHERE " + where, args
}

// billingEpochRebase tracks which billing fields changed so a traffic epoch
// bump can be folded into the same UPDATE. The epoch only advances when a
// value actually differs from what is stored, which is why the conditions are
// evaluated in SQL rather than in Go.
type billingEpochRebase struct {
	conditions []string
	args       []any
}

func (r *billingEpochRebase) add(condition string, arg any) {
	r.conditions = append(r.conditions, condition)
	r.args = append(r.args, arg)
}

func (r *billingEpochRebase) empty() bool {
	return len(r.conditions) == 0
}

// applyTo folds the conditional epoch increment into a patch.
func (r *billingEpochRebase) applyTo(patch *sqlPatch) {
	if r.empty() {
		return
	}
	patch.setExpression(
		"billing_traffic_epoch = billing_traffic_epoch + CASE WHEN "+
			strings.Join(r.conditions, " OR ")+" THEN 1 ELSE 0 END",
		r.args...,
	)
}

// buildAdminNodePatch maps an admin node update onto its SET clause. Changing
// billing mode or reset day also rebases the traffic epoch, so accounting does
// not attribute pre-change usage to the new cycle.
func buildAdminNodePatch(update AdminNodeUpdateRequest) *sqlPatch {
	patch := newSQLPatch(16)
	patch.addString("display_name", update.DisplayName)
	patch.addNullableString("country_code", update.CountryCode)
	patch.addNullableString("region", update.Region)
	patch.addNullableString("home_probe_target_id", update.HomeProbeTargetID)
	patch.addNullableString("expiry_date", update.ExpiryDate)
	patch.addBoolInt("expiry_permanent", update.ExpiryPermanent)
	patch.addNullableString("billing_cycle", update.BillingCycle)
	patch.addOptionalFloat("renewal_amount", update.RenewalAmount)
	patch.addString("renewal_currency", update.RenewalCurrency)

	var rebase billingEpochRebase
	if update.BillingMode != nil {
		patch.set("billing_mode", *update.BillingMode)
		rebase.add("COALESCE(billing_mode, '') <> ?", *update.BillingMode)
	}
	if update.MonthlyResetDay != nil {
		patch.set("monthly_reset_day", *update.MonthlyResetDay)
		rebase.add("COALESCE(monthly_reset_day, 1) <> ?", *update.MonthlyResetDay)
	}

	patch.addInt("display_order", update.DisplayOrder)
	patch.addNullableString("public_ipv4", update.PublicIPv4)
	patch.addNullableString("public_ipv6", update.PublicIPv6)
	patch.addOptionalInt64("monthly_quota_bytes", update.MonthlyQuotaBytes)
	patch.addBoolInt("disabled", update.Disabled)
	rebase.applyTo(patch)
	return patch
}

// adminProbeTargetConfig is the stored probe configuration a partial update is
// validated against.
type adminProbeTargetConfig struct {
	targetType  string
	address     string
	port        sql.NullInt64
	count       int
	timeoutMS   int
	intervalSec int
}

// merge overlays the supplied fields of an update onto the stored config,
// producing the configuration that would result from applying the patch.
func (c adminProbeTargetConfig) merge(update AdminProbeTargetUpdateRequest) adminProbeTargetConfig {
	merged := c
	if update.Type != nil {
		merged.targetType = *update.Type
	}
	if update.Address != nil {
		merged.address = *update.Address
	}
	if update.Port.Set {
		merged.port = sql.NullInt64{Valid: update.Port.Valid, Int64: update.Port.Value}
	}
	if update.Count != nil {
		merged.count = *update.Count
	}
	if update.TimeoutMS != nil {
		merged.timeoutMS = *update.TimeoutMS
	}
	if update.IntervalSec != nil {
		merged.intervalSec = *update.IntervalSec
	}
	return merged
}

// probeResourceConfigNeedsValidation reports whether the resulting probe load
// must be re-checked. Touching count, timeout or interval always requires it;
// so does enabling the target on a node, because that adds probe volume even
// when the timing fields are untouched.
func probeResourceConfigNeedsValidation(update AdminProbeTargetUpdateRequest) bool {
	if update.Count != nil || update.TimeoutMS != nil || update.IntervalSec != nil {
		return true
	}
	for _, assignment := range update.Assignments {
		if assignment.Enabled {
			return true
		}
	}
	return false
}

// validateAdminProbeTargetUpdate checks the merged configuration, so a partial
// update can never combine with stored values to form an invalid target.
func validateAdminProbeTargetUpdate(current adminProbeTargetConfig, update AdminProbeTargetUpdateRequest) error {
	merged := current.merge(update)
	if !validAdminProbeTargetForType(merged.targetType, merged.address, merged.port) {
		return errInvalidAdminTargetWrite
	}
	if probeResourceConfigNeedsValidation(update) &&
		!validProbeTargetResourceConfig(merged.count, merged.timeoutMS, merged.intervalSec) {
		return errInvalidAdminTargetWrite
	}
	return nil
}

// buildAdminProbeTargetPatch maps a probe target update onto its SET clause.
func buildAdminProbeTargetPatch(update AdminProbeTargetUpdateRequest) *sqlPatch {
	patch := newSQLPatch(9)
	patch.addString("name", update.Name)
	patch.addString("type", update.Type)
	patch.addString("address", update.Address)
	if update.Port.Set {
		patch.set("port", adminOptionalInt64SQLValue(update.Port))
	}
	patch.addInt("count", update.Count)
	patch.addInt("timeout_ms", update.TimeoutMS)
	patch.addInt("interval_sec", update.IntervalSec)
	patch.addInt("display_order", update.DisplayOrder)
	return patch
}
