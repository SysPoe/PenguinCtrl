package health

// Collector provides one or more current component observations.
type Collector interface {
	Collect() []Component
}

// CollectorFunc adapts a function into a Collector.
type CollectorFunc func() []Component

// Collect calls f.
func (f CollectorFunc) Collect() []Component { return f() }

// CollectAll flattens the observations from each registered collector.
func CollectAll(collectors ...Collector) []Component {
	var components []Component
	for _, collector := range collectors {
		if collector != nil {
			components = append(components, collector.Collect()...)
		}
	}
	return components
}
