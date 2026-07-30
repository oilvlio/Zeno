package api

import (
	"bytes"
	"encoding/json"
	"math"
	"net"
	"strings"
	"time"
)

func normalizeAdminNodeDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil || parsed.Format("2006-01-02") != trimmed {
		return "", errInvalidAdminNodeUpdate
	}
	return trimmed, nil
}

func normalizeAdminNodeShortText(value string, maxRunes int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > maxRunes {
		return "", errInvalidAdminNodeUpdate
	}
	return trimmed, nil
}

func normalizeAdminNodeIP(value string, family int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	parsed := net.ParseIP(trimmed)
	if parsed == nil {
		return "", errInvalidAdminNodeUpdate
	}
	if family == 4 {
		ipv4 := parsed.To4()
		if ipv4 == nil {
			return "", errInvalidAdminNodeUpdate
		}
		return ipv4.String(), nil
	}
	if family == 6 {
		if parsed.To4() != nil || parsed.To16() == nil {
			return "", errInvalidAdminNodeUpdate
		}
		return parsed.String(), nil
	}
	return "", errInvalidAdminNodeUpdate
}

func normalizeAdminNodeBillingMode(value string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		return "both", true
	}
	switch mode {
	case "in", "download", "inbound":
		return "in", true
	case "out", "upload", "outbound":
		return "out", true
	case "both", "sum", "total":
		return "both", true
	case "max", "higher":
		return "max", true
	default:
		return "", false
	}
}

func normalizeAdminNodeMonthlyResetDay(value int) (int, bool) {
	if value == 0 {
		return 1, true
	}
	if value < 1 || value > 31 {
		return 0, false
	}
	return value, true
}

// AdminNodesResponse is the authenticated management view for node inventory.
// It intentionally omits token hashes and other credentials.
type AdminNodesResponse struct {
	Nodes []AdminNode `json:"nodes"`
}

type AdminNodeResponse struct {
	Node AdminNode `json:"node"`
}

type AdminNodeInstallCommandRequest struct {
	ControllerURL string `json:"controller_url,omitempty"`
}

type AdminNodeInstallCommandResponse struct {
	NodeID                  string            `json:"node_id"`
	Command                 string            `json:"command"`
	Commands                map[string]string `json:"commands,omitempty"`
	EnrollmentExpiresAt     string            `json:"enrollment_expires_at"`
	EnrollmentOneTime       bool              `json:"enrollment_one_time"`
	SupersedesPreviousToken bool              `json:"supersedes_previous_enrollment"`
}

type AdminNodeCreateRequest struct {
	ID                string             `json:"id,omitempty"`
	DisplayName       string             `json:"display_name"`
	CountryCode       string             `json:"country_code,omitempty"`
	Region            string             `json:"region,omitempty"`
	ExpiryDate        string             `json:"expiry_date,omitempty"`
	ExpiryPermanent   bool               `json:"expiry_permanent,omitempty"`
	BillingCycle      string             `json:"billing_cycle,omitempty"`
	RenewalAmount     adminOptionalFloat `json:"renewal_amount,omitempty"`
	RenewalCurrency   string             `json:"renewal_currency,omitempty"`
	BillingMode       string             `json:"billing_mode,omitempty"`
	MonthlyResetDay   *int               `json:"monthly_reset_day,omitempty"`
	DisplayOrder      int                `json:"display_order,omitempty"`
	PublicIPv4        string             `json:"public_ipv4,omitempty"`
	PublicIPv6        string             `json:"public_ipv6,omitempty"`
	MonthlyQuotaBytes adminOptionalInt64 `json:"monthly_quota_bytes,omitempty"`
	Disabled          bool               `json:"disabled,omitempty"`
}

