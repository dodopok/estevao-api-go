package s3

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)

// Authorization headers from Aws::Sigv4::Signer (service s3, uri_escape_path
// false) for the same requests.
func TestSignRequestMatchesAwsSigv4(t *testing.T) {
	c := &Client{AccessKey: "test", SecretKey: "test", Region: "auto"}
	req, _ := http.NewRequest("PUT", "http://127.0.0.1:9000/test-avatars/abc%20d/x.png", strings.NewReader("hello"))
	req.Header.Set("Content-Md5", "XUFAKrxLKna5cZ2REBfFkg==")
	req.Header.Set("Content-Type", "image/png")
	c.SignRequest(req, []byte("hello"), at)
	want := "AWS4-HMAC-SHA256 Credential=test/20260924/auto/s3/aws4_request, SignedHeaders=content-md5;content-type;host;x-amz-content-sha256;x-amz-date, Signature=46eb9252497680773af54a4d4cbc9d094783de821dcab2e8e8698991ef0e50cd"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("put:\n got  %s\n want %s", got, want)
	}
	req, _ = http.NewRequest("POST", "http://127.0.0.1:9000/test-avatars?delete=", strings.NewReader("<Delete/>"))
	req.Header.Set("Content-Type", "application/xml")
	c.SignRequest(req, []byte("<Delete/>"), at)
	want = "AWS4-HMAC-SHA256 Credential=test/20260924/auto/s3/aws4_request, SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date, Signature=56f6fc39cdc0485667989d0ab5df3da77ce177fd65cc737de86d1fe596beb438"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("post:\n got  %s\n want %s", got, want)
	}
}

func TestPresignedGetMatchesRails(t *testing.T) {
	c := &Client{AccessKey: "test", SecretKey: "test", Region: "auto", Bucket: "test-avatars", Endpoint: "http://127.0.0.1:9000"}
	key := "606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3"
	got := c.PresignedGet("audio/tts/openai/"+key, 3600, `inline; filename="`+key+`"; filename*=UTF-8''`+key, "audio/mpeg", at)
	want := "http://127.0.0.1:9000/test-avatars/audio/tts/openai/606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3?response-content-disposition=inline%3B%20filename%3D%22606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3%22%3B%20filename%2A%3DUTF-8%27%27606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3&response-content-type=audio%2Fmpeg&X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=test%2F20260924%2Fauto%2Fs3%2Faws4_request&X-Amz-Date=20260924T060000Z&X-Amz-Expires=3600&X-Amz-SignedHeaders=host&X-Amz-Signature=6891ba955a8ca68edb52f76dfd8be81b4fd64e989fadf17effed2c622003aee3"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestPresignedPutMatchesRails(t *testing.T) {
	c := &Client{AccessKey: "test", SecretKey: "test", Region: "auto", Bucket: "test-avatars", Endpoint: "http://127.0.0.1:9000"}
	got := c.PresignedPut("i8f53s9bircsezmtvg8hgt0sbbj3", 300, "image/png", 5, "XUFAKrxLKna5cZ2REBfFkg==", at)
	want := "http://127.0.0.1:9000/test-avatars/i8f53s9bircsezmtvg8hgt0sbbj3?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=test%2F20260924%2Fauto%2Fs3%2Faws4_request&X-Amz-Date=20260924T060000Z&X-Amz-Expires=300&X-Amz-SignedHeaders=content-length%3Bcontent-md5%3Bcontent-type%3Bhost&X-Amz-Signature=d1807419fca0e11e192a72208b93a599d5ca5f3ae8d028939ab93be71faa3f7a"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
