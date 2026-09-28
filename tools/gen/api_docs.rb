# Writes internal/apidocs/assets from what the Rails app mounts at /api-docs:
# the OpenAPI files as Rswag::Api serves them (parsed, filtered, dumped again,
# once per server list the filter can produce), the Swagger UI page rendered
# from Rswag::Ui's template, and the swagger-ui-dist files Rack::Static serves
# (with their modification times and Rack::Mime types).
#   bin/rails runner tools/gen/api_docs.rb /path/to/estevao-api-go
require "fileutils"
require "json"
require "yaml"

go_root = ARGV.fetch(0)
out = File.join(go_root, "internal/apidocs/assets")
FileUtils.rm_rf(out)
FileUtils.mkdir_p(out)

manifest = { "api" => [], "ui" => [] }

# Rswag::Api: the configured filter only sets "servers", from Rails.env.
api_config = Rswag::Api.config
root = api_config.resolve_openapi_root({})
servers = {
  "production" => [ { "url" => "https://api.caminhoanglicano.com.br", "description" => "Production server" } ],
  "development" => [ { "url" => "http://localhost:3000", "description" => "Development server" } ]
}
Dir.glob(File.join(root, "**", "*")).sort.each do |filename|
  next unless File.file?(filename)

  path = filename.delete_prefix(root.to_s)
  yaml = /\.ya?ml$/.match?(filename)
  entry = { "path" => path, "type" => Rack::Mime.mime_type(File.extname(path), "text/plain"), "bodies" => {} }
  servers.each do |env, list|
    swagger = yaml ? YAML.safe_load(File.read(filename)) : JSON.parse(File.read(filename))
    swagger["servers"] = list if swagger.is_a?(Hash)
    body = yaml ? YAML.dump(swagger) : JSON.dump(swagger)
    name = "api/#{env}#{path}"
    FileUtils.mkdir_p(File.dirname(File.join(out, name)))
    File.binwrite(File.join(out, name), body)
    entry["bodies"][env] = name
  end
  manifest["api"] << entry
end

# Rswag::Ui: the rendered index and the static distribution.
ui_config = Rswag::Ui.config
template = ui_config.template_locations.find { |f| File.exist?(f) }
File.binwrite(File.join(out, "ui_index.html"), ERB.new(File.read(template)).result(ui_config.get_binding))
assets = ui_config.assets_root
Dir.glob(File.join(assets, "**", "*"), File::FNM_DOTMATCH).sort.each do |filename|
  next unless File.file?(filename)

  path = filename.delete_prefix(assets)
  name = "ui#{path}"
  FileUtils.mkdir_p(File.dirname(File.join(out, name)))
  FileUtils.cp(filename, File.join(out, name))
  manifest["ui"] << { "path" => path, "file" => name, "type" => Rack::Mime.mime_type(File.extname(path), "text/plain"),
                      "mtime" => File.mtime(filename).httpdate }
end

File.write(File.join(out, "manifest.json"), JSON.pretty_generate(manifest))
puts "wrote #{out} (#{manifest['api'].size} OpenAPI files, #{manifest['ui'].size} UI files)"
