package suites

import (
	"github.com/dodopok/estevao-api-go/test/diff"
)

func init() {
	registerScenarios("active_storage_writes", func() []diff.Scenario {
		h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Host": "api.example.test"}
		form := map[string]string{"Accept": "*/*", "Content-Type": "application/x-www-form-urlencoded", "Host": "api.example.test"}
		volatile := []string{"key", "created_at", "url"}
		post := func(body string, hh map[string]string) diff.Request {
			return diff.Request{Method: "POST", Path: "/rails/active_storage/direct_uploads", Headers: hh, Body: body, Volatile: volatile}
		}
		return []diff.Scenario{{
			Name:   "direct uploads",
			Setup:  `DELETE FROM active_storage_blobs WHERE filename LIKE 'difftest-du-%';`,
			Tables: []string{"active_storage_blobs"},
			Steps: []diff.Request{
				post(`{"blob":{"filename":"difftest-du-a.png","byte_size":5,"checksum":"XUFAKrxLKna5cZ2REBfFkg==","content_type":"image/png"}}`, h),
				post(`{"blob":{"filename":"difftest-du-b é.txt","byte_size":"12","checksum":"abc==","metadata":{"x":1,"y":{"z":[1,2]}}}}`, h),
				post(`{"blob":{"filename":"difftest-du-c.svg","byte_size":3,"checksum":"c==","content_type":"image/svg+xml","extra":"x"}}`, h),
				post(`{"blob":{"filename":"difftest-du-d","byte_size":3,"checksum":""}}`, h),
				post(`{"blob":{"filename":"difftest-du-e","checksum":"x"}}`, h),
				post(`{"blob":{}}`, h),
				post(`{"other":1}`, h),
				post(`{"blob":"x"}`, h),
				post(`blob[filename]=difftest-du-f.png&blob[byte_size]=7&blob[checksum]=f%3D%3D&blob[content_type]=image/png`, form),
			},
			Snapshot: []string{`SELECT id, filename, content_type, metadata, service_name, byte_size, checksum, length(key) FROM active_storage_blobs WHERE filename LIKE 'difftest-du-%' ORDER BY id`},
		}}
	})
}
