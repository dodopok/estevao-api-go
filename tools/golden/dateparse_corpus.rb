require "date"
require "json"
srand(20260924)
parts = %w[2026 26 04 4 05 5 12 31 30 2 2019 1999 99 69 70 00 0 13 32 366 095 2026095 20260405 260405 0405 405 1231 -04 +2026 '26 '2026]
seps = ["-", "/", ".", " ", ", ", "", "T", ":", "--", "-W", "W"]
words = %w[jan February mar Apr may JUNE jul aug Sept oct nov dec sunday Mon tue wednesday thu fri sat bc BCE ad am pm st nd rd th h m s utc gmt est z abc xyz Heisei H30 R02 S64]
extras = ["10:00", "23:59:59", "9am", "12 pm", "+03:00", "T10:00:00Z", "10h30", " ", "@", "[", "]"]
inputs = []
2000.times do
  n = 1 + rand(4)
  s = +""
  n.times do |i|
    r = rand
    s << (r < 0.6 ? parts.sample : r < 0.85 ? words.sample : extras.sample)
    s << seps.sample if i < n - 1
  end
  inputs << s
end
inputs += ["2026-04-05", "2026-4-5", "20260405", "2026/04/05", "05/04/2026", "04/05/2026", "5 Apr 2026", "April 5, 2026", "Apr 5", "2026-04-05T10:00:00Z", "2026-13-01", "2026-02-30", "abc", "", "  2026-04-05 ", "26-04-05", "05.04.2026", "2026.04.05", "Sunday", "5", "15", "2026", "0405", "H30.04.05", "2026-W14-7", "2026-095", "monday 2026-04-05", "2026-04-05 garbage", "x2026-04-05", "10:00", "Apr", "5th", "'26", "2026-W14", "--04-05", "--0405", "-W-3", "5 Apr 2026 BC", "2026-02-29", "2024-02-29", "31/12/1999"]
File.open(ARGV[0], "w") do |f|
  inputs.each do |s|
    [true, false].each do |comp|
      r = begin; Date.parse(s, comp).iso8601; rescue Date::Error, ArgumentError => e; "ERR"; end
      f.puts({ s: s, comp: comp, r: r }.to_json)
    end
  end
end
puts inputs.size
