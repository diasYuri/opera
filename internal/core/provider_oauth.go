package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/diasYuri/y/pkg/providers/auth"
	"github.com/diasYuri/y/pkg/providers/oauth"
)

// OAuthCredentialRecord is the stable secret-store representation used by
// generic OAuth and provider login flows. Keep its JSON fields compatible.
type OAuthCredentialRecord struct {
	Provider     string       `json:"provider"`
	Flow         string       `json:"flow,omitempty"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresAt    time.Time    `json:"expires_at"`
	Config       oauth.Config `json:"config"`
}

// ResolveOAuthCredential applies provider-owned refresh semantics and returns
// updated JSON only when a refresh changed the stored credential.
func ResolveOAuthCredential(ctx context.Context, provider, raw string, now time.Time) (secret, updated string, changed bool, err error) {
	var record OAuthCredentialRecord
	if json.Unmarshal([]byte(raw), &record) != nil || record.Provider != provider {
		return "", "", false, errors.New("invalid stored OAuth credential")
	}
	if record.Flow != "" {
		return "", "", false, errors.New("provider authentication handler unavailable")
	}
	credential := auth.Credential{ProviderID: provider, AccessToken: record.AccessToken, RefreshToken: record.RefreshToken, TokenType: record.TokenType, ExpiresAt: record.ExpiresAt}
	if credential.Expired(now) {
		credential, err = (oauth.Client{Config: record.Config}).Refresh(ctx, credential)
		if err != nil {
			return "", "", false, err
		}
		record.AccessToken, record.RefreshToken, record.TokenType, record.ExpiresAt = credential.AccessToken, credential.RefreshToken, credential.TokenType, credential.ExpiresAt
		updatedBytes, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return "", "", false, errors.New("cannot encode refreshed credential")
		}
		updated, changed = string(updatedBytes), true
	}
	return credential.Secret(), updatedValue(raw, updated), changed, nil
}

func updatedValue(raw, updated string) string {
	if updated == "" {
		return raw
	}
	return updated
}
