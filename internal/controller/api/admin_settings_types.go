package api

import (
	"math"
	"net"
	"net/url"
	"regexp"
	"strings"
)

type AdminSettingsResponse struct {
	Settings SiteSettings `json:"settings"`
}

type AdminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AdminLoginResponse struct {
	Username string `json:"username"`
	Token    string `json:"token,omitempty"`
}

type AdminAccountResponse struct {
	Account AdminAccount `json:"account"`
}

type AdminAccountUpdateRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type SiteSettings struct {
	SiteTitle            string  `json:"site_title"`
	SiteSubtitle         string  `json:"site_subtitle"`
	LogoURL              string  `json:"logo_url"`
	Theme                string  `json:"theme"`
	AgentControllerURL   string  `json:"agent_controller_url"`
	BackgroundURL        string  `json:"background_url"`
	DesktopBackgroundURL string  `json:"desktop_background_url"`
	MobileBackgroundURL  string  `json:"mobile_background_url"`
	AppearancePreset     string  `json:"appearance_preset"`
	CardOpacity          float64 `json:"card_opacity"`
	CardBlur             float64 `json:"card_blur"`
	CardRadius           float64 `json:"card_radius"`
	BorderStrength       float64 `json:"border_strength"`
	ShadowStrength       float64 `json:"shadow_strength"`
	BackgroundOverlay    float64 `json:"background_overlay"`
	ThemeColor           string  `json:"theme_color"`
	CustomCode           string  `json:"custom_code"`
	UpdatedAt            string  `json:"updated_at,omitempty"`
}

type AdminSettingsUpdateRequest struct {
	SiteTitle            *string  `json:"site_title,omitempty"`
	SiteSubtitle         *string  `json:"site_subtitle,omitempty"`
	LogoURL              *string  `json:"logo_url,omitempty"`
	Theme                *string  `json:"theme,omitempty"`
	AgentControllerURL   *string  `json:"agent_controller_url,omitempty"`
	BackgroundURL        *string  `json:"background_url,omitempty"`
	DesktopBackgroundURL *string  `json:"desktop_background_url,omitempty"`
	MobileBackgroundURL  *string  `json:"mobile_background_url,omitempty"`
	AppearancePreset     *string  `json:"appearance_preset,omitempty"`
	CardOpacity          *float64 `json:"card_opacity,omitempty"`
	CardBlur             *float64 `json:"card_blur,omitempty"`
	CardRadius           *float64 `json:"card_radius,omitempty"`
	BorderStrength       *float64 `json:"border_strength,omitempty"`
	ShadowStrength       *float64 `json:"shadow_strength,omitempty"`
	BackgroundOverlay    *float64 `json:"background_overlay,omitempty"`
	ThemeColor           *string  `json:"theme_color,omitempty"`
	CustomCode           *string  `json:"custom_code,omitempty"`
}

const maxSettingsCustomCodeRunes = 60000

var settingsThemeColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func defaultSiteSettings() SiteSettings {
	return SiteSettings{
		SiteTitle:            "Zeno",
		SiteSubtitle:         "服务器运行概览",
		LogoURL:              "/assets/logo/id.png",
		Theme:                "system",
		AgentControllerURL:   "",
		BackgroundURL:        "",
		DesktopBackgroundURL: "",
		MobileBackgroundURL:  "",
		AppearancePreset:     "default",
		CardOpacity:          0.72,
		CardBlur:             0,
		CardRadius:           20,
		BorderStrength:       0.26,
		ShadowStrength:       0.22,
		BackgroundOverlay:    0,
		ThemeColor:           "#2563eb",
		CustomCode:           "",
	}
}

