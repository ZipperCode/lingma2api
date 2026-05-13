package auth

import (
	"fmt"
	"net/url"
	"strconv"
)

// CallbackV2Result holds the decoded fields from a V2 (or V1-fallback) callback.
type CallbackV2Result struct {
	UID                string
	AID                string
	Name               string
	SecurityOAuthToken string
	RefreshToken       string
	ExpireTime         int64  // unix millis, 0 = unknown
	ExpireTimeRaw      string // original string if parsing failed
}

// ParseCallbackV2 tries to parse query parameters as a V2 callback first
// (auth=<Encode1> + token=<Encode1>), then falls back to V1 flat params.
func ParseCallbackV2(query url.Values) (*CallbackV2Result, error) {
	result := &CallbackV2Result{}

	// V2 path: auth and token are Encode=1 encoded
	if auth := query.Get("auth"); auth != "" {
		parts, err := CustomDecryptParts(auth, 3)
		if err == nil && len(parts) >= 3 {
			result.UID = parts[0]
			result.AID = parts[1]
			result.Name = parts[2]
		} else {
			return nil, fmt.Errorf("v2 auth decode failed: %w", err)
		}
	}

	if tokenParam := query.Get("token"); tokenParam != "" {
		parts, err := CustomDecryptParts(tokenParam, 3)
		if err == nil && len(parts) >= 3 {
			result.SecurityOAuthToken = parts[0]
			result.RefreshToken = parts[1]
			result.ExpireTimeRaw = parts[2]
			if v, parseErr := strconv.ParseInt(parts[2], 10, 64); parseErr == nil {
				result.ExpireTime = v
			}
		} else {
			return nil, fmt.Errorf("v2 token decode failed: %w", err)
		}
	}

	// If V2 decoded successfully, return
	if result.UID != "" && result.SecurityOAuthToken != "" {
		return result, nil
	}

	// V1 fallback: flat params
	if uid := query.Get("uid"); uid != "" {
		result.UID = uid
	}
	if aid := query.Get("aid"); aid != "" {
		result.AID = aid
	}
	if name := query.Get("name"); name != "" {
		result.Name = name
	}
	if result.UID != "" {
		return result, nil
	}

	return nil, fmt.Errorf("callback contains neither V2 (auth+token) nor V1 (uid/aid/name) parameters")
}

// ParseCallbackV2FromStrings is a convenience wrapper that decodes auth and
// token Encode=1 strings directly, without requiring url.Values.
func ParseCallbackV2FromStrings(authParam, tokenParam string) (*CallbackV2Result, error) {
	result := &CallbackV2Result{}

	if authParam != "" {
		parts, err := CustomDecryptParts(authParam, 3)
		if err == nil && len(parts) >= 3 {
			result.UID = parts[0]
			result.AID = parts[1]
			result.Name = parts[2]
		} else {
			return nil, fmt.Errorf("v2 auth decode failed: %w", err)
		}
	}

	if tokenParam != "" {
		parts, err := CustomDecryptParts(tokenParam, 3)
		if err == nil && len(parts) >= 3 {
			result.SecurityOAuthToken = parts[0]
			result.RefreshToken = parts[1]
			result.ExpireTimeRaw = parts[2]
			if v, parseErr := strconv.ParseInt(parts[2], 10, 64); parseErr == nil {
				result.ExpireTime = v
			}
		} else {
			return nil, fmt.Errorf("v2 token decode failed: %w", err)
		}
	}

	if result.UID == "" || result.SecurityOAuthToken == "" {
		return nil, fmt.Errorf("v2 callback missing uid or token after decode")
	}

	return result, nil
}
