# Performs the jobs the oracle enqueued in Solid Queue (it runs no worker), in
# enqueue order, so their persisted effects can be compared with the Go
# server's. Usage: bin/rails runner perform_jobs.rb Audio::RecordUserUsageJob
classes = ARGV
scope = SolidQueue::Job.where(finished_at: nil).order(:id)
scope = scope.where(class_name: classes) if classes.any?
count = 0
scope.each do |job|
  ActiveJob::Base.execute(job.arguments)
  job.destroy
  count += 1
end
puts "performed #{count} jobs"
