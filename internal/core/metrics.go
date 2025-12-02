// Package core
package core

type CPUMetric struct {
	Spec      CPUSpec   `json:"spec"`
	Usage     float64   `json:"usage"`
	PerCore   []float64 `json:"per_core"`
	Watt      float64   `json:"watt"`
	Temp      float64   `json:"temp"`
	Frequency float64   `json:"frequency"`
}

type CPUSpec struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	Cores    int    `json:"cores"`
	Threads  int    `json:"threads"`
	Arch     string `json:"arch"`
	BaseFreq int    `json:"base_freq"`
	MaxFreq  int    `json:"max_freq"`
}

type GPUMetric struct {
	Spec      GPUSpec `json:"spec"`
	Usage     float64 `json:"usage"`
	VramTotal float64 `json:"vram_total"`
	VramUsed  float64 `json:"vram_used"`
	Temp      float64 `json:"temp"`
	Watt      float64 `json:"watt"`
}

type GPUSpec struct {
	Vendor string `json:"vendor"`
	Model  string `json:"model"`
}

type MemoryMetric struct {
	Specs        []MemorySpec `json:"specs"`
	MemTotal     float64      `json:"mem_total"`
	MemAvailable float64      `json:"mem_available"`
	MemUsed      float64      `json:"mem_used"`
	SwapTotal    float64      `json:"swap_total"`
	SwapFree     float64      `json:"swap_free"`
	SwapUsed     float64      `json:"swap_used"`
}

type MemorySpec struct {
	Size         string `json:"size"`
	Type         string `json:"type"`
	Speed        string `json:"speed"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"part_number"`
	FormFactor   string `json:"form_factor"`
}

type DiskMetric struct {
	Total float64 `json:"total"`
	Free  float64 `json:"free"`
	Used  float64 `json:"used"`
	Temp  float64 `json:"temp"`
}

type NetworkMetric struct {
	Upload   float64 `json:"upload"`
	Download float64 `json:"download"`
}

type SystemMetric struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	Uptime   string `json:"uptime"`
}

type Metrics struct {
	CPU     CPUMetric     `json:"cpu"`
	GPU     GPUMetric     `json:"gpu"`
	Memory  MemoryMetric  `json:"memory"`
	Disk    DiskMetric    `json:"disk"`
	Network NetworkMetric `json:"network"`
	System  SystemMetric  `json:"system"`
}
