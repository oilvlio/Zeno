package api

import (
	"math"
	"strings"
)

type AdminNotificationChannelsResponse struct {
	Channels []AdminNotificationChannel `json:"channels"`
}

type AdminNotificationChannelResponse struct {
	Channel AdminNotificationChannel `json:"channel"`
}

type AdminNotificationTypeResponse struct {
	Type AdminNotificationType `json:"type"`
}

type AdminAlertRulesResponse struct {
	Rules []AdminAlertRule `json:"rules"`
}

type AdminAlertRuleResponse struct {
	Rule AdminAlertRule `json:"rule"`
}

type AdminNotificationTestResponse struct {
	Delivery AdminNotificationDelivery `json:"delivery"`
}

type AdminNotificationRetryResponse struct {
	DeliveryID int64  `json:"delivery_id"`
	State      string `json:"state"`
}

type AdminNotificationChannelCreateRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Credential  string `json:"credential"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

func (request *AdminNotificationChannelCreateRequest) normalize() error {
	request.ID = normalizeAdminNodeID(request.ID)
	request.Name = strings.TrimSpace(request.Name)
	request.Destination = strings.TrimSpace(request.Destination)
	request.Credential = strings.TrimSpace(request.Credential)
	if request.Name == "" || request.Destination == "" || request.Credential == "" {
		return errInvalidAdminNotificationChannelWrite
	}
	return nil
}

type AdminNotificationChannelUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Destination *string `json:"destination,omitempty"`
	Credential  *string `json:"credential,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

func (request *AdminNotificationChannelUpdateRequest) normalize() error {
	changed := false
	if request.Name != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.Name)
		if trimmed == "" {
			return errInvalidAdminNotificationChannelWrite
		}
		request.Name = &trimmed
	}
	if request.Destination != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.Destination)
		if trimmed == "" {
			return errInvalidAdminNotificationChannelWrite
		}
		request.Destination = &trimmed
	}
	if request.Credential != nil {
		trimmed := strings.TrimSpace(*request.Credential)
		if trimmed == "" {
			// Notification credentials are write-only. Treat an explicitly blank
			// PATCH credential the same as an omitted credential so admin forms can
			// leave the field empty without clearing or exposing the stored token.
			request.Credential = nil
		} else {
			changed = true
			request.Credential = &trimmed
		}
	}
	if request.Enabled != nil {
		changed = true
	}
	if !changed {
		return errInvalidAdminNotificationChannelWrite
	}
	return nil
}

type AdminNotificationTypeUpdateRequest struct {
	Enabled *bool `json:"enabled,omitempty"`
}

func (request AdminNotificationTypeUpdateRequest) normalize() error {
	if request.Enabled == nil {
		return errInvalidAdminNotificationTypeWrite
	}
	return nil
}

type AdminAlertRuleUpdateRequest struct {
	Enabled      *bool     `json:"enabled,omitempty"`
	Threshold    *float64  `json:"threshold,omitempty"`
	DurationSec  *int      `json:"duration_sec,omitempty"`
	ScopeNodeIDs *[]string `json:"scope_node_ids,omitempty"`
}

func (request *AdminAlertRuleUpdateRequest) normalize() error {
	changed := false
	if request.Enabled != nil {
		changed = true
	}
	if request.Threshold != nil {
		changed = true
		if math.IsNaN(*request.Threshold) || math.IsInf(*request.Threshold, 0) || *request.Threshold < 0 {
			return errInvalidAdminAlertRuleUpdate
		}
	}
	if request.DurationSec != nil {
		changed = true
		if *request.DurationSec < 0 {
			return errInvalidAdminAlertRuleUpdate
		}
	}
	if request.ScopeNodeIDs != nil {
		changed = true
		normalized := make([]string, 0, len(*request.ScopeNodeIDs))
		seen := map[string]bool{}
		for _, rawNodeID := range *request.ScopeNodeIDs {
			nodeID := normalizeAdminNodeID(rawNodeID)
			if nodeID == "" || seen[nodeID] {
				return errInvalidAdminAlertRuleUpdate
			}
			seen[nodeID] = true
			normalized = append(normalized, nodeID)
		}
		request.ScopeNodeIDs = &normalized
	}
	if !changed {
		return errInvalidAdminAlertRuleUpdate
	}
	return nil
}

type AdminAlertRule struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Category              string   `json:"category"`
	Metric                string   `json:"metric"`
	Comparator            string   `json:"comparator"`
	Threshold             float64  `json:"threshold"`
	ThresholdUnit         string   `json:"threshold_unit"`
	DurationSec           int      `json:"duration_sec"`
	Enabled               bool     `json:"enabled"`
	NotificationEventType string   `json:"notification_event_type"`
	NotificationLabel     string   `json:"notification_label"`
	Description           string   `json:"description"`
	ScopeNodeIDs          []string `json:"scope_node_ids"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
}

type AdminNotificationChannel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Destination   string `json:"destination"`
	Credential    string `json:"-"`
	CredentialSet bool   `json:"credential_set"`
	Enabled       bool   `json:"enabled"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type AdminNotificationType struct {
	EventType string `json:"event_type"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type AdminNotificationDelivery struct {
	ID             int64  `json:"id"`
	EventType      string `json:"event_type"`
	Label          string `json:"label"`
	NodeID         string `json:"node_id"`
	NodeName       string `json:"node_name"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
	ChannelID      string `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
	CreatedAt      string `json:"created_at"`
}
