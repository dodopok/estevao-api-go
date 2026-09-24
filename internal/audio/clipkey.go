package audio

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// clipNamespace is Audio::ClipKey::NAMESPACE.
const clipNamespace = "3f6d4a8e-5b21-4c0e-9d77-1a2b3c4d5e6f"

// ClipKey ports Audio::ClipKey.for: an RFC 4122 v5 UUID of the voice, the
// provider signature and the normalized text. Rails runs with
// use_rfc4122_namespaced_uuids (load_defaults 8.1), so the namespace is
// hashed as its 16 bytes.
func ClipKey(p *Provider, normalized string) string {
	return uuidV5(clipNamespace, p.Voice+" "+p.CacheSignature(&normalized)+" "+normalized)
}

func uuidV5(namespace, name string) string {
	ns, err := hex.DecodeString(strings.ReplaceAll(namespace, "-", ""))
	if err != nil || len(ns) != 16 {
		panic("audio: bad uuid namespace")
	}
	h := sha1.New()
	h.Write(ns)
	h.Write([]byte(name))
	sum := h.Sum(nil)[:16]
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// TextDigest ports Audio::Customization.text_digest.
func TextDigest(normalized string) string {
	s := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(s[:])
}
