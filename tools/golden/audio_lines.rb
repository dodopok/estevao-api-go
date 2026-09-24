# Golden corpus for the narration text pipeline: every distinct spoken line of
# a Daily Office corpus (tools/golden/daily_office_*.rb output), with what
# Rails derives from it: context_required?, Normalizer output, provider
# segments and clip keys (OpenAI and Google profiles).
#   bin/rails runner tools/golden/audio_lines.rb $OFFICE_CORPUS $OUT_FILE
require "json"
require "zlib"

providers = Hash.new do |h, (name, lang)|
  h[[ name, lang ]] = Audio::Providers.build(name, language: lang)
end
generators = Hash.new { |h, provider| h[provider] = Audio::ClipGenerator.new(provider:, storage: Object.new) }
seen = {}
out = Zlib::GzipWriter.open(ARGV.fetch(1))
count = 0
Zlib::GzipReader.open(ARGV.fetch(0)) do |gz|
  gz.each_line do |line|
    row = JSON.parse(line)
    result = row["result"] or next
    language = result.dig("metadata", "language") || PrayerBook.find_by(code: row.dig("prefs", "prayer_book_code"))&.language
    Audio::OfficeWalker.each_line(result) do |entry|
      next unless Audio::SpeakableLine::SPOKEN.include?(entry.type) && entry.text.present?

      id = [ entry.text, entry.type, entry.slug, entry.verse_number, language ]
      next if seen[id]

      seen[id] = true
      record = { text: entry.text, type: entry.type, slug: entry.slug, verse_number: entry.verse_number, language: language }
      record[:context_required] = Audio::ContextualText.context_required?(entry.text, language: language)
      record[:normalized] = Audio::Normalizer.call(entry.text, line_type: entry.type, slug: entry.slug,
        verse_number: entry.verse_number, language: language)
      %w[openai google].each do |name|
        provider = providers[[ name, language ]]
        segments = generators[provider].segments_for(entry.text, line_type: entry.type, slug: entry.slug,
          verse_number: entry.verse_number)
        record[name] = segments.map { |s| [ s, Audio::ClipKey.for(provider:, normalized: s) ] }
      end
      out.puts(record.to_json)
      count += 1
    end
  end
end
out.close
puts "wrote #{count} lines"
