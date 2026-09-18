package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxSignatureAge and minSignatureAge bound how stale or skewed a webhook
// timestamp may be before it's rejected outright, per Zavu's own
// documented scheme (https://docs.zavu.dev/guides/receiving-messages/security).
const (
	maxSignatureAge = 5 * time.Minute
	minSignatureAge = -1 * time.Minute
)

// VerifySignature checks an inbound Zavu webhook's X-Zavu-Signature header
// (format "t=<unix_seconds>,v2=<hex_hmac_sha256>") against rawBody using
// secret. The v2 scheme hashes "{timestamp}.{raw_body}" with HMAC-SHA256 --
// rawBody must be the exact bytes Zavu sent, before any JSON
// parse/re-serialize, since the signature covers those exact bytes.
// Comparison is constant-time to avoid a timing side-channel.
func VerifySignature(header string, rawBody []byte, secret string) error {
	ts, sig, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	age := time.Since(time.Unix(ts, 0))
	if age > maxSignatureAge {
		return fmt.Errorf("webhook signature timestamp too old: %s", age)
	}
	if age < minSignatureAge {
		return fmt.Errorf("webhook signature timestamp too far in the future: %s", -age)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(fmt.Appendf(nil, "%d.", ts))
	mac.Write(rawBody)
	expected := mac.Sum(nil)

	got, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("decode webhook signature: %w", err)
	}
	if !hmac.Equal(expected, got) {
		return fmt.Errorf("webhook signature mismatch")
	}
	return nil
}

// parseSignatureHeader extracts t= and v2= (preferring v2 over the legacy
// v1 scheme) from a header like "t=1786113454,v2=b4b2b6...".
func parseSignatureHeader(header string) (ts int64, sig string, err error) {
	var tsStr, v1, v2 string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			tsStr = kv[1]
		case "v1":
			v1 = kv[1]
		case "v2":
			v2 = kv[1]
		}
	}
	if tsStr == "" {
		return 0, "", fmt.Errorf("webhook signature header missing timestamp")
	}
	ts, err = strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse webhook signature timestamp: %w", err)
	}
	if v2 != "" {
		return ts, v2, nil
	}
	if v1 != "" {
		return 0, "", fmt.Errorf("webhook signature header only has the legacy v1 scheme, which this verifier does not support")
	}
	return 0, "", fmt.Errorf("webhook signature header missing v2 signature")
}
