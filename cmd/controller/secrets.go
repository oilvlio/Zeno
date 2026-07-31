package main

import (
	"os"
	"strings"
)

// controllerSecrets is the resolved secret material the controller needs.
//
// Each notification key has two mutually exclusive sources (a single-key file
// or a key ring, either flag- or environment-provided). Resolving them in main
// interleaved that source selection with process startup; keeping it here means
// the precedence rules are stated once.
type controllerSecrets struct {
	agentToken  string
	adminToken  string
	authority   notificationAuthorityMaterial
	credentials notificationCredentialMaterial
}

// notificationAuthorityMaterial carries either a single authority key or a ring.
type notificationAuthorityMaterial struct {
	key         string
	activeKeyID string
	keys        map[string]string
}

// notificationCredentialMaterial carries either a single credential key or a ring.
type notificationCredentialMaterial struct {
	key         []byte
	activeKeyID string
	keys        map[string][]byte
}

// resolveControllerSecrets reads every secret the controller starts with.
//
// A key ring always wins over the single-key file: an operator who configured a
// ring is mid-rotation, and silently falling back to one key would drop the
// previous key needed to prove continuity.
func resolveControllerSecrets(options *controllerOptions) (controllerSecrets, error) {
	var secrets controllerSecrets

	agentToken, err := readAgentToken(options.agentToken, options.agentTokenFile)
	if err != nil {
		return controllerSecrets{}, err
	}
	secrets.agentToken = agentToken

	adminToken, err := readSecret(options.adminToken, options.adminTokenFile)
	if err != nil {
		return controllerSecrets{}, err
	}
	secrets.adminToken = adminToken

	authority, err := resolveNotificationAuthorityMaterial(options)
	if err != nil {
		return controllerSecrets{}, err
	}
	secrets.authority = authority

	credentials, err := resolveNotificationCredentialMaterial(options)
	if err != nil {
		return controllerSecrets{}, err
	}
	secrets.credentials = credentials

	return secrets, nil
}

func resolveNotificationAuthorityMaterial(options *controllerOptions) (notificationAuthorityMaterial, error) {
	keyringFile := firstNonEmpty(
		options.notificationAuthorityKeyringFile,
		os.Getenv("ZENO_NOTIFICATION_AUTHORITY_KEYRING_FILE"),
	)
	if keyringFile != "" {
		activeKeyID, keys, err := readNotificationAuthorityKeyringFile(keyringFile)
		if err != nil {
			return notificationAuthorityMaterial{}, err
		}
		return notificationAuthorityMaterial{activeKeyID: activeKeyID, keys: keys}, nil
	}
	key, err := readNotificationAuthorityKeyFile(options.notificationAuthorityKeyFile)
	if err != nil {
		return notificationAuthorityMaterial{}, err
	}
	return notificationAuthorityMaterial{key: key}, nil
}

func resolveNotificationCredentialMaterial(options *controllerOptions) (notificationCredentialMaterial, error) {
	keyringFile := firstNonEmpty(
		options.notificationCredentialKeyringFile,
		os.Getenv("ZENO_NOTIFICATION_CREDENTIAL_KEYRING_FILE"),
	)
	if keyringFile != "" {
		activeKeyID, keys, err := readNotificationCredentialKeyringFile(keyringFile)
		if err != nil {
			return notificationCredentialMaterial{}, err
		}
		return notificationCredentialMaterial{activeKeyID: activeKeyID, keys: keys}, nil
	}
	keyFile := firstNonEmpty(
		options.notificationCredentialKeyFile,
		os.Getenv("ZENO_NOTIFICATION_CREDENTIAL_KEY_FILE"),
	)
	key, err := readNotificationCredentialKeyFile(keyFile)
	if err != nil {
		return notificationCredentialMaterial{}, err
	}
	return notificationCredentialMaterial{key: key}, nil
}

// notificationsDisabled reports the kill switch used by deployments that run
// the dashboard without any outbound delivery.
func notificationsDisabled() bool {
	value := strings.TrimSpace(os.Getenv("ZENO_NOTIFICATIONS_DISABLED"))
	return strings.EqualFold(value, "true") || value == "1"
}

// firstNonEmpty returns the first value that is non-blank after trimming, which
// is how flag-then-environment precedence is expressed.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
