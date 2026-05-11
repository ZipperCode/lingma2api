package auth

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"
)

const (
	encode1Alpha  = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"
	encode1StdB64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

var (
	alphaToStd [256]byte
	stdToAlpha [256]byte
)

func init() {
	for i := range alphaToStd {
		alphaToStd[i] = 255
		stdToAlpha[i] = 255
	}
	for i := 0; i < len(encode1Alpha); i++ {
		alphaToStd[encode1Alpha[i]] = encode1StdB64[i]
	}
	for i := 0; i < len(encode1StdB64); i++ {
		stdToAlpha[encode1StdB64[i]] = encode1Alpha[i]
	}
}

func lingmaEncode(data []byte) string {
	std := base64.StdEncoding.EncodeToString(data)
	std = strings.TrimRight(std, "=")

	var custom strings.Builder
	custom.Grow(len(std))
	for i := 0; i < len(std); i++ {
		custom.WriteByte(stdToAlpha[std[i]])
	}
	encoded := custom.String()

	e := len(encoded)
	bs := int(math.Ceil(float64(e) / 3.0))
	pad := (4 - e%4) % 4

	b0 := encoded[:bs]
	var b1, b2 string
	if 2*bs <= e {
		b1 = encoded[bs : 2*bs]
		b2 = encoded[2*bs:]
	} else if bs < e {
		b1 = encoded[bs:]
	}

	return b2 + strings.Repeat("$", pad) + b1 + b0
}

// lingmaDecode reverses the Encode=1 transformation: strip $ padding →
// unscramble the three blocks → reverse alphabet substitution → base64 decode.
//
// Encode layout:  output = b2 + '$'*pad + b1 + b0
//   b0 = original[:f]     where f = floor(E/3)
//   b1 = original[f:2*c]  where c = ceil(E/3)
//   b2 = original[2*c:]
//
// Decode: remove '$', then split clean into (b2, b1, b0) using the
// reciprocal sizes, then reassemble as b0+b1+b2.
func lingmaDecode(body string) []byte {
	// 1. Remove $ padding
	dollarStart := strings.IndexByte(body, '$')
	var clean string
	if dollarStart < 0 {
		clean = body
	} else {
		p := 0
		pos := dollarStart
		for pos < len(body) && body[pos] == '$' {
			p++
			pos++
		}
		clean = body[:dollarStart] + body[dollarStart+p:]
	}

	// 2. Reverse block order
	e := len(clean)
	c := int(math.Ceil(float64(e) / 3.0))
	f := e / 3 // floor(E/3) for integers
	lb := e - 2*c // b2 size (may be 0 or negative)
	if lb < 0 {
		lb = 0
	}
	b1Size := 2*c - f
	if lb+b1Size > e {
		b1Size = e - lb
	}

	b2Block := clean[:lb]
	b1Block := clean[lb : lb+b1Size]
	b0Block := clean[lb+b1Size:]

	reordered := b0Block + b1Block + b2Block

	// 3. Reverse alphabet substitution + base64 decode
	var converted strings.Builder
	converted.Grow(len(reordered))
	for i := 0; i < len(reordered); i++ {
		b := alphaToStd[reordered[i]]
		if b != 255 {
			converted.WriteByte(b)
		}
	}

	stdStr := converted.String()
	missing := (4 - len(stdStr)%4) % 4
	stdStr += strings.Repeat("=", missing)

	decoded, err := base64.StdEncoding.DecodeString(stdStr)
	if err != nil {
		return nil
	}
	return decoded
}

// DecodeString is the public wrapper that returns (nil, error) on failure
// instead of (nil, nil).
func DecodeString(body string) ([]byte, error) {
	raw := lingmaDecode(body)
	if raw == nil {
		return nil, fmt.Errorf("encode1: decode failed")
	}
	return raw, nil
}

// CustomDecryptParts decodes an Encode=1 string and splits it by newline
// into exactly expectedParts parts (or returns an error).
func CustomDecryptParts(encoded string, expectedParts int) ([]string, error) {
	raw, err := DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	text := string(raw)
	parts := strings.SplitN(text, "\n", expectedParts)
	if len(parts) < expectedParts {
		return nil, fmt.Errorf("expected %d parts, got %d", expectedParts, len(parts))
	}
	return parts, nil
}

func LingmaEncode(data []byte) string {
	return lingmaEncode(data)
}

func LingmaDecode(body string) []byte {
	return lingmaDecode(body)
}

func LingmaEncodeAES(plaintext, aesKey []byte) (string, error) {
	return lingmaEncodeAES(plaintext, aesKey)
}

func lingmaEncodeAES(plaintext, aesKey []byte) (string, error) {
	encrypted, err := EncryptCacheUser(string(aesKey), plaintext)
	if err != nil {
		return "", err
	}
	return lingmaEncode(encrypted), nil
}

func lingmaDecodeAES(encoded string, aesKey []byte) ([]byte, error) {
	decoded := lingmaDecode(encoded)
	if decoded == nil {
		return nil, nil
	}
	return DecryptCacheUser(string(aesKey), decoded)
}
