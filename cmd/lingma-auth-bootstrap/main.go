package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"lingma2api/internal/auth"
	"lingma2api/internal/proxy"
)

// lingma-auth-bootstrap drives the Lingma login flow without ever touching the
// standard Alibaba Cloud OAuth grant. Lingma issues login URLs against
// devops.aliyun.com/lingma/login (no client_id required); the server injects
// client_id in its 302 chain. Tokens come back via /auth/callback as Lingma's
// custom auth/token params (Encode=1) or via window.user_info posted by a
// bookmarklet to /submit-userinfo. From there, DeriveCredentialsRemotely
// (or DeriveCredentialsWithLingma) produces cosy_key + encrypt_user_info.
func main() {
	var (
		listenAddr        string
		outputPath        string
		lingmaBin         string
		useLingma         bool
		sessionKey        string
		machineIDOverride string
		importLingmaCache bool
		userInfoJSON      string
		userInfoFile      string
		loginURLOverride  string
		refreshFile       string
	)
	flag.StringVar(&listenAddr, "listen-addr", "127.0.0.1:37510", "local callback listen address")
	flag.StringVar(&outputPath, "output", "./auth/credentials.json", "output credentials.json file")
	flag.StringVar(&lingmaBin, "lingma-bin", "", "path to Lingma binary (auto-detect if empty)")
	flag.BoolVar(&useLingma, "use-lingma", true, "use local Lingma binary to complete credential derivation")
	flag.StringVar(&sessionKey, "session-key", "", "Signature session_key for pure remote derivation (no Lingma binary)")
	flag.StringVar(&machineIDOverride, "machine-id", "", "machine_id for callback / derivation (auto-generated UUID if empty)")
	flag.BoolVar(&importLingmaCache, "import-lingma-cache", false, "import credentials from ~/.lingma cache and exit")
	flag.StringVar(&userInfoJSON, "user-info-json", "", "window.user_info JSON from Lingma callback page")
	flag.StringVar(&userInfoFile, "user-info-file", "", "file containing window.user_info JSON")
	flag.StringVar(&loginURLOverride, "login-url", "", "window.login_url from Lingma callback page (for machine_id extraction)")
	flag.StringVar(&refreshFile, "refresh", "", "refresh existing credentials.json via local Lingma WebSocket")
	flag.Parse()

	switch {
	case importLingmaCache:
		runImportLingmaCache(outputPath)
	case userInfoJSON != "" || userInfoFile != "":
		runBootstrapFromUserInfo(loadUserInfo(userInfoJSON, userInfoFile), loginURLOverride, sessionKey, outputPath)
	case refreshFile != "":
		runRefresh(refreshFile, useLingma, lingmaBin)
	default:
		runBootstrap(listenAddr, outputPath, sessionKey, useLingma, lingmaBin, machineIDOverride)
	}
}

func runImportLingmaCache(outputPath string) {
	stored, err := auth.TryImportFromLingmaCache(outputPath)
	if err != nil {
		log.Fatalf("import from ~/.lingma cache: %v", err)
	}
	fmt.Printf("Imported credentials from ~/.lingma cache.\n")
	fmt.Printf("  Source: %s\n", stored.Source)
	fmt.Printf("  UserID: %s\n", stored.Auth.UserID)
	fmt.Printf("  MachineID: %s\n", stored.Auth.MachineID)
	fmt.Printf("  TokenExpireTime: %s\n", stored.TokenExpireTime)
	fmt.Printf("  Saved to: %s\n", outputPath)
}

func loadUserInfo(inline, fromFile string) string {
	if inline != "" {
		return inline
	}
	data, err := os.ReadFile(fromFile)
	if err != nil {
		log.Fatalf("read user-info-file: %v", err)
	}
	return string(data)
}

func runRefresh(refreshFile string, useLingma bool, lingmaBin string) {
	fmt.Println("Refreshing tokens via local Lingma WebSocket...")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := auth.RefreshAndSave(ctx, refreshFile, &auth.WSRefresher{}, useLingma, lingmaBin); err != nil {
		log.Fatalf("refresh failed: %v", err)
	}
	fmt.Printf("\nCredentials refreshed and written to %s\n", refreshFile)
}

