// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP uses HMAC-SHA1; every authenticator app supports it
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238): HMAC-SHA1, 6 digits, 30-second steps; a code of the
// step before or after is accepted too (clock skew).
const (
	totpPeriod = 30
	totpDigits = 6
	totpSkew   = 1
	// totpIssuer is the name the authenticator app shows.
	totpIssuer = "Colonyhall"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret returns 20 random bytes in base32, as authenticator apps
// expect them.
func newTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return b32.EncodeToString(raw), nil
}

// totpURI is the otpauth:// address for the authenticator app (QR code).
func totpURI(handle, secret string) string {
	label := url.PathEscape(totpIssuer + " operator:" + handle)
	q := url.Values{"secret": {secret}, "issuer": {totpIssuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// totpStep is the time step of t.
func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// totpCode is the code of a step.
func totpCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", fmt.Errorf("totp: bad secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1_000_000), nil
}

// totpMatch returns the step whose code is code, within the skew around
// now and after lastStep (each step's code is accepted once).
func totpMatch(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	cur := totpStep(now)
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		step := cur + d
		if step <= lastStep {
			continue
		}
		want, err := totpCode(secret, step)
		if err == nil && hmac.Equal([]byte(want), []byte(code)) {
			return step, true
		}
	}
	return 0, false
}

// Backup codes: 10 characters from an alphabet without look-alikes,
// shown as XXXXX-XXXXX; only the SHA-256 of the normalised code is kept.
const backupAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newBackupCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, 0, 11)
	for i, b := range raw {
		if i == 5 {
			out = append(out, '-')
		}
		out = append(out, backupAlphabet[int(b)%len(backupAlphabet)])
	}
	return string(out), nil
}

// normaliseBackupCode drops spaces and dashes and upper-cases.
func normaliseBackupCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		if r >= 'a' && r <= 'z' {
			return r - 'a' + 'A'
		}
		return r
	}, code)
}

func hashBackupCode(code string) []byte {
	sum := sha256.Sum256([]byte(normaliseBackupCode(code)))
	return sum[:]
}

// isTOTPCode: six digits (anything else is tried as a backup code).
func isTOTPCode(code string) bool {
	if len(code) != totpDigits {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CodeAt is the TOTP code of a secret at t: the e2e tests log in with it,
// as an authenticator app would.
func CodeAt(secret string, t time.Time) string {
	c, err := totpCode(secret, totpStep(t))
	if err != nil {
		return ""
	}
	return c
}
