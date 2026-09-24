# Golden corpus for rb.TimeParse: Ruby's Time.parse(s, now) under two
# process time zones. Run twice:
#   TZ=UTC ruby timeparse_corpus.rb out_utc.jsonl
#   TZ=America/New_York ruby timeparse_corpus.rb out_ny.jsonl
require "time"
require "json"
srand(20260924)
dates = %w[2026-10-01 2026-02-29 2024-02-29 2026-02-30 2026-04-31 2026-13-01 20261001 2026-274 2026-W40-4 Oct\ 1\ 2026 1\ Oct\ 2026 10/01/2026 26-10-01]
times = ["T12:00:00Z", "T12:00:00.123Z", "T12:00:00.123456789Z", " 12:00:00 +0300", "T23:59:60Z", "T24:00:00Z", "T24:00:01Z",
         "T12:00:00-03:00", "T12:00:00+05:30", "T12:00:00+0530", "T12:00:00+05", "T12:00:00 EST", "T12:00:00 PDT", "T12:00 pm",
         " 9am", "T12:00:00", "", "T00:00:00-12:00", "T23:00:00+14:00", "T12:00:00 GMT", "T12:00:00 BRT", "T12:00:00 Z",
         "T12:00:00+05:30:15", "T12:00:00 utc+3", "T12:00:00[+9]", "T120000Z", "T12:00:00,5Z", " 10h30", "T12:00:00 J"]
inputs = []
dates.each { |d| times.each { |t| inputs << d + t } }
inputs += ["12:00", "12:00:00.5", "Z", "abc", "", "2026", "Oct", "5th", "2026-274T01:00:00Z", "2026-366T00:00:00Z", "2024-366T00:00:00Z",
           "2026-10-01T12:00:00.000Z", "2019-07-26T23:50:40Z", "2099-12-31T23:59:59Z", "1969-12-31T23:59:59Z",
           "Thu, 01 Oct 2026 12:00:00 GMT", "2026-03-08T02:30:00", "2026-11-01T01:30:00", "20261001T120000Z", "20261001120000"]
now = Time.at(1790251200, 123456789, :nsec) # 2026-09-24T12:00:00.123456789Z
File.open(ARGV[0], "w") do |f|
  inputs.each do |s|
    r = begin
      t = Time.parse(s, now)
      "#{t.to_i}.#{format('%09d', t.nsec)}"
    rescue ArgumentError, RangeError => e
      "ERR:#{e.message}"
    end
    f.puts({ s: s, r: r }.to_json)
  end
end
puts inputs.size
