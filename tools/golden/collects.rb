# Golden corpus for CollectService#find_collects over every Prayer Book.
#   bin/rails runner tools/golden/collects.rb $OUT_FILE
require "json"
require "zlib"
out = Zlib::GzipWriter.open(ARGV.fetch(0))
count = 0
PrayerBook.pluck(:code).sort.each do |code|
  (Date.new(2025, 11, 1)..Date.new(2027, 1, 31)).each_with_index do |date, i|
    variants = [ { office_type: :morning, language_style: nil, include_fixed_office_collect: false } ]
    if (i % 4).zero?
      variants << { office_type: :evening, language_style: "traditional", include_fixed_office_collect: true }
      variants << { office_type: :morning, language_style: "contemporary", include_fixed_office_collect: true }
    end
    variants.each do |v|
      opts = { prayer_book_code: code }.merge(v)
      row = begin
        { date: date.to_s, opts: opts, result: CollectService.new(date, **opts).send(:find_collects_uncached) }
      rescue StandardError => e
        { date: date.to_s, opts: opts, error: e.class.name, message: e.message }
      end
      out.puts(ActiveSupport::JSON.encode(row))
      count += 1
    end
  end
  $stderr.puts "#{code}: #{count}"
end
out.close
puts "wrote (#{count} cases)"
