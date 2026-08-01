package api

import "context"

// schemaStageRunner executes ordered migration stages, stopping at the first
// failure.
//
// ensureSchema is a strictly sequential list of stages, but expressing it as
// one `if err := ...; err != nil { return err }` per stage made the function's
// cyclomatic complexity grow with the number of migrations even though there is
// no real branching. Accumulating the first error lets the stage list read as
// data while preserving fail-fast behaviour: once err is set, later stages are
// skipped rather than run against a half-migrated database.
type schemaStageRunner struct {
	ctx   context.Context
	store *SQLiteStore
	err   error
}

func newSchemaStageRunner(ctx context.Context, store *SQLiteStore) *schemaStageRunner {
	return &schemaStageRunner{ctx: ctx, store: store}
}

// run executes one named stage unless an earlier stage already failed.
func (r *schemaStageRunner) run(name string, operation func() error) {
	if r.err != nil {
		return
	}
	if _, err := r.store.measureSchemaStage(r.ctx, name, operation); err != nil {
		r.err = err
	}
}

// columns adds any missing columns to a table as a single named stage.
func (r *schemaStageRunner) columns(name, table string, columns map[string]string) {
	r.run(name, func() error {
		for column, columnType := range columns {
			if err := r.store.ensureColumn(r.ctx, table, column, columnType); err != nil {
				return err
			}
		}
		return nil
	})
}

// result reports the first stage failure, if any.
func (r *schemaStageRunner) result() error {
	return r.err
}
