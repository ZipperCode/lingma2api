package auth

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
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
// (auth=<Encode1> + token=<Encode1>), then falls back to plain token fields
// and finally to V1 flat params.
func ParseCallbackV2(query url.Values) (*CallbackV2Result, error) {
	result := &CallbackV2Result{}

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

	if result.UID != "" && result.SecurityOAuthToken != "" {
		return result, nil
	}

	if uid := query.Get("uid"); uid != "" {
		result.UID = uid
	}
	if aid := query.Get("aid"); aid != "" {
		result.AID = aid
	}
	if name := query.Get("name"); name != "" {
		result.Name = name
	}
	if accessToken := query.Get("access_token"); accessToken != "" {
		result.SecurityOAuthToken = accessToken
	}
	if refreshToken := query.Get("refresh_token"); refreshToken != "" {
		result.RefreshToken = refreshToken
	}
	if expireTime := query.Get("expire_time"); expireTime != "" {
		result.ExpireTimeRaw = expireTime
		if v, err := strconv.ParseInt(expireTime, 10, 64); err == nil {
			result.ExpireTime = v
		}
	}

	if result.UID != "" && result.SecurityOAuthToken != "" {
		return result, nil
	}
	if result.UID != "" {
		return result, nil
	}

	return nil, fmt.Errorf("callback contains neither V2 (auth+token) nor V1/plain token parameters")
}

// ParseCallbackV2FromURL parses a full callback URL string.
func ParseCallbackV2FromURL(rawURL string) (*CallbackV2Result, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("callback url is empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse callback url: %w", err)
	}
	if parsed.RawQuery == "" {
		return nil, fmt.Errorf("callback url missing query parameters")
	}

	return ParseCallbackV2(parsed.Query())
}

// ParseCallbackV2FromStrings decodes Encode=1 auth/token strings directly.
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
