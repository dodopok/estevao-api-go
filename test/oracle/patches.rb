# Replaces the Rails app's outbound integrations with the deterministic fakes
# of the differential tests. Loaded by oracle.ru (the server) and by
# test/effects/perform_jobs.rb (the jobs performed outside it), so both call
# the fakes. The Rails repository itself is not modified.

ORACLE_DIR = File.expand_path(__dir__) unless defined?(ORACLE_DIR)

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


# Firebase Admin: send the service-account token exchange and the account
# deletion to the fake Google server (test/oracle/fake_google.py). The
# credential checks and the request logic run unchanged.
if (fake_google = ENV["ORACLE_FAKE_GOOGLE_URL"].presence)
  require "googleauth"
  Google::Auth::ServiceAccountCredentials.prepend(Module.new do
    define_method(:token_credential_uri) { Addressable::URI.parse("#{fake_google}/token") }
  end)
  Integrations::Firebase::Client.prepend(Module.new do
    define_method(:delete_user) do |uid:|
      uri = URI("#{fake_google}/v1/projects/#{@project_id}/accounts:delete")
      request = Net::HTTP::Post.new(uri)
      request["Authorization"] = "Bearer #{@access_token}"
      request["Content-Type"] = "application/json"
      request.body = { localId: uid }.to_json
      http = @http_factory.net_http(uri, env_key: "FIREBASE_HTTP_TIMEOUT")
      Integrations::RetryPolicy.call(operation: "firebase.delete_user", max_attempts: 1, error_code: "FIREBASE_UNAVAILABLE") do
        http.request(request)
      end
    end
  end)
end

# FCM (Net::HTTP through Integrations::HttpClient): connect to the fake
# instead of fcm.googleapis.com; the request path is unchanged.
if (fake_fcm = ENV["FCM_API_URL"].presence)
  fcm = URI(fake_fcm)
  Integrations::HttpClient.singleton_class.prepend(Module.new do
    define_method(:net_http) do |uri, **opts|
      uri = URI::HTTP.build(host: fcm.host, port: fcm.port) if uri.host == "fcm.googleapis.com"
      super(uri, **opts)
    end
  end)
end

# Outbound calls made with http.rb (RevenueCat, ...): send the production
# base URLs to the fakes named by the same variables the Go server reads.
ORACLE_HTTP_REWRITES = {
  "https://api.revenuecat.com/v1" => ENV["REVENUECAT_API_URL"].presence,
  "https://api.perplexity.ai" => ENV["PERPLEXITY_API_URL"].presence
}.compact.freeze
unless ORACLE_HTTP_REWRITES.empty?
  require "http"
  HTTP::Client.prepend(Module.new do
    define_method(:request) do |verb, uri, opts = {}|
      target = uri.to_s
      ORACLE_HTTP_REWRITES.each do |from, to|
        if target.start_with?(from)
          target = to + target.delete_prefix(from)
          break
        end
      end
      super(verb, target, opts)
    end
  end)
end