func (request *AdminNodeCreateRequest) normalize() error {
	trimmedName := strings.TrimSpace(request.DisplayName)
	if trimmedName == "" {
		return errInvalidAdminNodeCreate
	}
	request.DisplayName = trimmedName
	request.ID = strings.TrimSpace(request.ID)
	request.CountryCode = strings.ToUpper(strings.TrimSpace(request.CountryCode))
	if len(request.CountryCode) > 8 {
		return errInvalidAdminNodeCreate
	}
	request.Region = strings.TrimSpace(request.Region)
	expiryDate, err := normalizeAdminNodeDate(request.ExpiryDate)
	if err != nil {
		return errInvalidAdminNodeCreate
	}
	request.ExpiryDate = expiryDate
	billingCycle, err := normalizeAdminNodeShortText(request.BillingCycle, 64)
	if err != nil {
		return errInvalidAdminNodeCreate
	}
	request.BillingCycle = billingCycle
	if request.RenewalAmount.Set && request.RenewalAmount.Valid {
		amount, ok := normalizeAdminNodeRenewalAmount(request.RenewalAmount.Value)
		if !ok {
			return errInvalidAdminNodeCreate
		}
		request.RenewalAmount.Value = amount
	}
	currency, ok := normalizeAdminNodeRenewalCurrency(request.RenewalCurrency)
	if !ok {
		return errInvalidAdminNodeCreate
	}
	request.RenewalCurrency = currency
	billingMode, ok := normalizeAdminNodeBillingMode(request.BillingMode)
	if !ok {
		return errInvalidAdminNodeCreate
	}
	request.BillingMode = billingMode
	if request.MonthlyResetDay == nil {
		defaultResetDay := 1
		request.MonthlyResetDay = &defaultResetDay
	} else {
		if *request.MonthlyResetDay == 0 {
			return errInvalidAdminNodeCreate
		}
		resetDay, ok := normalizeAdminNodeMonthlyResetDay(*request.MonthlyResetDay)
		if !ok {
			return errInvalidAdminNodeCreate
		}
		request.MonthlyResetDay = &resetDay
	}
	if request.DisplayOrder < 0 {
		return errInvalidAdminNodeCreate
	}
	publicIPv4, err := normalizeAdminNodeIP(request.PublicIPv4, 4)
	if err != nil {
		return errInvalidAdminNodeCreate
	}
	request.PublicIPv4 = publicIPv4
	publicIPv6, err := normalizeAdminNodeIP(request.PublicIPv6, 6)
	if err != nil {
		return errInvalidAdminNodeCreate
	}
	request.PublicIPv6 = publicIPv6
	if request.MonthlyQuotaBytes.Set && request.MonthlyQuotaBytes.Valid && request.MonthlyQuotaBytes.Value < 0 {
		return errInvalidAdminNodeCreate
	}
	return nil
}

type AdminNodeUpdateRequest struct {
	DisplayName       *string            `json:"display_name,omitempty"`
	CountryCode       *string            `json:"country_code,omitempty"`
	Region            *string            `json:"region,omitempty"`
	HomeProbeTargetID *string            `json:"home_probe_target_id,omitempty"`
	ExpiryDate        *string            `json:"expiry_date,omitempty"`
	ExpiryPermanent   *bool              `json:"expiry_permanent,omitempty"`
	BillingCycle      *string            `json:"billing_cycle,omitempty"`
	RenewalAmount     adminOptionalFloat `json:"renewal_amount,omitempty"`
	RenewalCurrency   *string            `json:"renewal_currency,omitempty"`
	BillingMode       *string            `json:"billing_mode,omitempty"`
	MonthlyResetDay   *int               `json:"monthly_reset_day,omitempty"`
	DisplayOrder      *int               `json:"display_order,omitempty"`
	PublicIPv4        *string            `json:"public_ipv4,omitempty"`
	PublicIPv6        *string            `json:"public_ipv6,omitempty"`
	MonthlyQuotaBytes adminOptionalInt64 `json:"monthly_quota_bytes,omitempty"`
	Disabled          *bool              `json:"disabled,omitempty"`
	ProbeTargetIDs    []string           `json:"probe_target_ids,omitempty"`
}

