# Golden corpus for the lectionary resolver: Reading::Resolver#resolve for
# every Prayer Book over a date window, for the default request and both
# offices, plus lectionary variants / tracks / psalm tables and a smaller
# with-content sample. Output: one JSON object per line (ActiveSupport JSON).
#   bin/rails runner tools/golden/readings.rb $OUT_FILE
require "json"
require "zlib"
out_path = ARGV.fetch(0)
out = Zlib::GzipWriter.open(out_path)

def run_case(out, date, opts)
  resolver = Reading::Resolver.for(date, **opts)
  result = resolver.resolve.to_h
  row = { date: date.to_s, opts: opts, cycle: resolver.cycle, result: result }
rescue StandardError => e
  row = { date: date.to_s, opts: opts, error: e.class.name, message: e.message }
ensure
  out.puts(ActiveSupport::JSON.encode(row))
end

books = PrayerBook.pluck(:code).sort
dates = (Date.new(2025, 11, 1)..Date.new(2027, 1, 31)).to_a
service_types = [ nil, "morning_prayer", "evening_prayer" ]
count = 0

books.each do |code|
  pb = PrayerBook.find_by_code(code)
  caps = PrayerBooks::Capabilities.for(pb)
  dates.each do |date|
    service_types.each do |st|
      run_case(out, date, { prayer_book_code: code, translation: "nvi", service_type: st, load_content: false })
      count += 1
    end
  end

  extra = []
  caps.available_lectionary_variants.each { |v| extra << { service_variant: v } }
  caps.available_reading_types.each { |t| extra << { reading_type: t } }
  %w[course monthly].each { |t| extra << { psalm_table: t } }
  extra.each do |e|
    dates.each_with_index do |date, i|
      next unless (i % 3).zero?

      service_types.each do |st|
        run_case(out, date, { prayer_book_code: code, translation: "nvi", service_type: st, load_content: false }.merge(e))
        count += 1
      end
    end
  end

  translation = pb.language.to_s.start_with?("en") ? "kjv" : "nvi"
  dates.each_with_index do |date, i|
    next unless (i % 23).zero?

    service_types.each do |st|
      run_case(out, date, { prayer_book_code: code, translation: translation, service_type: st, load_content: true })
      run_case(out, date, { prayer_book_code: code, translation: translation, psalm_translation: "coverdale", service_type: st, load_content: true }) if st
      count += 1
    end
  end
  $stderr.puts "#{code}: #{count}"
end
out.close
puts "wrote #{out_path} (#{count} cases)"
