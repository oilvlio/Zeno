package api

import (
	"context"
	"net/http"
)

func (h *handler) handleAdminAlertRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	store, ok := h.authorizeAdminRequest(w, r)
	if !ok {
		return
	}
	rules, err := store.AdminAlertRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, AdminAlertRulesResponse{Rules: rules})
}

func (h *handler) handleAdminAlertRuleResource(w http.ResponseWriter, r *http.Request) {
	handleAdminPatchResource(h, w, r, "/api/admin/v1/alert-rules/", updateAdminAlertRule)
}

func updateAdminAlertRule(ctx context.Context, store adminStore, ruleID string, update AdminAlertRuleUpdateRequest) (AdminAlertRuleResponse, error) {
	rule, err := store.UpdateAdminAlertRule(ctx, ruleID, update)
	return AdminAlertRuleResponse{Rule: rule}, err
}
