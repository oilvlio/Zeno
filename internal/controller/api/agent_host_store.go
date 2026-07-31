package api

import (
	"context"
	"strings"
	"time"
)

func (s *sqliteAgentDomain) UpsertAgentHost(ctx context.Context, nodeID string, host AgentHostRequest) error {
	return s.writes.withAgentWrite(ctx, nodeID, func(ctx context.Context) error {
		return s.upsertAgentHostOnce(ctx, nodeID, host)
	})
}

func (s *sqliteAgentDomain) upsertAgentHostOnce(ctx context.Context, nodeID string, host AgentHostRequest) error {
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { rollbackUnlessCommitted(tx) }()
	if err := lockAgentNodeWriteTx(ctx, tx, nodeID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO host_info (
			node_id, hostname, os_name, os_version, kernel, arch, virtualization,
			cpu_model, cpu_cores, memory_total_bytes, disk_total_bytes, boot_time,
			agent_version, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			hostname = excluded.hostname,
			os_name = excluded.os_name,
			os_version = excluded.os_version,
			kernel = excluded.kernel,
			arch = excluded.arch,
			virtualization = excluded.virtualization,
			cpu_model = excluded.cpu_model,
			cpu_cores = excluded.cpu_cores,
			memory_total_bytes = excluded.memory_total_bytes,
			disk_total_bytes = excluded.disk_total_bytes,
			boot_time = excluded.boot_time,
			agent_version = excluded.agent_version,
			updated_at = excluded.updated_at
	`, nodeID, strings.TrimSpace(host.Hostname), strings.TrimSpace(host.OSName), strings.TrimSpace(host.OSVersion), strings.TrimSpace(host.Kernel), strings.TrimSpace(host.Arch), strings.TrimSpace(host.Virtualization), strings.TrimSpace(host.CPUModel), host.CPUCores, host.MemoryTotalBytes, host.DiskTotalBytes, nullableUnix(host.BootTime), strings.TrimSpace(host.AgentVersion), now); err != nil {
		return err
	}
	publicIPv4 := normalizeAgentPublicIP(host.PublicIPv4, 4)
	publicIPv6 := normalizeAgentPublicIP(host.PublicIPv6, 6)
	countryCode := normalizeAgentCountryCode(host.CountryCode)
	if _, err := tx.ExecContext(ctx, `
		UPDATE nodes
		SET status = CASE WHEN status IN ('warning', 'offline') THEN status ELSE 'online' END,
		    last_seen_at = CASE WHEN last_seen_at IS NULL OR last_seen_at <= ? THEN ? ELSE last_seen_at END,
		    updated_at = CASE WHEN updated_at <= ? THEN ? ELSE updated_at END,
		    public_ipv4 = CASE WHEN ? <> '' THEN ? ELSE public_ipv4 END,
		    public_ipv6 = CASE WHEN ? <> '' THEN ? ELSE public_ipv6 END,
		    country_code = CASE WHEN ? <> '' THEN ? ELSE country_code END
		WHERE id = ? AND disabled = 0
	`, now, now, now, now, publicIPv4, publicIPv4, publicIPv6, publicIPv6, countryCode, countryCode, nodeID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}
