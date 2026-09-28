# Dumps, for every controller the app routes to, the options
# ActionController::ParamsWrapper uses (wrapper key, formats and the include
# list derived from the model), as JSON for internal/web/params_wrapper_gen.go.
#   bin/rails runner tools/gen/params_wrapper.rb > out.json
require "json"
Rails.application.eager_load!
out = {}
Rails.application.routes.routes.each do |route|
  controller = route.defaults[:controller]
  next if controller.blank? || out.key?(controller)
  klass = "#{controller.camelize}Controller".safe_constantize
  next unless klass && klass < ActionController::Metal && klass.respond_to?(:_wrapper_options)
  options = klass._wrapper_options
  next if options.format.blank?
  out[controller] = { name: options.name, format: options.format.map(&:to_s), include: options.include,
                      exclude: options.exclude }
end
puts JSON.pretty_generate(out.sort.to_h)
