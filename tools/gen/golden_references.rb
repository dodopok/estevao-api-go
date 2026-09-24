# Golden corpus for the Bible reference parser/normalizer: every distinct
# reference string in lectionary_readings, parsed by the Rails code.
#   bin/rails runner tools/gen/golden_references.rb $GO_ROOT
require "json"
require "zlib"
GO_ROOT = ARGV.fetch(0)
cols = %w[first_reading psalm psalm_alternative second_reading gospel second_reading_alternative]
refs = cols.flat_map { |c| LectionaryReading.distinct.pluck(c) }.compact.uniq
refs += LiturgicalText.where("slug LIKE ?", "%").pluck(:reference).compact.uniq rescue nil
refs = refs.compact.uniq.sort
out = Zlib::GzipWriter.open(File.join(GO_ROOT, "test/golden/references.jsonl.gz"))
refs.each do |ref|
  segs = Bible::ReferenceParser.parse_all(ref).map do |s|
    { book: s[:book], chapter: s[:chapter], verse_start: s[:verse_start], verse_end: s[:verse_end],
      fetch_all: !!s[:fetch_all], fetch_all_from_verse: !!s[:fetch_all_from_verse] }
  end
  out.puts({ ref: ref, normalized: Bible::ReferenceNormalizer.call(ref), segments: segs }.to_json)
end
out.close
puts "wrote test/golden/references.jsonl.gz (#{refs.size} references)"
