# Wide-date golden corpus for DailyOfficeService#call: every day of a year,
# every Prayer Book and office (standard and family rite), book defaults.
# Complements daily_office.rb, whose preference sweep uses a small date sample.
#   bin/rails runner tools/golden/daily_office_year.rb $OUT_FILE YEAR [codes]
require "json"
require "zlib"
out = Zlib::GzipWriter.open(ARGV.fetch(0))
year = Integer(ARGV.fetch(1))
only = ARGV[2]&.split(",")
dates = (Date.new(year, 1, 1)..Date.new(year, 12, 31)).to_a

count = 0
PrayerBook.pluck(:code).sort.each do |code|
  next if only && !only.include?(code)

  pb = PrayerBook.find_by_code(code)
  caps = PrayerBooks::Capabilities.for(pb)
  bible = BibleVersion.default_for_language(pb.language)&.code || "nvi"
  base = { prayer_book_code: code, bible_version: bible }
  runs = caps.available_offices(family_rite: false).map { |o| [ o, base ] }
  if caps.supports_family_rite?
    runs += caps.available_offices(family_rite: true).map { |o| [ o, base.merge(family_rite: true) ] }
  end
  dates.each do |date|
    runs.each do |office, prefs|
      row = begin
        { date: date.to_s, office: office, prefs: prefs,
          result: DailyOfficeService.new(date:, office_type: office, preferences: prefs).call }
      rescue StandardError => e
        { date: date.to_s, office: office, prefs: prefs, error: e.class.name, message: e.message }
      end
      out.puts(ActiveSupport::JSON.encode(row))
      count += 1
    end
  end
  $stderr.puts "#{code}: #{count}"
end
out.close
puts "wrote (#{count} cases)"
