# Randomized golden corpus for DailyOfficeService#call: per Prayer Book,
# CASES offices on random dates (2025-2028) with a random combination of the
# book's select preferences, sometimes an explicit seed and a list value.
# Deterministic for a given SEED.
#   bin/rails runner tools/golden/daily_office_fuzz.rb $OUT_FILE [CASES] [SEED] [codes]
require "json"
require "zlib"
out = Zlib::GzipWriter.open(ARGV.fetch(0))
cases = Integer(ARGV[1] || 200)
rng = Random.new(Integer(ARGV[2] || 20260924))
only = ARGV[3]&.split(",")
first, last = Date.new(2025, 1, 1), Date.new(2028, 12, 31)

count = 0
PrayerBook.pluck(:code).sort.each do |code|
  next if only && !only.include?(code)
  next unless DailyOffice::Registry.codes.include?(code)

  pb = PrayerBook.find_by_code(code)
  caps = PrayerBooks::Capabilities.for(pb)
  bible = BibleVersion.default_for_language(pb.language)&.code || "nvi"
  standard = caps.available_offices(family_rite: false)
  family = caps.supports_family_rite? ? caps.available_offices(family_rite: true) : []
  definitions = Preferences::DefinitionSet.for(pb).definitions.filter_map do |d|
    next if %w[prayer_book_code bible_version].include?(d.key.to_s)
    options = Array(d.options).map { |o| o.is_a?(Hash) ? (o["value"] || o[:value]) : o }.compact.map(&:to_s).uniq
    [ d.key.to_s, options ] if options.any?
  end

  cases.times do
    date = first + rng.rand((last - first).to_i + 1)
    fam = family.any? && rng.rand < 0.25
    office = (fam ? family : standard).sample(random: rng)
    prefs = { prayer_book_code: code, bible_version: bible }
    prefs[:family_rite] = true if fam
    definitions.each do |key, options|
      next unless rng.rand < 0.6
      prefs[key.to_sym] = if rng.rand < 0.1 && options.size > 1
        options.sample(2, random: rng)
      else
        options.sample(random: rng)
      end
    end
    prefs[:seed] = rng.rand(1_000_000) if rng.rand < 0.3
    row = begin
      { date: date.to_s, office: office, prefs: prefs,
        result: DailyOfficeService.new(date:, office_type: office, preferences: prefs).call }
    rescue StandardError => e
      { date: date.to_s, office: office, prefs: prefs, error: e.class.name, message: e.message }
    end
    out.puts(ActiveSupport::JSON.encode(row))
    count += 1
  end
  $stderr.puts "#{code}: #{count}"
end
out.close
puts "wrote (#{count} cases)"