func (request *AdminNodeUpdateRequest) normalize() error {
	changed := false
	if request.DisplayName != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.DisplayName)
		if trimmed == "" {
			return errInvalidAdminNodeUpdate
		}
		request.DisplayName = &trimmed
	}
	if request.CountryCode != nil {
		changed = true
		trimmed := strings.ToUpper(strings.TrimSpace(*request.CountryCode))
		if len(trimmed) > 8 {
			return errInvalidAdminNodeUpdate
		}
		request.CountryCode = &trimmed
	}
	if request.Region != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.Region)
		request.Region = &trimmed
	}
	if request.HomeProbeTargetID != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.HomeProbeTargetID)
		request.HomeProbeTargetID = &trimmed
	}
	if request.ExpiryDate != nil {
		changed = true
		trimmed, err := normalizeAdminNodeDate(*request.ExpiryDate)
		if err != nil {
			return errInvalidAdminNodeUpdate
		}
		request.ExpiryDate = &trimmed
	}
	if request.ExpiryPermanent != nil {
		changed = true
	}
	if request.BillingCycle != nil {
		changed = true
		trimmed, err := normalizeAdminNodeShortText(*request.BillingCycle, 64)
		if err != nil {
			return errInvalidAdminNodeUpdate
		}
		request.BillingCycle = &trimmed
	}
	if request.RenewalAmount.Set {
		changed = true
		if request.RenewalAmount.Valid {
			amount, ok := normalizeAdminNodeRenewalAmount(request.RenewalAmount.Value)
			if !ok {
				return errInvalidAdminNodeUpdate
			}
			request.RenewalAmount.Value = amount
		}
	}
	if request.RenewalCurrency != nil {
		changed = true
		currency, ok := normalizeAdminNodeRenewalCurrency(*request.RenewalCurrency)
		if !ok {
			return errInvalidAdminNodeUpdate
		}
		request.RenewalCurrency = &currency
	}
	if request.BillingMode != nil {
		changed = true
		mode, ok := normalizeAdminNodeBillingMode(*request.BillingMode)
		if !ok {
			return errInvalidAdminNodeUpdate
		}
		request.BillingMode = &mode
	}
	if request.MonthlyResetDay != nil {
		changed = true
		if *request.MonthlyResetDay == 0 {
			return errInvalidAdminNodeUpdate
		}
		resetDay, ok := normalizeAdminNodeMonthlyResetDay(*request.MonthlyResetDay)
		if !ok {
			return errInvalidAdminNodeUpdate
		}
		request.MonthlyResetDay = &resetDay
	}
	if request.DisplayOrder != nil {
		changed = true
		if *request.DisplayOrder < 0 {
			return errInvalidAdminNodeUpdate
		}
	}
	if request.PublicIPv4 != nil {
		changed = true
		trimmed, err := normalizeAdminNodeIP(*request.PublicIPv4, 4)
		if err != nil {
			return errInvalidAdminNodeUpdate
		}
		request.PublicIPv4 = &trimmed
	}
	if request.PublicIPv6 != nil {
		changed = true
		trimmed, err := normalizeAdminNodeIP(*request.PublicIPv6, 6)
		if err != nil {
			return errInvalidAdminNodeUpdate
		}
		request.PublicIPv6 = &trimmed
	}
	if request.MonthlyQuotaBytes.Set {
		changed = true
		if request.MonthlyQuotaBytes.Valid && request.MonthlyQuotaBytes.Value < 0 {
			return errInvalidAdminNodeUpdate
		}
	}
	if request.Disabled != nil {
		changed = true
	}
	if request.ProbeTargetIDs != nil {
		changed = true
		seen := make(map[string]struct{}, len(request.ProbeTargetIDs))
		normalized := make([]string, 0, len(request.ProbeTargetIDs))
		for _, targetID := range request.ProbeTargetIDs {
			targetID = strings.TrimSpace(targetID)
			if targetID == "" {
				return errInvalidAdminNodeUpdate
			}
			if _, exists := seen[targetID]; exists {
				return errInvalidAdminNodeUpdate
			}
			seen[targetID] = struct{}{}
			normalized = append(normalized, targetID)
		}
		request.ProbeTargetIDs = normalized
	}
	if !changed {
		return errInvalidAdminNodeUpdate
	}
	return nil
}

