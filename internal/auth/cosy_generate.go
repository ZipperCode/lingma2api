package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
)

// Embedded 1024-bit RSA public key (IDA @ 0x1425bd8e8, PKIX DER).
const rsaPubKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

var embeddedRSAPubKey *rsa.PublicKey

func init() {
	block, _ := pem.Decode([]byte(rsaPubKeyPEM))
	if block == nil {
		panic("failed to decode embedded RSA public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic(fmt.Sprintf("failed to parse embedded RSA public key: %v", err))
	}
	var ok bool
	embeddedRSAPubKey, ok = pub.(*rsa.PublicKey)
	if !ok {
		panic("embedded key is not an RSA public key")
	}
}

// cosyUserInfo mirrors the full JSON structure used by Lingma (21 fields).
// This matches the Python implementation in lingma_oauth_standalone.py.
type cosyUserInfo struct {
	Name                 string `json:"name"`
	AID                  string `json:"aid"`
	UID                  string `json:"uid"`
	YxUID                string `json:"yx_uid"`
	OrganizationID       string `json:"organization_id"`
	OrganizationName     string `json:"organization_name"`
	StaffID              string `json:"staffId"`
	AvatarURL            string `json:"avatar_url"`
	Key                  string `json:"key"`
	EncryptUserInfo      string `json:"encrypt_user_info"`
	UserSourceChannel    string `json:"user_source_channel"`
	SecurityOauthToken   string `json:"security_oauth_token"`
	RefreshToken         string `json:"refresh_token"`
	ExpireTime           int    `json:"expire_time"`
	UserType             string `json:"user_type"`
	DataPolicyAgreed     bool   `json:"data_policy_agreed"`
	Email                string `json:"email"`
	IsDataPolicyModifiable bool  `json:"is_data_policy_modifiable"`
	IsQuotaExceeded      bool   `json:"is_quota_exceeded"`
	OrganizationTags     any    `json:"organization_tags"`
}

// CosyCredentialInput holds the user info needed to generate COSY credentials.
type CosyCredentialInput struct {
	Name               string
	UID                string
	AID                string
	YxUID              string
	OrganizationID     string
	OrganizationName   string
	UserType           string
	SecurityOAuthToken string
	RefreshToken       string
}

// GenerateCosyCredentials locally generates cosy_key and encrypt_user_info
// using the embedded 1024-bit RSA public key and AES-128-CBC.
//
// Algorithm:
//  1. Generate random 16-char hex string as AES temp key (uuid-style)
//  2. RSA-PKCS1v15 encrypt tempKey with embedded pubkey → base64 → cosy_key
//  3. Build full 21-field CosyUserInfo JSON → AES-128-CBC(key=IV=tempKey, PKCS7) → base64 → encrypt_user_info
func GenerateCosyCredentials(in CosyCredentialInput) (cosyKey, encryptUserInfo string, err error) {
	// Generate 16 random bytes, encode as hex, take first 16 chars (uuid-style)
	var randBuf [16]byte
	if _, randErr := rand.Read(randBuf[:]); randErr != nil {
		return "", "", fmt.Errorf("generate temp key: %w", randErr)
	}
	tempKeyStr := hex.EncodeToString(randBuf[:])[:16]
	tempKey := []byte(tempKeyStr)

	// RSA encrypt temp key → base64 → cosy_key
	encryptedKey, rsaErr := rsa.EncryptPKCS1v15(rand.Reader, embeddedRSAPubKey, tempKey)
	if rsaErr != nil {
		return "", "", fmt.Errorf("rsa encrypt temp key: %w", rsaErr)
	}
	cosyKey = base64.StdEncoding.EncodeToString(encryptedKey)

	// Build full CosyUserInfo JSON (21-field structure matching Lingma)
	aid := in.AID
	if aid == "" {
		aid = in.UID
	}
	userType := in.UserType
	if userType == "" {
		userType = "" // Python uses empty string, not "personal_standard"
	}
	info := cosyUserInfo{
		Name:                 in.Name,
		AID:                  aid,
		UID:                  in.UID,
		YxUID:                "", // empty per Python
		OrganizationID:       "", // empty per Python
		OrganizationName:     "", // empty per Python
		StaffID:              "", // empty per Python
		AvatarURL:            "", // empty per Python
		Key:                  "", // empty per Python
		EncryptUserInfo:      "", // empty per Python
		UserSourceChannel:    "", // empty per Python
		SecurityOauthToken:   in.SecurityOAuthToken,
		RefreshToken:         in.RefreshToken,
		ExpireTime:           0, // 0 per Python
		UserType:             userType,
		DataPolicyAgreed:     false, // false per Python
		Email:                "", // empty per Python
		IsDataPolicyModifiable: false, // false per Python
		IsQuotaExceeded:      false, // false per Python
		OrganizationTags:     nil, // null per Python
	}

	// Use compact JSON encoding (no spaces) to match Python's separators=(",", ":")
	infoJSON, jsonErr := json.Marshal(info)
	if jsonErr != nil {
		return "", "", fmt.Errorf("marshal cosy user info: %w", jsonErr)
	}

	// AES-128-CBC encrypt (key=IV=tempKey, PKCS7 padding)
	block, aesErr := aes.NewCipher(tempKey)
	if aesErr != nil {
		return "", "", fmt.Errorf("create aes cipher: %w", aesErr)
	}

	plaintext := infoJSON
	padLen := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := make([]byte, len(plaintext)+padLen)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}

	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, tempKey).CryptBlocks(ciphertext, padded)
	encryptUserInfo = base64.StdEncoding.EncodeToString(ciphertext)

	return cosyKey, encryptUserInfo, nil
}