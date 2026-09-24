package suites

import (
	"bytes"
	"net/http"

	"github.com/dodopok/estevao-api-go/internal/activestorage"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/test/diff"
)

// blobBytes is the content of the fixture blobs present in the fake S3.
var blobBytes = []byte("abcdefghijklmnopqrstuvwxyz")

var fixtureBlobKeys = map[int64]string{
	900001: "difftestavatarpng0000000001", 900002: "difftestvector00000000000002", 900003: "difftestnotes000000000000003",
	900006: "difftestnotype00000000000006",
}

func putFixtureObjects() {
	endpoint, bucket := envOr("AVATAR_BUCKET_ENDPOINT", "http://127.0.0.1:9000"), envOr("AVATAR_BUCKET_NAME", "test-avatars")
	for _, key := range fixtureBlobKeys {
		req, _ := http.NewRequest(http.MethodPut, endpoint+"/"+bucket+"/"+key, bytes.NewReader(blobBytes))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			panic("fake S3: " + err.Error())
		}
		resp.Body.Close()
	}
}

func init() {
	register("active_storage", func() []diff.Request {
		putFixtureObjects()
		h := map[string]string{"Accept": "*/*", "Host": "api.example.test"}
		jsonH := map[string]string{"Accept": "application/json", "Host": "api.example.test"}
		var out []diff.Request
		signed := func(id int64) string { return activestorage.Generate(id, "blob_id") }
		for _, id := range []int64{900001, 900002, 900003, 900004, 900005, 900006, 900007, 999999999} {
			sid := signed(id)
			for _, q := range []string{"", "?disposition=attachment", "?disposition=inline", "?disposition=bogus"} {
				out = append(out,
					diff.Request{Path: "/rails/active_storage/blobs/redirect/" + sid + "/any%20name.png" + q, Headers: h},
					diff.Request{Path: "/rails/active_storage/blobs/" + sid + "/f.png" + q, Headers: h},
					diff.Request{Path: "/rails/active_storage/blobs/proxy/" + sid + "/f.png" + q, Headers: h},
				)
			}
			out = append(out, diff.Request{Path: "/rails/active_storage/blobs/redirect/" + sid + "/f.png", Headers: jsonH})
			for _, rng := range []string{"bytes=0-3", "bytes=-2", "bytes=5-", "bytes=100-200", "bytes=3-1", "bytes=0-1,3-4", "items=0-1"} {
				out = append(out, diff.Request{Name: rng, Path: "/rails/active_storage/blobs/proxy/" + sid + "/f.png",
					Headers: map[string]string{"Accept": "*/*", "Range": rng, "Host": "api.example.test"}})
			}
			out = append(out,
				diff.Request{Path: "/rails/active_storage/representations/redirect/" + sid + "/bogus--key/f.png", Headers: h},
				diff.Request{Path: "/rails/active_storage/representations/proxy/" + sid + "/bogus--key/f.png", Headers: h},
				diff.Request{Path: "/rails/active_storage/representations/" + sid + "/bogus--key/f.png", Headers: h},
			)
		}
		for _, bad := range []string{"abc--def", "eyJfcmFpbHMiOnsiZGF0YSI6MTIzLCJwdXIiOiJibG9iX2lkIn19--0000", "x", "--", activestorage.Generate(900001, "other")} {
			out = append(out,
				diff.Request{Path: "/rails/active_storage/blobs/redirect/" + bad + "/f.png", Headers: h},
				diff.Request{Path: "/rails/active_storage/blobs/proxy/" + bad + "/f.png", Headers: jsonH},
				diff.Request{Path: "/rails/active_storage/disk/" + bad + "/f.png", Headers: h},
				diff.Request{Method: "PUT", Path: "/rails/active_storage/disk/" + bad, Headers: h, Body: "abc"},
			)
		}
		for _, svc := range []string{"production", "local", "railway_avatars", "nope"} {
			tok := activestorage.GenerateData(rb.M("key", "difftestlegacy00000000000004", "disposition", "inline; filename=\"legacy.jpg\"",
				"content_type", "image/jpeg", "service_name", svc), "blob_key", nil)
			out = append(out, diff.Request{Name: svc, Path: "/rails/active_storage/disk/" + tok + "/legacy.jpg", Headers: h})
		}
		for _, p := range []string{"mailgun/inbound_emails/mime", "mandrill/inbound_emails", "postmark/inbound_emails", "relay/inbound_emails", "sendgrid/inbound_emails"} {
			out = append(out, diff.Request{Method: "POST", Path: "/rails/action_mailbox/" + p, Headers: h, Body: "x"})
		}
		out = append(out, diff.Request{Path: "/rails/action_mailbox/mandrill/inbound_emails", Headers: jsonH})
		for _, p := range []string{"inbound_emails", "inbound_emails/new", "inbound_emails/5", "inbound_emails/sources/new"} {
			out = append(out, diff.Request{Path: "/rails/conductor/action_mailbox/" + p, Headers: h})
		}
		for _, p := range []string{"inbound_emails", "inbound_emails/sources", "5/reroute", "5/incinerate"} {
			out = append(out, diff.Request{Method: "POST", Path: "/rails/conductor/action_mailbox/" + p, Headers: h})
		}
		return out
	})
}