type adminOptionalInt64 struct {
	Set   bool
	Valid bool
	Value int64
}

type adminOptionalFloat struct {
	Set   bool
	Valid bool
	Value float64
}

func (value *adminOptionalFloat) UnmarshalJSON(data []byte) error {
	value.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Valid = false
		value.Value = 0
		return nil
	}
	var parsed float64
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	value.Valid = true
	value.Value = parsed
	return nil
}

func (value *adminOptionalInt64) UnmarshalJSON(data []byte) error {
	value.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Valid = false
		value.Value = 0
		return nil
	}
	var parsed int64
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	value.Valid = true
	value.Value = parsed
	return nil
}

type AdminNode struct {
	ID                string   `json:"id"`
	DisplayName       string   `json:"display_name"`
	Status            string   `json:"status"`
	CountryCode       string   `json:"country_code,omitempty"`
	Region            string   `json:"region,omitempty"`
	HomeProbeTargetID string   `json:"home_probe_target_id,omitempty"`
	Disabled          bool     `json:"disabled"`
	BillingMode       string   `json:"billing_mode"`
	MonthlyResetDay   int      `json:"monthly_reset_day"`
	ExpiryDate        string   `json:"expiry_date,omitempty"`
	ExpiryPermanent   bool     `json:"expiry_permanent"`
	BillingCycle      string   `json:"billing_cycle,omitempty"`
	RenewalAmount     *float64 `json:"renewal_amount,omitempty"`
	RenewalCurrency   string   `json:"renewal_currency"`
	DisplayOrder      int      `json:"display_order"`
	PublicIPv4        string   `json:"public_ipv4,omitempty"`
	PublicIPv6        string   `json:"public_ipv6,omitempty"`
	MonthlyQuotaBytes *int64   `json:"monthly_quota_bytes,omitempty"`
	LastSeenAt        *string  `json:"last_seen_at,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	Hostname          string   `json:"hostname,omitempty"`
	OSName            string   `json:"os_name,omitempty"`
	OSVersion         string   `json:"os_version,omitempty"`
	Kernel            string   `json:"kernel,omitempty"`
	Arch              string   `json:"arch,omitempty"`
	Virtualization    string   `json:"virtualization,omitempty"`
	CPUModel          string   `json:"cpu_model,omitempty"`
	CPUCores          *int     `json:"cpu_cores,omitempty"`
	MemoryTotalBytes  *int64   `json:"memory_total_bytes,omitempty"`
	DiskTotalBytes    *int64   `json:"disk_total_bytes,omitempty"`
	BootTime          *string  `json:"boot_time,omitempty"`
	AgentVersion      string   `json:"agent_version,omitempty"`
}

var adminNodeRenewalCurrencies = map[string]struct{}{
	"CNY": {},
	"USD": {},
	"HKD": {},
	"EUR": {},
	"GBP": {},
	"JPY": {},
	"SGD": {},
	"AUD": {},
	"CAD": {},
	"KRW": {},
}

func normalizeAdminNodeRenewalCurrency(value string) (string, bool) {
	currency := strings.ToUpper(strings.TrimSpace(value))
	if currency == "" {
		currency = "CNY"
	}
	_, ok := adminNodeRenewalCurrencies[currency]
	return currency, ok
}

func normalizeAdminNodeRenewalAmount(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1_000_000_000 {
		return 0, false
	}
	return math.Round(value*100) / 100, true
}
