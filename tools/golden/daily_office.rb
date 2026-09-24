# Golden corpus for DailyOfficeService#call (anonymous caller): every Prayer
# Book and office (standard and family rite) over a date sample with the
# book's defaults, then every option of every select preference on two dates.
#   bin/rails runner tools/golden/daily_office.rb $OUT_FILE [codes]
require "json"
require "zlib"
out = Zlib::GzipWriter.open(ARGV.fetch(0))
only = ARGV[1]&.split(",")
DATES = %w[
  2025-11-30 2025-12-08 2025-12-24 2025-12-25 2025-12-28 2026-01-01 2026-01-06 2026-01-11 2026-01-25
  2026-02-02 2026-02-15 2026-02-18 2026-02-22 2026-03-19 2026-03-25 2026-03-29 2026-04-01 2026-04-02
  2026-04-03 2026-04-04 2026-04-05 2026-04-06 2026-04-12 2026-05-14 2026-05-17 2026-05-24 2026-05-27
  2026-05-31 2026-06-04 2026-06-24 2026-06-29 2026-07-22 2026-08-06 2026-08-15 2026-09-16 2026-09-29
  2026-11-01 2026-11-02 2026-11-22 2026-12-31
].map { |d| Date.parse(d) }.freeze

def run(out, code, date, office, prefs)
  row = begin
    { date: date.to_s, office: office, prefs: prefs,
      result: DailyOfficeService.new(date:, office_type: office, preferences: prefs).call }
  rescue StandardError => e
    { date: date.to_s, office: office, prefs: prefs, error: e.class.name, message: e.message }
  end
  out.puts(ActiveSupport::JSON.encode(row))
end

count = 0
PrayerBook.pluck(:code).sort.each do |code|
  next if only && !only.include?(code)

  pb = PrayerBook.find_by_code(code)
  caps = PrayerBooks::Capabilities.for(pb)
  bible = BibleVersion.default_for_language(pb.language)&.code || "nvi"
  base = { prayer_book_code: code, bible_version: bible }
  standard = caps.available_offices(family_rite: false)
  family = caps.supports_family_rite? ? caps.available_offices(family_rite: true) : []

  DATES.each do |date|
    standard.each { |o| run(out, code, date, o, base); count += 1 }
    family.each { |o| run(out, code, date, o, base.merge(family_rite: true)); count += 1 }
  end

  definitions = Preferences::DefinitionSet.for(pb).definitions
  definitions.each do |d|
    options = Array(d.options).map { |o| o.is_a?(Hash) ? (o["value"] || o[:value]) : o }.compact.map(&:to_s).uniq
    next if options.empty? || d.key.to_s == "prayer_book_code" || d.key.to_s == "bible_version"

    offices = (standard + family).uniq.select { |o| d.key.to_s.start_with?("#{o}_") || d.key.to_s.start_with?("family_#{o}_") }
    offices = standard if offices.empty?
    options.each_with_index do |value, i|
      [ DATES[(i * 7) % DATES.size], DATES[(i * 7 + 19) % DATES.size] ].each do |date|
        offices.each do |o|
          prefs = base.merge(d.key.to_sym => value)
          prefs[:family_rite] = true if family.include?(o) && (!standard.include?(o) || d.key.to_s.start_with?("family_"))
          run(out, code, date, o, prefs)
          count += 1
        end
      end
    end
  end
  $stderr.puts "#{code}: #{count}"
end
out.close
puts "wrote (#{count} cases)"
