package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *SQLiteStore) ensureSchema(ctx context.Context) error {
	// Keep schema and data invariants blocking: Ready must never expose a partly
	// migrated store. Per-stage metrics make future regressions visible without
	// moving today's bounded startup work into an unsafe background lifecycle.
	runStage := func(name string, operation func() error) error {
		_, err := s.measureSchemaStage(ctx, name, operation)
		return err
	}
	ensureColumns := func(stage, table string, columns map[string]string) error {
		return runStage(stage, func() error {
			for column, columnType := range columns {
				if err := s.ensureColumn(ctx, table, column, columnType); err != nil {
					return err
				}
			}
			return nil
		})
	}
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 1000;`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			pending_token_hash TEXT,
			pending_token_expires_at INTEGER,
			-- Deprecated migration-only column. Startup clears every value and
			-- no current write path stores a runtime credential here.
			install_token TEXT,
			status TEXT NOT NULL DEFAULT 'no_data',
			country_code TEXT,
			region TEXT,
			home_probe_target_id TEXT,
			expiry_date TEXT,
			expiry_permanent INTEGER NOT NULL DEFAULT 0,
			billing_cycle TEXT,
			renewal_amount REAL,
			renewal_currency TEXT NOT NULL DEFAULT 'CNY',
			display_order INTEGER NOT NULL DEFAULT 0,
			public_ipv4 TEXT,
			public_ipv6 TEXT,
			billing_mode TEXT NOT NULL DEFAULT 'both',
			monthly_quota_bytes INTEGER,
			monthly_reset_day INTEGER NOT NULL DEFAULT 1,
			billing_traffic_epoch INTEGER NOT NULL DEFAULT 0,
			probe_config_applied_version INTEGER NOT NULL DEFAULT 0,
			probe_config_applied_at INTEGER,
			disabled INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			last_seen_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS agent_enrollment_tokens (
			token_hash TEXT PRIMARY KEY,
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			used_at INTEGER,
			revoked_at INTEGER
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_enrollment_node_created ON agent_enrollment_tokens(node_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_enrollment_expiry ON agent_enrollment_tokens(expires_at);`,
		`CREATE TABLE IF NOT EXISTS host_info (
			node_id TEXT PRIMARY KEY REFERENCES nodes(id),
			hostname TEXT,
			os_name TEXT,
			os_version TEXT,
			kernel TEXT,
			arch TEXT,
			virtualization TEXT,
			cpu_model TEXT,
			cpu_cores INTEGER,
			memory_total_bytes INTEGER,
			disk_total_bytes INTEGER,
			boot_time INTEGER,
			agent_version TEXT,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS state_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT NOT NULL REFERENCES nodes(id),
			sample_id TEXT,
			payload_hash TEXT NOT NULL DEFAULT '',
			received_at INTEGER NOT NULL DEFAULT 0,
			ts INTEGER NOT NULL,
			cpu_percent REAL,
			load1 REAL,
			load5 REAL,
			load15 REAL,
			memory_used_bytes INTEGER,
			memory_total_bytes INTEGER,
			swap_used_bytes INTEGER,
			swap_total_bytes INTEGER,
			disk_used_bytes INTEGER,
			disk_total_bytes INTEGER,
			net_in_total_bytes INTEGER,
			net_out_total_bytes INTEGER,
			net_in_speed_bps REAL,
			net_out_speed_bps REAL,
			process_count INTEGER,
			tcp_connection_count INTEGER,
			udp_connection_count INTEGER,
			uptime_seconds INTEGER
		);`,
		`CREATE INDEX IF NOT EXISTS idx_state_samples_node_ts ON state_samples(node_id, ts);`,
		`CREATE TABLE IF NOT EXISTS traffic_monthly (
			node_id TEXT NOT NULL REFERENCES nodes(id),
			month TEXT NOT NULL,
			billing_epoch INTEGER NOT NULL DEFAULT 0,
			reset_day INTEGER NOT NULL DEFAULT 1,
			billing_mode TEXT NOT NULL DEFAULT 'both',
			in_bytes INTEGER NOT NULL DEFAULT 0,
			out_bytes INTEGER NOT NULL DEFAULT 0,
			billable_bytes INTEGER NOT NULL DEFAULT 0,
			last_in_total_bytes INTEGER,
			last_out_total_bytes INTEGER,
			counter_source TEXT NOT NULL DEFAULT '',
			last_sample_ts INTEGER,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (node_id, month, billing_epoch)
		);`,
		`CREATE TABLE IF NOT EXISTS traffic_lifetime (
			node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
			in_bytes INTEGER NOT NULL DEFAULT 0,
			out_bytes INTEGER NOT NULL DEFAULT 0,
			last_in_total_bytes INTEGER,
			last_out_total_bytes INTEGER,
			counter_source TEXT NOT NULL DEFAULT '',
			last_sample_ts INTEGER,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS exchange_rates (
			currency TEXT PRIMARY KEY,
			cny_rate REAL NOT NULL,
			source_date TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS probe_targets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			address TEXT NOT NULL,
			port INTEGER,
			count INTEGER NOT NULL,
			timeout_ms INTEGER NOT NULL,
			interval_sec INTEGER NOT NULL,
			display_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS node_probe_targets (
			node_id TEXT NOT NULL REFERENCES nodes(id),
			target_id TEXT NOT NULL REFERENCES probe_targets(id),
			enabled INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (node_id, target_id)
		);`,
		`CREATE TABLE IF NOT EXISTS probe_config_meta (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			version INTEGER NOT NULL DEFAULT 1,
			updated_at INTEGER NOT NULL
		);`,
		`INSERT OR IGNORE INTO probe_config_meta (id, version, updated_at) VALUES (1, 1, strftime('%s', 'now'));`,
		`CREATE TABLE IF NOT EXISTS probe_rounds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT NOT NULL REFERENCES nodes(id),
			target_id TEXT NOT NULL REFERENCES probe_targets(id),
			ts INTEGER NOT NULL,
			type TEXT NOT NULL,
			idempotency_key TEXT NOT NULL DEFAULT '',
			agent_round_id TEXT,
			payload_hash TEXT NOT NULL DEFAULT '',
			sent INTEGER NOT NULL,
			received INTEGER NOT NULL,
			loss_percent REAL NOT NULL,
			min_ms REAL,
			avg_ms REAL,
			median_ms REAL,
			max_ms REAL,
			stddev_ms REAL,
			error TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_rounds_node_target_ts ON probe_rounds(node_id, target_id, ts);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_rounds_node_ts ON probe_rounds(node_id, ts);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_rounds_target_ts ON probe_rounds(target_id, ts);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_rounds_ts_target_node ON probe_rounds(ts, target_id, node_id);`,
		`CREATE TABLE IF NOT EXISTS probe_samples (
			round_id INTEGER NOT NULL REFERENCES probe_rounds(id) ON DELETE CASCADE,
			seq INTEGER NOT NULL,
			success INTEGER NOT NULL,
			latency_ms REAL,
			error TEXT,
			PRIMARY KEY (round_id, seq)
		);`,
		`CREATE TABLE IF NOT EXISTS notification_channels (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			destination TEXT NOT NULL,
			credential TEXT NOT NULL,
			delivery_version INTEGER NOT NULL DEFAULT 1,
			destination_fingerprint TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS notification_types (
			event_type TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			metric TEXT NOT NULL,
			comparator TEXT NOT NULL,
			threshold REAL NOT NULL,
			threshold_unit TEXT NOT NULL,
			duration_sec INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			notification_event_type TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rules_sort_order ON alert_rules(sort_order ASC, id ASC);`,
		`CREATE TABLE IF NOT EXISTS alert_rule_node_scopes (
			rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (rule_id, node_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rule_node_scopes_node ON alert_rule_node_scopes(node_id, rule_id);`,
		`CREATE TABLE IF NOT EXISTS alert_rule_states (
			node_id TEXT NOT NULL REFERENCES nodes(id),
			rule_id TEXT NOT NULL REFERENCES alert_rules(id),
			active INTEGER NOT NULL DEFAULT 0,
			first_seen_at INTEGER,
			last_seen_at INTEGER,
			last_value REAL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (node_id, rule_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rule_states_node_active ON alert_rule_states(node_id, active);`,
		`CREATE TABLE IF NOT EXISTS notification_event_marks (
			event_type TEXT NOT NULL,
			node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			mark TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (event_type, node_id, mark)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_notification_event_marks_event_node ON notification_event_marks(event_type, node_id);`,
		`CREATE TABLE IF NOT EXISTS notification_deliveries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			node_id TEXT NOT NULL DEFAULT '',
			node_name TEXT NOT NULL DEFAULT '',
			node_ip TEXT NOT NULL DEFAULT '',
			previous_status TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			event_ts TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			channel_id TEXT NOT NULL,
			channel_name TEXT NOT NULL DEFAULT '',
			channel_version INTEGER NOT NULL DEFAULT 1,
			destination_fingerprint TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			next_attempt_at INTEGER NOT NULL,
			last_error TEXT NOT NULL DEFAULT '',
			lease_until INTEGER NOT NULL DEFAULT 0,
			claim_token TEXT NOT NULL DEFAULT '',
			causal_predecessor_event_id TEXT NOT NULL DEFAULT '',
			superseded_by_event_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			delivered_at INTEGER
		);`,
		`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_pending ON notification_deliveries(state, next_attempt_at, id);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
			token_hash TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS admin_deletion_jobs (
			entity_kind TEXT NOT NULL CHECK (entity_kind IN ('node', 'probe_target')),
			entity_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'running', 'completed')),
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			completed_at INTEGER,
			PRIMARY KEY (entity_kind, entity_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_admin_deletion_jobs_state_updated ON admin_deletion_jobs(state, updated_at, entity_kind, entity_id);`,
	}
	if err := runStage("base-ddl", func() error {
		for _, statement := range statements {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := runStage("notification-channels", func() error { return s.migrateNotificationChannels(ctx) }); err != nil {
		return err
	}
	stateSampleColumns := map[string]string{
		"sample_id":            "TEXT",
		"payload_hash":         "TEXT NOT NULL DEFAULT ''",
		"received_at":          "INTEGER NOT NULL DEFAULT 0",
		"load1":                "REAL",
		"load5":                "REAL",
		"load15":               "REAL",
		"swap_used_bytes":      "INTEGER",
		"swap_total_bytes":     "INTEGER",
		"process_count":        "INTEGER",
		"tcp_connection_count": "INTEGER",
		"udp_connection_count": "INTEGER",
	}
	if err := ensureColumns("state-sample-columns", "state_samples", stateSampleColumns); err != nil {
		return err
	}
	if err := runStage("state-sample-idempotency", func() error { return s.ensureStateSampleIdempotency(ctx) }); err != nil {
		return err
	}
	if err := ensureColumns("traffic-lifetime-columns", "traffic_lifetime", map[string]string{
		"counter_source": "TEXT NOT NULL DEFAULT ''",
	}); err != nil {
		return err
	}
	if err := ensureColumns("probe-round-columns", "probe_rounds", map[string]string{
		"idempotency_key": "TEXT NOT NULL DEFAULT ''",
		"agent_round_id":  "TEXT",
		"payload_hash":    "TEXT NOT NULL DEFAULT ''",
	}); err != nil {
		return err
	}
	if err := runStage("probe-round-idempotency", func() error {
		return s.runValidatedSchemaMigration(ctx, "20260718_probe_round_idempotency_v2", s.probeRoundIdempotencyMigrationCurrent, s.migrateProbeRoundIdempotency)
	}); err != nil {
		return err
	}
	nodeColumns := map[string]string{
		"install_token":                "TEXT",
		"pending_token_hash":           "TEXT",
		"pending_token_expires_at":     "INTEGER",
		"home_probe_target_id":         "TEXT",
		"expiry_date":                  "TEXT",
		"expiry_permanent":             "INTEGER NOT NULL DEFAULT 0",
		"billing_cycle":                "TEXT",
		"renewal_amount":               "REAL",
		"renewal_currency":             "TEXT NOT NULL DEFAULT 'CNY'",
		"billing_mode":                 "TEXT NOT NULL DEFAULT 'both'",
		"monthly_quota_bytes":          "INTEGER",
		"monthly_reset_day":            "INTEGER NOT NULL DEFAULT 1",
		"billing_traffic_epoch":        "INTEGER NOT NULL DEFAULT 0",
		"probe_config_applied_version": "INTEGER NOT NULL DEFAULT 0",
		"probe_config_applied_at":      "INTEGER",
		"disabled":                     "INTEGER NOT NULL DEFAULT 0",
		"display_order":                "INTEGER NOT NULL DEFAULT 0",
		"public_ipv4":                  "TEXT",
		"public_ipv6":                  "TEXT",
		"last_seen_at":                 "INTEGER",
	}
	if err := ensureColumns("node-columns", "nodes", nodeColumns); err != nil {
		return err
	}
	if err := runStage("exchange-rate-default", func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO exchange_rates (currency, cny_rate, source_date, updated_at)
			VALUES ('CNY', 1, '', ?)
			ON CONFLICT(currency) DO UPDATE SET cny_rate = 1
		`, time.Now().UTC().Unix())
		return err
	}); err != nil {
		return err
	}
	if err := runStage("legacy-agent-credentials", func() error { return s.migrateLegacyAgentCredentials(ctx) }); err != nil {
		return err
	}
	notificationChannelColumns := map[string]string{
		"delivery_version":        "INTEGER NOT NULL DEFAULT 1",
		"destination_fingerprint": "TEXT NOT NULL DEFAULT ''",
	}
	if err := ensureColumns("notification-channel-columns", "notification_channels", notificationChannelColumns); err != nil {
		return err
	}
	notificationDeliveryColumns := map[string]string{
		"event_id":                    "TEXT NOT NULL DEFAULT ''",
		"node_ip":                     "TEXT NOT NULL DEFAULT ''",
		"event_ts":                    "TEXT NOT NULL DEFAULT ''",
		"channel_version":             "INTEGER NOT NULL DEFAULT 1",
		"destination_fingerprint":     "TEXT NOT NULL DEFAULT ''",
		"lease_until":                 "INTEGER NOT NULL DEFAULT 0",
		"claim_token":                 "TEXT NOT NULL DEFAULT ''",
		"causal_predecessor_event_id": "TEXT NOT NULL DEFAULT ''",
		"superseded_by_event_id":      "TEXT NOT NULL DEFAULT ''",
	}
	if err := ensureColumns("notification-delivery-columns", "notification_deliveries", notificationDeliveryColumns); err != nil {
		return err
	}
	if err := runStage("notification-routing-bindings", func() error { return s.migrateNotificationRoutingBindings(ctx) }); err != nil {
		return err
	}
	// Existing databases predate the lease columns. Build the claim index only
	// after both columns have been added; otherwise CREATE INDEX aborts startup
	// before the migration can run.
	if err := runStage("notification-delivery-indexes", func() error {
		for _, statement := range []string{
			`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_claim ON notification_deliveries(state, next_attempt_at, lease_until, id)`,
			`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_causal ON notification_deliveries(channel_id, node_id, event_type, id, state)`,
			`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_event_route ON notification_deliveries(channel_id, channel_version, destination_fingerprint, event_id, state)`,
		} {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := runStage("traffic-monthly-schema", func() error { return s.migrateTrafficMonthlySchema(ctx) }); err != nil {
		return err
	}
	if err := ensureColumns("traffic-monthly-columns", "traffic_monthly", map[string]string{
		"counter_source": "TEXT NOT NULL DEFAULT ''",
	}); err != nil {
		return err
	}
	if err := runStage("traffic-aggregate-normalize", func() error { return s.normalizeTrafficAggregateStorage(ctx) }); err != nil {
		return err
	}
	probeTargetColumns := map[string]string{
		"display_order": "INTEGER NOT NULL DEFAULT 0",
	}
	if err := ensureColumns("probe-target-columns", "probe_targets", probeTargetColumns); err != nil {
		return err
	}
	if err := runStage("probe-target-global-enabled", func() error { return s.migrateProbeTargetGlobalEnabled(ctx) }); err != nil {
		return err
	}
	if err := runStage("traffic-last-sample-backfill", func() error {
		_, err := s.db.ExecContext(ctx, `
			UPDATE traffic_monthly
			SET last_sample_ts = COALESCE(
				(SELECT MAX(ss.ts)
				 FROM state_samples ss
				 WHERE ss.node_id = traffic_monthly.node_id
				   AND ss.ts <= CAST(strftime('%s', 'now') AS INTEGER) + 300),
				updated_at
			)
			WHERE last_sample_ts IS NULL
		`)
		return err
	}); err != nil {
		return err
	}
	if err := runStage("traffic-lifetime-backfill", func() error { return s.backfillLifetimeTraffic(ctx) }); err != nil {
		return err
	}
	alertRuleStateColumns := map[string]string{
		"last_value": "REAL",
	}
	if err := ensureColumns("alert-rule-state-columns", "alert_rule_states", alertRuleStateColumns); err != nil {
		return err
	}
	if err := runStage("default-alert-rules", func() error { return s.ensureDefaultAlertRules(ctx) }); err != nil {
		return err
	}
	if err := runStage("retired-notification-config-prune", func() error { return s.pruneRetiredNotificationConfig(ctx) }); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) normalizeTrafficAggregateStorage(ctx context.Context) error {
	// SQLite promotes overflowing INTEGER arithmetic to REAL. Older Controller
	// builds performed monthly accumulation inside SQL and could therefore leave
	// a REAL value or a negative billable value after int64 overflow. Traffic is
	// monotonic and non-negative, so the only safe recovery is saturation.
	const maxInt64 = "9223372036854775807"
	statements := []string{
		`UPDATE traffic_monthly SET
			in_bytes = CASE WHEN typeof(in_bytes) <> 'integer' OR in_bytes < 0 THEN ` + maxInt64 + ` ELSE in_bytes END,
			out_bytes = CASE WHEN typeof(out_bytes) <> 'integer' OR out_bytes < 0 THEN ` + maxInt64 + ` ELSE out_bytes END,
			billable_bytes = CASE WHEN typeof(billable_bytes) <> 'integer' OR billable_bytes < 0 THEN ` + maxInt64 + ` ELSE billable_bytes END
		 WHERE typeof(in_bytes) <> 'integer' OR in_bytes < 0
		    OR typeof(out_bytes) <> 'integer' OR out_bytes < 0
		    OR typeof(billable_bytes) <> 'integer' OR billable_bytes < 0`,
		`UPDATE traffic_lifetime SET
			in_bytes = CASE WHEN typeof(in_bytes) <> 'integer' OR in_bytes < 0 THEN ` + maxInt64 + ` ELSE in_bytes END,
			out_bytes = CASE WHEN typeof(out_bytes) <> 'integer' OR out_bytes < 0 THEN ` + maxInt64 + ` ELSE out_bytes END
		 WHERE typeof(in_bytes) <> 'integer' OR in_bytes < 0
		    OR typeof(out_bytes) <> 'integer' OR out_bytes < 0`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) migrateLegacyAgentCredentials(ctx context.Context) error {
	// v0.6.1 and earlier retained the Agent runtime token in install_token so it
	// could be placed back into generated commands. Preserve token_hash (and thus
	// compatibility with every already-installed v0.3.0 Agent) while removing
	// the directly usable plaintext on the first upgraded startup.
	_, err := s.db.ExecContext(ctx, `
		UPDATE nodes
		SET install_token = NULL
		WHERE install_token IS NOT NULL
	`)
	return err
}

func (s *SQLiteStore) migrateNotificationChannels(ctx context.Context) error {
	hasType, err := s.columnExists(ctx, "notification_channels", "type")
	if err != nil {
		return err
	}
	if !hasType {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE notification_channels_new (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			destination TEXT NOT NULL,
			credential TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO notification_channels_new (id, name, destination, credential, enabled, created_at, updated_at)
		SELECT id, name, destination, credential, enabled, created_at, updated_at
		FROM notification_channels
		WHERE type = 'telegram' OR TRIM(COALESCE(type, '')) = ''
	`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE notification_channels`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE notification_channels_new RENAME TO notification_channels`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *SQLiteStore) ensureColumn(ctx context.Context, table, column, columnType string) error {
	if !safeSQLIdentifier(table) || !safeSQLIdentifier(column) || strings.TrimSpace(columnType) == "" {
		return fmt.Errorf("invalid schema identifier")
	}
	exists, err := s.columnExists(ctx, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType))
	return err
}

func (s *SQLiteStore) migrateProbeTargetGlobalEnabled(ctx context.Context) error {
	exists, err := s.columnExists(ctx, "probe_targets", "enabled")
	if err != nil || !exists {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE probe_targets DROP COLUMN enabled`)
	return err
}

func (s *SQLiteStore) ensureStateSampleIdempotency(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_state_samples_node_sample_id
		ON state_samples(node_id, sample_id)
		WHERE sample_id IS NOT NULL AND sample_id <> ''
	`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_state_samples_node_received
		ON state_samples(node_id, received_at DESC, id DESC)
	`); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) migrateTrafficMonthlySchema(ctx context.Context) error {
	columns, err := s.tableColumns(ctx, "traffic_monthly")
	if err != nil {
		return err
	}
	pkIncludesEpoch, err := s.primaryKeyIncludes(ctx, "traffic_monthly", "billing_epoch")
	if err != nil {
		return err
	}
	requiresRebuild := !pkIncludesEpoch || !columns["billing_epoch"] || !columns["reset_day"] || !columns["billing_mode"] || !columns["last_sample_ts"]
	if !requiresRebuild {
		return nil
	}

	billingEpochExpr := "0"
	if columns["billing_epoch"] {
		billingEpochExpr = "COALESCE(billing_epoch, 0)"
	}
	resetDayExpr := "COALESCE((SELECT n.monthly_reset_day FROM nodes n WHERE n.id = traffic_monthly.node_id), 1)"
	if columns["reset_day"] {
		resetDayExpr = "COALESCE(reset_day, " + resetDayExpr + ")"
	}
	billingModeExpr := "COALESCE((SELECT n.billing_mode FROM nodes n WHERE n.id = traffic_monthly.node_id), 'both')"
	if columns["billing_mode"] {
		billingModeExpr = "COALESCE(NULLIF(TRIM(billing_mode), ''), " + billingModeExpr + ")"
	}
	lastSampleExpr := "NULL"
	if columns["last_sample_ts"] {
		lastSampleExpr = "last_sample_ts"
	}
	counterSourceExpr := "''"
	if columns["counter_source"] {
		counterSourceExpr = "COALESCE(counter_source, '')"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS traffic_monthly_new`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE traffic_monthly_new (
			node_id TEXT NOT NULL REFERENCES nodes(id),
			month TEXT NOT NULL,
			billing_epoch INTEGER NOT NULL DEFAULT 0,
			reset_day INTEGER NOT NULL DEFAULT 1,
			billing_mode TEXT NOT NULL DEFAULT 'both',
			in_bytes INTEGER NOT NULL DEFAULT 0,
			out_bytes INTEGER NOT NULL DEFAULT 0,
			billable_bytes INTEGER NOT NULL DEFAULT 0,
			last_in_total_bytes INTEGER,
			last_out_total_bytes INTEGER,
			counter_source TEXT NOT NULL DEFAULT '',
			last_sample_ts INTEGER,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (node_id, month, billing_epoch)
		)
	`); err != nil {
		return err
	}
	insertSQL := fmt.Sprintf(`
		INSERT OR REPLACE INTO traffic_monthly_new (
			node_id, month, billing_epoch, reset_day, billing_mode,
			in_bytes, out_bytes, billable_bytes, last_in_total_bytes,
			last_out_total_bytes, counter_source, last_sample_ts, updated_at
		)
		SELECT node_id, month, %s, %s, %s,
		       in_bytes, out_bytes, billable_bytes, last_in_total_bytes,
		       last_out_total_bytes, %s, %s, updated_at
		FROM traffic_monthly
	`, billingEpochExpr, resetDayExpr, billingModeExpr, counterSourceExpr, lastSampleExpr)
	if _, err := tx.ExecContext(ctx, insertSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE traffic_monthly`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE traffic_monthly_new RENAME TO traffic_monthly`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *SQLiteStore) backfillLifetimeTraffic(ctx context.Context) error {
	// Existing installations may retain only partial raw history. Seed lifetime
	// totals from the latest valid interface counters and use that same sample as
	// the new baseline rather than trying to reconstruct pruned history.
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO traffic_lifetime (
			node_id, in_bytes, out_bytes, last_in_total_bytes,
			last_out_total_bytes, last_sample_ts, updated_at
		)
		SELECT n.id,
		       latest_sample.net_in_total_bytes,
		       latest_sample.net_out_total_bytes,
		       latest_sample.net_in_total_bytes,
		       latest_sample.net_out_total_bytes,
		       latest_sample.ts,
		       CASE WHEN latest_sample.received_at > 0 THEN latest_sample.received_at ELSE latest_sample.ts END
		FROM nodes n
		JOIN state_samples latest_sample ON latest_sample.id = (
			SELECT latest.id
			FROM state_samples latest
			WHERE latest.node_id = n.id
			  AND latest.net_in_total_bytes IS NOT NULL
			  AND latest.net_out_total_bytes IS NOT NULL
			ORDER BY latest.ts DESC, latest.id DESC
			LIMIT 1
		)
		WHERE NOT EXISTS (
			SELECT 1 FROM traffic_lifetime lifetime WHERE lifetime.node_id = n.id
		)
	`)
	return err
}

func (s *SQLiteStore) tableColumns(ctx context.Context, table string) (map[string]bool, error) {
	if !safeSQLIdentifier(table) {
		return nil, fmt.Errorf("invalid schema identifier")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func (s *SQLiteStore) primaryKeyIncludes(ctx context.Context, table, column string) (bool, error) {
	if !safeSQLIdentifier(table) || !safeSQLIdentifier(column) {
		return false, fmt.Errorf("invalid schema identifier")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column && primaryKey > 0 {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func (s *SQLiteStore) columnExists(ctx context.Context, table, column string) (bool, error) {
	if !safeSQLIdentifier(table) || !safeSQLIdentifier(column) {
		return false, fmt.Errorf("invalid schema identifier")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func safeSQLIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}
