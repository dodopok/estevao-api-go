# Performs the jobs the oracle enqueued in Solid Queue (it runs no worker), in
# enqueue order, so their persisted effects can be compared with the Go
# server's. Jobs enqueued by the jobs performed here (a publication queued by
# an unpublication, say) run in a following pass, as a worker would run them;
# retries scheduled for later are left alone.
#   bin/rails runner perform_jobs.rb Audio::RecordUserUsageJob
require File.expand_path("../oracle/patches", __dir__)

classes = ARGV
count = 0
5.times do
  scope = SolidQueue::Job.where(finished_at: nil)
    .where("scheduled_at IS NULL OR scheduled_at <= ?", Time.current).order(:id)
  scope = scope.where(class_name: classes) if classes.any?
  batch = scope.to_a
  break if batch.empty?

  batch.each do |job|
    job.destroy
    begin
      ActiveJob::Base.execute(job.arguments)
    rescue StandardError => e
      warn "#{job.class_name} failed: #{e.class}: #{e.message}"
    end
    count += 1
  end
end
puts "performed #{count} jobs"
