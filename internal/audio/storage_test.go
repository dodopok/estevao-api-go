package audio

import (
	"bufio"
	"compress/gzip"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// The oracle's presigned_url(:get, time: 2026-09-24 06:00:00 UTC) for the
// test bucket of test/oracle/oracle.env.
func TestPresignedURLMatchesRails(t *testing.T) {
	svc := &s3Service{accessKey: "test", secretKey: "test", region: "auto", bucket: "test-avatars", endpoint: "http://127.0.0.1:9000"}
	key := "606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3"
	got := svc.presignedGet("audio/tts/openai/"+key, 3600, key, time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC))
	want := "http://127.0.0.1:9000/test-avatars/audio/tts/openai/606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3?response-content-disposition=inline%3B%20filename%3D%22606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3%22%3B%20filename%2A%3DUTF-8%27%27606406e2-d44b-5b23-86e1-d0c39aa59d11.mp3&response-content-type=audio%2Fmpeg&X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=test%2F20260924%2Fauto%2Fs3%2Faws4_request&X-Amz-Date=20260924T060000Z&X-Amz-Expires=3600&X-Amz-SignedHeaders=host&X-Amz-Signature=6891ba955a8ca68edb52f76dfd8be81b4fd64e989fadf17effed2c622003aee3"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// testdata/round3.txt.gz holds "rational rounded" pairs from Ruby's
// Float#round(3) on 20000 random floats.
func TestRoundMatchesRuby(t *testing.T) {
	f, err := os.Open("testdata/round3.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(gz)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		r, ok := new(big.Rat).SetString(parts[0])
		if !ok {
			t.Fatal(parts[0])
		}
		x, _ := r.Float64()
		want, _ := strconv.ParseFloat(parts[1], 64)
		if got := rb.RoundFloat(x, 3); got != want {
			t.Errorf("%v.round(3): ruby %v go %v", x, want, got)
		}
	}
}
