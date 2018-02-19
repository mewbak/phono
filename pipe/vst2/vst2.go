package vst2

import (
	"context"

	"github.com/dudk/phono"
	"github.com/dudk/phono/vst2"
)

// VST2 represents vst2 sound processor
type VST2 struct {
	plugin vst2.Plugin
}

//NewProcessor creates new vst2 processor
func NewProcessor(plugin vst2.Plugin) *VST2 {
	return &VST2{plugin: plugin}
}

//Process implements processor.Processor
func (v VST2) Process(ctx context.Context, in <-chan phono.Message) (<-chan phono.Message, <-chan error, error) {
	errc := make(chan error, 1)
	out := make(chan phono.Message)
	go func() {
		defer close(out)
		defer close(errc)
		for in != nil {
			select {
			case message, ok := <-in:
				if !ok {
					in = nil
				} else {
					v.plugin.Process(message.Samples().Samples)
					out <- message
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, errc, nil
}