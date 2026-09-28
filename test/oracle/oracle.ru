# Runs the Rails application as the differential-testing oracle WITHOUT
# modifying the Rails repository: this rackup file boots the app from its
# own path and replaces outbound integrations with deterministic fakes.
#
#   cd $RAILS_ROOT && bundle exec puma -p 3000 $THIS_DIR/oracle.ru
require File.join(ENV.fetch("RAILS_ROOT"), "config/environment")

require File.join(__dir__, "patches")

run Rails.application
