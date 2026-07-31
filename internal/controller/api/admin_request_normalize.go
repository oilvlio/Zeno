package api

import (
	"math"
	"strings"
)

// patchNormalizer removes the boilerplate shared by every admin PATCH request:
// each optional field has to mark the request as non-empty, normalize its
// value in place, and fail with that endpoint's invalid-write error.
//
// Semantics match the hand-written form it replaces: the first failure wins,
// later fields are left untouched, and a request that supplied no field at all
// is rejected so a no-op PATCH cannot silently succeed.
type patchNormalizer struct {
	changed bool
	err     error
	invalid error
}

func newPatchNormalizer(invalid error) *patchNormalizer {
	return &patchNormalizer{invalid: invalid}
}

// result reports the first failure, or the empty-request error when no field
// was supplied.
func (n *patchNormalizer) result() error {
	if n.err != nil {
		return n.err
	}
	if !n.changed {
		return n.invalid
	}
	return nil
}

// present marks a field as supplied when it needs no normalization.
func (n *patchNormalizer) present(supplied bool) {
	if supplied {
		n.changed = true
	}
}

func (n *patchNormalizer) fail() {
	if n.err == nil {
		n.err = n.invalid
	}
}

// active reports whether a supplied field should still be processed.
func (n *patchNormalizer) active() bool {
	return n.err == nil
}

// text normalizes an optional string field in place.
func (n *patchNormalizer) text(field **string, transform func(string) (string, bool)) {
	if *field == nil {
		return
	}
	n.changed = true
	if !n.active() {
		return
	}
	value, ok := transform(**field)
	if !ok {
		n.fail()
		return
	}
	*field = &value
}

// number normalizes an optional int field in place.
func (n *patchNormalizer) number(field **int, transform func(int) (int, bool)) {
	if *field == nil {
		return
	}
	n.changed = true
	if !n.active() {
		return
	}
	value, ok := transform(**field)
	if !ok {
		n.fail()
		return
	}
	*field = &value
}

// optionalFloat validates a tri-state float, normalizing only a real value.
func (n *patchNormalizer) optionalFloat(field *adminOptionalFloat, transform func(float64) (float64, bool)) {
	if !field.Set {
		return
	}
	n.changed = true
	if !n.active() || !field.Valid {
		return
	}
	value, ok := transform(field.Value)
	if !ok {
		n.fail()
		return
	}
	field.Value = value
}

// optionalInt64 validates a tri-state int64, checking only a real value.
func (n *patchNormalizer) optionalInt64(field *adminOptionalInt64, valid func(int64) bool) {
	if !field.Set {
		return
	}
	n.changed = true
	if !n.active() || !field.Valid {
		return
	}
	if !valid(field.Value) {
		n.fail()
	}
}

// identifiers normalizes a set of ids, rejecting blanks and duplicates so the
// stored selection is unambiguous.
func (n *patchNormalizer) identifiers(field *[]string) {
	if *field == nil {
		return
	}
	n.changed = true
	if !n.active() {
		return
	}
	normalized, ok := normalizeIdentifierSet(*field, strings.TrimSpace)
	if !ok {
		n.fail()
		return
	}
	*field = normalized
}

// identifierSet normalizes an optional id selection held behind a pointer,
// where a nil pointer means "not supplied" and an empty slice means "select
// nothing". The caller supplies the per-id normalizer because scopes use the
// stricter node-id rules rather than a plain trim.
func (n *patchNormalizer) identifierSet(field **[]string, normalize func(string) string) {
	if *field == nil {
		return
	}
	n.changed = true
	if !n.active() {
		return
	}
	normalized, ok := normalizeIdentifierSet(**field, normalize)
	if !ok {
		n.fail()
		return
	}
	*field = &normalized
}

// normalizeIdentifierSet applies normalize to every id, rejecting blanks and
// duplicates. It always returns a non-nil slice so an empty selection stays
// distinguishable from an omitted one.
func normalizeIdentifierSet(ids []string, normalize func(string) string) ([]string, bool) {
	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := normalize(raw)
		if id == "" {
			return nil, false
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, false
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized, true
}

// clearableText normalizes a write-only string field where an explicitly blank
// value means "leave the stored value alone" rather than "clear it". The field
// is reset to nil and deliberately does not mark the request as changed, so a
// PATCH carrying only a blank secret is still rejected as empty.
func (n *patchNormalizer) clearableText(field **string) {
	if *field == nil || !n.active() {
		// An earlier field already failed; leave this one untouched so the
		// request is not partially rewritten before being discarded.
		return
	}
	trimmed := strings.TrimSpace(**field)
	if trimmed == "" {
		*field = nil
		return
	}
	n.changed = true
	*field = &trimmed
}

// Field transforms shared by admin requests. Each reports ok=false to reject
// the value, matching the (value, ok) convention of the existing normalizers.

func trimRequired(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	return trimmed, trimmed != ""
}

func trimOptional(value string) (string, bool) {
	return strings.TrimSpace(value), true
}

func trimUpperMax(limit int) func(string) (string, bool) {
	return func(value string) (string, bool) {
		trimmed := strings.ToUpper(strings.TrimSpace(value))
		return trimmed, len(trimmed) <= limit
	}
}

// fromError adapts a normalizer that reports failure as an error.
func fromError(transform func(string) (string, error)) func(string) (string, bool) {
	return func(value string) (string, bool) {
		normalized, err := transform(value)
		return normalized, err == nil
	}
}

func nonNegativeInt(value int) (int, bool) {
	return value, value >= 0
}

// finiteNonNegativeFloat rejects NaN and infinities alongside negatives, so a
// malformed JSON number cannot reach a threshold comparison.
func finiteNonNegativeFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func nonNegativeInt64(value int64) bool {
	return value >= 0
}

// boundedInt accepts an integer inside an inclusive range.
func boundedInt(minimum, maximum int) func(int) (int, bool) {
	return func(value int) (int, bool) {
		return value, value >= minimum && value <= maximum
	}
}

// float validates an optional float field. Values are range-checked but never
// rewritten, matching the settings contract where the client's exact value is
// stored.
func (n *patchNormalizer) float(field **float64, valid func(float64) bool) {
	if *field == nil {
		return
	}
	n.changed = true
	if !n.active() {
		return
	}
	if !valid(**field) {
		n.fail()
	}
}

// trimRequiredMaxRunes accepts a non-empty string within a rune budget.
// Length is counted in runes so multi-byte titles are not truncated early.
func trimRequiredMaxRunes(limit int) func(string) (string, bool) {
	return func(value string) (string, bool) {
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed != "" && len([]rune(trimmed)) <= limit
	}
}

// trimMaxRunes accepts a possibly empty string within a rune budget.
func trimMaxRunes(limit int) func(string) (string, bool) {
	return func(value string) (string, bool) {
		trimmed := strings.TrimSpace(value)
		return trimmed, len([]rune(trimmed)) <= limit
	}
}

// trimOptionalValid accepts the empty string as "clear this value" and
// otherwise requires the supplied predicate to pass.
func trimOptionalValid(valid func(string) bool) func(string) (string, bool) {
	return func(value string) (string, bool) {
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed == "" || valid(trimmed)
	}
}

// trimLowerValid lower-cases an enum-style value before validating it.
func trimLowerValid(valid func(string) bool) func(string) (string, bool) {
	return func(value string) (string, bool) {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		return trimmed, valid(trimmed)
	}
}

// settingsFloatRange binds an inclusive range for an appearance setting.
func settingsFloatRange(minimum, maximum float64) func(float64) bool {
	return func(value float64) bool {
		return validSettingsFloat(value, minimum, maximum)
	}
}
