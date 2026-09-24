# Runs the Rails application as the differential-testing oracle WITHOUT
# modifying the Rails repository: this rackup file boots the app from its
# own path and replaces outbound integrations with deterministic fakes.
#
#   cd $RAILS_ROOT && bundle exec puma -p 3000 $THIS_DIR/oracle.ru
require File.join(ENV.fetch("RAILS_ROOT"), "config/environment")

ORACLE_DIR = File.expand_path(__dir__)

# Firebase: serve the test certificate for kid "test-kid" (both the app and
# the developer portal projects), so tokens minted by the harness verify.
module OracleFirebaseCertificates
  def certificates
    body = { "test-kid" => File.read(File.join(ORACLE_DIR, "test_firebase_cert.pem")) }.to_json
    response = Net::HTTPOK.new("1.1", "200", "OK")
    response.instance_variable_set(:@read, true)
    response.instance_variable_set(:@body, body)
    response
  end
end
Integrations::Firebase::Client.prepend(OracleFirebaseCertificates)

run Rails.application