// runBootstrap drives the Lingma login URL flow.
//
// Pipeline:
//  1. Build a Lingma login entry URL (no client_id; Lingma server injects
//     it in the 302 chain).
//  2. Listen on listenAddr for the callback.
//  3. Accept any of:
//     - POST /submit-userinfo body {userInfo, loginUrl} (bookmarklet)
//     - GET /auth/callback?auth=...&token=... (Lingma's Encode=1 callback)
//  4. Derive cosy_key + encrypt_user_info via DeriveCredentialsRemotely
//     (or DeriveCredentialsWithLingma when --use-lingma).
//  5. Persist to outputPath.
func runBootstrap(listenAddr, outputPath, sessionKey string, useLingma bool, lingmaBin, machineIDOverride string) {
	machineID := machineIDOverride
	if machineID == "" {
		machineID = auth.NewMachineID()
		fmt.Printf("Auto-generated machine_id: %s\n", machineID)
	}

	loginURL, _, _, err := auth.BuildLingmaLoginEntryURL(auth.LingmaLoginEntryConfig{
		MachineID: machineID,
		Port:      portFromListenAddr(listenAddr),
	})
	if err != nil {
		log.Fatalf("build lingma login entry url: %v", err)
	}

	browserURL, err := auth.WrapLingmaLoginURLForBrowser(loginURL)
	if err != nil {
		log.Fatalf("wrap login url: %v", err)
	}

	fmt.Println("=== Lingma login URL flow (no client_id required) ===")
	fmt.Println()
	fmt.Println("1. Open the following URL in your browser:")
	fmt.Println()
	fmt.Println(browserURL)
	fmt.Println()
	fmt.Println("2. Complete Alibaba Cloud login.")
	fmt.Println("3. After login, the page redirects locally. On that page, press F12 → Console:")
	fmt.Println()
	fmt.Println(`   fetch('http://` + listenAddr + `/submit-userinfo', {`)
	fmt.Println(`     method: 'POST',`)
	fmt.Println(`     headers: {'Content-Type': 'application/json'},`)
	fmt.Println(`     body: JSON.stringify({userInfo: window.user_info, loginUrl: window.login_url})`)
	fmt.Println(`   }).then(r => r.text()).then(console.log)`)
	fmt.Println()
	fmt.Printf("Callback server listening on %s (timeout 5 min)...\n", listenAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	capture, err := auth.WaitForCallback(ctx, listenAddr, "/callback")
	if err != nil {
		log.Fatalf("wait for callback: %v", err)
	}

	// Path 1: bookmarklet POST /submit-userinfo
	if len(capture.Body) > 0 {
		handleSubmitUserInfo(capture.Body, sessionKey, outputPath)
		return
	}

	// Path 2: Lingma-specific ?auth=&token= callback (Encode=1)
	authParam := capture.Query.Get("auth")
	tokenParam := capture.Query.Get("token")
	if authParam != "" || tokenParam != "" {
		handleAuthTokenCallback(authParam, tokenParam, machineID, outputPath)
		return
	}

	log.Fatalf("callback did not contain recognizable Lingma auth data (path=%s, query=%v)",
		capture.Path, capture.Query)
}

func runBootstrapFromUserInfo(userInfoJSON, loginURL, sessionKey, outputPath string) {
	fmt.Println("=== Extracting tokens from window.user_info JSON ===")
	extracted, err := auth.ExtractFromCallbackPage(userInfoJSON, loginURL)
	if err != nil {
		log.Fatalf("parse user_info: %v", err)
	}
	fmt.Printf("  AccessToken:  %s\n", maskValue(extracted.AccessToken, 15))
	fmt.Printf("  RefreshToken: %s\n", maskValue(extracted.RefreshToken, 15))
	fmt.Printf("  UserID:       %s\n", extracted.UserID)
	fmt.Printf("  MachineID:    %s\n", extracted.MachineID)
	fmt.Printf("  ExpireTime:   %s\n", extracted.TokenExpireMs)

	deriveAndSave(extracted.AccessToken, extracted.RefreshToken, extracted.UserID, extracted.Username,
		extracted.MachineID, extracted.TokenExpireMs, sessionKey, false, "", outputPath)
}

func handleSubmitUserInfo(body []byte, sessionKey, outputPath string) {
	fmt.Println("=== Processing bookmarklet submission ===")

	var submission struct {
		UserInfo string `json:"userInfo"`
		LoginURL string `json:"loginUrl"`
	}
	if err := json.Unmarshal(body, &submission); err != nil {
		log.Fatalf("parse submit-userinfo body: %v\nRaw: %s", err, string(body))
	}
	if submission.UserInfo == "" {
		log.Fatal("submit-userinfo body missing userInfo field")
	}

	extracted, err := auth.ExtractFromCallbackPage(submission.UserInfo, submission.LoginURL)
	if err != nil {
		log.Fatalf("extract from callback page: %v", err)
	}
	fmt.Printf("  AccessToken:  %s\n", maskValue(extracted.AccessToken, 15))
	fmt.Printf("  UserID:       %s\n", extracted.UserID)
	fmt.Printf("  MachineID:    %s\n", extracted.MachineID)

	deriveAndSave(extracted.AccessToken, extracted.RefreshToken, extracted.UserID, extracted.Username,
		extracted.MachineID, extracted.TokenExpireMs, sessionKey, false, "", outputPath)
}

// handleAuthTokenCallback dumps the Encode=1 decoded auth/token params for
// further investigation. The full Lingma-specific format is still under
// reverse engineering; for production use, prefer --user-info-json.
func handleAuthTokenCallback(authEncoded, tokenEncoded, machineID, outputPath string) {
	fmt.Println("=== Lingma-specific callback (auth/token Encode=1 params) ===")
	fmt.Printf("  auth:  %s...\n", maskValue(authEncoded, 20))
	fmt.Printf("  token: %s...\n", maskValue(tokenEncoded, 20))

	authRaw := auth.LingmaDecode(authEncoded)
	tokenRaw := auth.LingmaDecode(tokenEncoded)
	if authRaw == nil || tokenRaw == nil {
		log.Fatal("could not Encode=1 decode auth/token params")
	}

	output := map[string]interface{}{
		"schema_version": 1,
		"source":         "lingma_auth_token_callback",
		"machine_id":     machineID,
		"auth_decoded":   string(authRaw),
		"token_decoded":  string(tokenRaw),
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
	data, _ := json.MarshalIndent(output, "", "  ")
	rawPath := outputPath + "-raw.json"
	if err := os.WriteFile(rawPath, data, 0o600); err != nil {
		log.Fatalf("write raw output: %v", err)
	}
	fmt.Printf("\nRaw decoded data saved to %s\n", rawPath)
	fmt.Println("Full decoding of auth/token format is still under investigation.")
	fmt.Println("For now, use --user-info-json with window.user_info from the browser console.")
	os.Exit(1)
}

func deriveAndSave(accessToken, refreshToken, userID, username, machineID, tokenExpireMs, sessionKey string, useLingma bool, lingmaBin, outputPath string) {
	fmt.Println("\n=== Deriving credentials ===")

	if machineID == "" {
		machineID = auth.NewMachineID()
		fmt.Printf("Auto-generated machine_id: %s\n", machineID)
	}

	var stored proxy.StoredCredentialFile
	var err error
	if useLingma {
		fmt.Println("Starting Lingma to sync credentials...")
		stored, err = auth.DeriveCredentialsWithLingma(auth.LingmaBridgeConfig{
			LingmaBinary:  lingmaBin,
			AccessToken:   accessToken,
			RefreshToken:  refreshToken,
			UserID:        userID,
			Username:      username,
			TokenExpireMs: tokenExpireMs,
		})
	} else {
		fmt.Println("Calling remote user/login...")
		stored, err = auth.DeriveCredentialsRemotely(auth.RemoteLoginConfig{
			AccessToken:   accessToken,
			RefreshToken:  refreshToken,
			UserID:        userID,
			Username:      username,
			MachineID:     machineID,
			TokenExpireMs: tokenExpireMs,
			SessionKey:    sessionKey,
		})
	}
	if err != nil {
		log.Fatalf("derive credentials: %v", err)
	}

	if userID != "" && stored.Auth.UserID == "" {
		stored.Auth.UserID = userID
	}
	if stored.Auth.MachineID == "" {
		stored.Auth.MachineID = machineID
	}

	if err := auth.SaveCredentialFile(outputPath, stored); err != nil {
		log.Fatalf("save credentials: %v", err)
	}

	fmt.Printf("\nCredentials written to %s\n", outputPath)
	if stored.Auth.CosyKey != "" {
		fmt.Println("cosy_key: PRESENT")
	} else {
		fmt.Println("cosy_key: MISSING — derivation may need Lingma binary or v3 endpoint fix")
	}
}

func portFromListenAddr(listenAddr string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		return "37510"
	}
	return port
}

func maskValue(value string, keep int) string {
	if value == "" {
		return ""
	}
	if len(value) <= keep {
		return value
	}
	return value[:keep] + "..."
}
