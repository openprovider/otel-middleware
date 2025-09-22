package otel

// Config holds OpenTelemetry configuration
type Config struct {
	Enabled        bool    `json:"enabled" mapstructure:"enabled"`
	Endpoint       string  `json:"endpoint" mapstructure:"endpoint"`
	ServiceName    string  `json:"serviceName" mapstructure:"serviceName"`
	ServiceVersion string  `json:"serviceVersion" mapstructure:"serviceVersion"`
	Environment    string  `json:"environment" mapstructure:"environment"`
	Protocol       string  `json:"protocol" mapstructure:"protocol"`
	Headers        string  `json:"headers" mapstructure:"headers"`
	BatchTimeout   int     `json:"batchTimeout" mapstructure:"batchTimeout"`
	SamplingRate   float64 `json:"samplingRate" mapstructure:"samplingRate"`
	AlwaysSample   bool    `json:"alwaysSample" mapstructure:"alwaysSample"`
}
