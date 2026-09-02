//go:build !linux

package monitoring

type ProcSampler struct{}

func NewProcSampler() *ProcSampler                           { return &ProcSampler{} }
func (*ProcSampler) Sample(process Process) (Process, error) { return process, nil }
