package oidc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"unicode/utf8"
)

const maxTokenLength = 32 << 10

var base64url = base64.RawURLEncoding.Strict()

type object map[string]json.RawMessage

// jsonObject rejects duplicate members and preserves exact case for claim names.
func jsonObject(data []byte) (object, bool) {
	if !utf8.Valid(data) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if start, err := decoder.Token(); err != nil || start != json.Delim('{') {
		return nil, false
	}
	fields := make(object)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name := key.(string)
		var value json.RawMessage
		if fields[name] != nil || decoder.Decode(&value) != nil {
			return nil, false
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, false
	}
	_, err := decoder.Token()
	return fields, err == io.EOF
}

func member[T any](fields object, name string) (T, bool) {
	var value T
	raw := fields[name]
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return value, false
	}
	return value, true
}

func jwtObject(segment string) (object, bool) {
	raw, err := base64url.DecodeString(segment)
	if err != nil {
		return nil, false
	}
	return jsonObject(raw)
}

func (c *Client) verifyToken(ctx context.Context, token, nonce string, response object) (string, string, bool, error) {
	if len(token) > maxTokenLength || strings.ContainsAny(token, "\r\n") {
		return "", "", false, ErrInvalidResponse
	}
	headerPart, rest, ok := strings.Cut(token, ".")
	if !ok {
		return "", "", false, ErrInvalidResponse
	}
	claimsPart, signaturePart, ok := strings.Cut(rest, ".")
	if !ok || signaturePart == "" || len(signaturePart) > 1_366 {
		return "", "", false, ErrInvalidResponse
	}
	header, ok := jwtObject(headerPart)
	if !ok || !validHeader(header) {
		return "", "", false, ErrInvalidResponse
	}
	signature, err := base64url.DecodeString(signaturePart)
	if err != nil {
		return "", "", false, ErrInvalidResponse
	}
	claims, ok := jwtObject(claimsPart)
	if !ok {
		return "", "", false, ErrInvalidResponse
	}
	kid, _ := member[string](header, "kid")
	key, err := c.signingKey(ctx, kid)
	if err != nil {
		return "", "", false, err
	}
	digest := sha256.Sum256([]byte(headerPart + "." + claimsPart))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return "", "", false, ErrInvalidResponse
	}
	subject, verified, ok := c.validateClaims(claims, nonce, response)
	if !ok {
		return "", "", false, ErrInvalidResponse
	}
	return c.issuer, subject, verified, nil
}

func validHeader(header object) bool {
	alg, _ := member[string](header, "alg")
	kid, _ := member[string](header, "kid")
	if alg != "RS256" || kid == "" || len(kid) > 255 {
		return false
	}
	if header["typ"] != nil {
		typ, _ := member[string](header, "typ")
		if typ != "JWT" {
			return false
		}
	}
	for _, forbidden := range []string{"crit", "b64", "jku", "jwk", "x5u", "x5c"} {
		if header[forbidden] != nil {
			return false
		}
	}
	return true
}

func (c *Client) signingKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURI, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	response, _, err := c.requestObject(request, keysBodyLimit, "application/jwk-set+json")
	if err != nil {
		return nil, err
	}
	keys, ok := member[[]json.RawMessage](response, "keys")
	if !ok || len(keys) > 64 {
		return nil, ErrInvalidResponse
	}
	var selected object
	for _, entry := range keys {
		fields, ok := jsonObject(entry)
		if !ok {
			return nil, ErrInvalidResponse
		}
		entryKID, _ := member[string](fields, "kid")
		if entryKID == kid {
			if selected != nil {
				return nil, ErrInvalidResponse
			}
			selected = fields
		}
	}
	key := rsaKey(selected)
	if key == nil {
		return nil, ErrInvalidResponse
	}
	return key, nil
}

func rsaKey(fields object) *rsa.PublicKey {
	kty, _ := member[string](fields, "kty")
	if kty != "RSA" {
		return nil
	}
	for name, expected := range map[string]string{"alg": "RS256", "use": "sig"} {
		if fields[name] != nil {
			value, _ := member[string](fields, name)
			if value != expected {
				return nil
			}
		}
	}
	if fields["key_ops"] != nil {
		ops, _ := member[[]string](fields, "key_ops")
		if len(ops) != 1 || ops[0] != "verify" {
			return nil
		}
	}
	n := jwkUint(fields, "n", 1_366)
	e := jwkUint(fields, "e", 6)
	if n == nil || e == nil || n.BitLen() < 2048 || n.BitLen() > 8192 || n.Bit(0) == 0 ||
		e.BitLen() > 31 || e.Int64() < 3 || e.Bit(0) == 0 {
		return nil
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}
}

func jwkUint(fields object, name string, maxLength int) *big.Int {
	value, _ := member[string](fields, name)
	if len(value) == 0 || len(value) > maxLength || strings.ContainsAny(value, "\r\n") {
		return nil
	}
	raw, err := base64url.DecodeString(value)
	if err != nil || len(raw) == 0 || raw[0] == 0 {
		return nil
	}
	return new(big.Int).SetBytes(raw)
}

func (c *Client) validateClaims(claims object, nonce string, response object) (string, bool, bool) {
	issuer, _ := member[string](claims, "iss")
	subject, _ := member[string](claims, "sub")
	actualNonce, _ := member[string](claims, "nonce")
	verified, hasVerified := member[bool](claims, "email_verified")
	if issuer != c.issuer || !validSubject(subject) || !hasVerified ||
		subtle.ConstantTimeCompare([]byte(actualNonce), []byte(nonce)) != 1 || !c.validAudience(claims) {
		return "", false, false
	}
	now := c.now().Unix()
	issuedAt, hasIssuedAt := member[int64](claims, "iat")
	expires, hasExpires := member[int64](claims, "exp")
	if !hasIssuedAt || !hasExpires || issuedAt < 0 || issuedAt > now || expires <= now || expires <= issuedAt {
		return "", false, false
	}
	if claims["nbf"] != nil {
		notBefore, ok := member[int64](claims, "nbf")
		if !ok || notBefore < 0 || notBefore > now {
			return "", false, false
		}
	}
	if claims["at_hash"] != nil && !validAccessTokenHash(claims, response) {
		return "", false, false
	}
	return subject, verified, true
}

func (c *Client) validAudience(claims object) bool {
	if claims["azp"] != nil {
		party, _ := member[string](claims, "azp")
		if party != c.clientID {
			return false
		}
	}
	if audience, ok := member[string](claims, "aud"); ok {
		return audience == c.clientID
	}
	audiences, _ := member[[]string](claims, "aud")
	// The confidential client is the only trusted audience in this integration.
	return len(audiences) == 1 && audiences[0] == c.clientID
}

func validSubject(subject string) bool {
	return len(subject) <= 255 && strings.TrimSpace(subject) != "" &&
		strings.IndexFunc(subject, func(char rune) bool { return char < 0x20 || char > 0x7e }) < 0
}

func validAccessTokenHash(claims, response object) bool {
	accessToken, _ := member[string](response, "access_token")
	accessHash, _ := member[string](claims, "at_hash")
	if accessToken == "" || accessHash == "" {
		return false
	}
	digest := sha256.Sum256([]byte(accessToken))
	expected := base64url.EncodeToString(digest[:len(digest)/2])
	return subtle.ConstantTimeCompare([]byte(accessHash), []byte(expected)) == 1
}