func (request *AdminSettingsUpdateRequest) normalize() error {
	changed := false
	if request.SiteTitle != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.SiteTitle)
		if trimmed == "" || len([]rune(trimmed)) > 64 {
			return errInvalidAdminSettingsUpdate
		}
		request.SiteTitle = &trimmed
	}
	if request.SiteSubtitle != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.SiteSubtitle)
		if len([]rune(trimmed)) > 140 {
			return errInvalidAdminSettingsUpdate
		}
		request.SiteSubtitle = &trimmed
	}
	if request.LogoURL != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.LogoURL)
		if trimmed != "" && !validSettingsAssetURL(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.LogoURL = &trimmed
	}
	if request.Theme != nil {
		changed = true
		trimmed := strings.ToLower(strings.TrimSpace(*request.Theme))
		if !validSettingsTheme(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.Theme = &trimmed
	}
	if request.AgentControllerURL != nil {
		changed = true
		trimmed := strings.TrimRight(strings.TrimSpace(*request.AgentControllerURL), "/")
		if trimmed != "" && !validAgentControllerURL(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.AgentControllerURL = &trimmed
	}
	if request.BackgroundURL != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.BackgroundURL)
		if trimmed != "" && !validSettingsAssetURL(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.BackgroundURL = &trimmed
	}
	if request.DesktopBackgroundURL != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.DesktopBackgroundURL)
		if trimmed != "" && !validSettingsAssetURL(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.DesktopBackgroundURL = &trimmed
	}
	if request.MobileBackgroundURL != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.MobileBackgroundURL)
		if trimmed != "" && !validSettingsAssetURL(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.MobileBackgroundURL = &trimmed
	}
	if request.AppearancePreset != nil {
		changed = true
		trimmed := strings.ToLower(strings.TrimSpace(*request.AppearancePreset))
		if !validAppearancePreset(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.AppearancePreset = &trimmed
	}
	if request.CardOpacity != nil {
		changed = true
		if !validSettingsFloat(*request.CardOpacity, 0.2, 1) {
			return errInvalidAdminSettingsUpdate
		}
	}
	if request.CardBlur != nil {
		changed = true
		if !validSettingsFloat(*request.CardBlur, 0, 40) {
			return errInvalidAdminSettingsUpdate
		}
	}
	if request.CardRadius != nil {
		changed = true
		if !validSettingsFloat(*request.CardRadius, 8, 36) {
			return errInvalidAdminSettingsUpdate
		}
	}
	if request.BorderStrength != nil {
		changed = true
		if !validSettingsFloat(*request.BorderStrength, 0, 1) {
			return errInvalidAdminSettingsUpdate
		}
	}
	if request.ShadowStrength != nil {
		changed = true
		if !validSettingsFloat(*request.ShadowStrength, 0, 1) {
			return errInvalidAdminSettingsUpdate
		}
	}
	if request.BackgroundOverlay != nil {
		changed = true
		if !validSettingsFloat(*request.BackgroundOverlay, 0, 0.8) {
			return errInvalidAdminSettingsUpdate
		}
	}
	if request.ThemeColor != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.ThemeColor)
		if !settingsThemeColorPattern.MatchString(trimmed) {
			return errInvalidAdminSettingsUpdate
		}
		request.ThemeColor = &trimmed
	}
	if request.CustomCode != nil {
		changed = true
		trimmed := strings.TrimSpace(*request.CustomCode)
		if len([]rune(trimmed)) > maxSettingsCustomCodeRunes {
			return errInvalidAdminSettingsUpdate
		}
		request.CustomCode = &trimmed
	}
	if !changed {
		return errInvalidAdminSettingsUpdate
	}
	return nil
}

func validAppearancePreset(value string) bool {
	return value == "default" || value == "gaussian_blur"
}

func validSettingsFloat(value, min, max float64) bool {
	return value >= min && value <= max && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validSettingsTheme(theme string) bool {
	switch theme {
	case "system", "dark", "light":
		return true
	default:
		return false
	}
}

func validSettingsAssetURL(value string) bool {
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "https"
}

func validAgentControllerURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (loopbackURLHost(parsed.Hostname()) || directIPURLHost(parsed))) {
		return false
	}
	return parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func directIPURLHost(parsed *url.URL) bool {
	host := strings.TrimSpace(strings.Trim(parsed.Hostname(), "[]"))
	return net.ParseIP(host) != nil && parsed.Port() != ""
}

func loopbackURLHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
