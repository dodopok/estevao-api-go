# Enqueues each named job class with no arguments (as the recurring schedule
# does), then performs the named classes like perform_jobs.rb.
#   bin/rails runner enqueue_jobs.rb FlushApiKeyUsageJob DatabaseCleanupJob
require File.expand_path("../oracle/patches", __dir__)

ARGV.each { |name| name.constantize.perform_later }
load File.expand_path("perform_jobs.rb", __dir__)
