// Package metrics lets domain code record the custom metrics the Rails app
// sent to New Relic (NewRelic::Agent.record_metric) without depending on an
// agent: Record is a no-op until internal/observe installs the agent.
package metrics

// Recorder receives a metric name without the "Custom/" prefix
// ("API/calendar#day/Duration") and a value.
var Recorder func(name string, value float64)

// Record ports NewRelic::Agent.record_metric("Custom/"+name, value).
func Record(name string, value float64) {
	if Recorder != nil {
		Recorder(name, value)
	}
}

// Increment ports NewRelic::Agent.increment_metric("Custom/"+name).
func Increment(name string) { Record(name, 1) }
