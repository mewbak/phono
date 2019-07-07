package pipe

import (
	"io"
)

// pumpRunner is pump's runner.
type pumpRunner struct {
	fn func(bufferSize int) ([][]float64, error)
	hooks
}

// processRunner represents processor's runner.
type processRunner struct {
	fn func([][]float64) ([][]float64, error)
	hooks
}

// sinkRunner represents sink's runner.
type sinkRunner struct {
	fn func([][]float64) error
	hooks
}

// Flusher defines component that must flushed in the end of execution.
type Flusher interface {
	Flush(string) error
}

// Interrupter defines component that has custom interruption logic.
type Interrupter interface {
	Interrupt(string) error
}

// Resetter defines component that must be resetted before consequent use.
type Resetter interface {
	Reset(string) error
}

// hook represents optional functions for components lyfecycle.
type hook func(string) error

// set of hooks for runners.
type hooks struct {
	flush     hook
	interrupt hook
	reset     hook
}

// bindHooks of component.
func bindHooks(v interface{}) hooks {
	return hooks{
		flush:     flusher(v),
		interrupt: interrupter(v),
		reset:     resetter(v),
	}
}

var do struct{}

// flusher checks if interface implements Flusher and if so, return it.
func flusher(i interface{}) hook {
	if v, ok := i.(Flusher); ok {
		return v.Flush
	}
	return nil
}

// flusher checks if interface implements Flusher and if so, return it.
func interrupter(i interface{}) hook {
	if v, ok := i.(Interrupter); ok {
		return v.Interrupt
	}
	return nil
}

// flusher checks if interface implements Flusher and if so, return it.
func resetter(i interface{}) hook {
	if v, ok := i.(Resetter); ok {
		return v.Reset
	}
	return nil
}

// bindPump creates the closure. it's separated from run to have pre-run
// logic executed in correct order for all components.
func bindPump(pipeID string, p Pump) (*pumpRunner, int, int, error) {
	fn, sampleRate, numChannels, err := p.Pump(pipeID)
	if err != nil {
		return nil, 0, 0, err
	}
	r := pumpRunner{
		fn:    fn,
		hooks: bindHooks(p),
	}
	return &r, sampleRate, numChannels, nil
}

// run the Pump runner.
func (r *pumpRunner) run(bufferSize int, pipeID, componentID string, cancel <-chan struct{}, provide chan<- string, consume <-chan message, meter ComponentMetric) (<-chan message, <-chan error) {
	out := make(chan message)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errc)
		call(r.reset, pipeID, errc) // reset hook
		var err error
		var m message
		for {
			// request new message
			select {
			case provide <- pipeID:
			case <-cancel:
				call(r.interrupt, pipeID, errc) // interrupt hook
				return
			}

			// receive new message
			select {
			case m = <-consume:
			case <-cancel:
				call(r.interrupt, pipeID, errc) // interrupt hook
				return
			}

			m.applyTo(componentID)           // apply params
			m.buffer, err = r.fn(bufferSize) // pump new buffer
			// process buffer
			if m.buffer != nil {
				if meter != nil {
					meter = meter.Message(m.buffer.Size())
				}
				m.feedback.applyTo(componentID) // apply feedback

				// push message further
				select {
				case out <- m:
				case <-cancel:
					call(r.interrupt, pipeID, errc) // interrupt hook
					return
				}
			}
			// handle error
			if err != nil {
				switch err {
				case io.EOF, io.ErrUnexpectedEOF:
					call(r.flush, pipeID, errc) // flush hook
				default:
					errc <- err
				}
				return
			}
		}
	}()
	return out, errc
}

// bindProcessor creates the closure. it's separated from run to have pre-run
// logic executed in correct order for all components.
func bindProcessor(pipeID string, sampleRate, numChannels int, p Processor) (*processRunner, error) {
	fn, err := p.Process(pipeID, sampleRate, numChannels)
	if err != nil {
		return nil, err
	}
	r := processRunner{
		fn:    fn,
		hooks: bindHooks(p),
	}
	return &r, nil
}

// run the Processor runner.
func (r *processRunner) run(pipeID, componentID string, cancel <-chan struct{}, in <-chan message, meter ComponentMetric) (<-chan message, <-chan error) {
	errc := make(chan error, 1)
	out := make(chan message)
	go func() {
		defer close(out)
		defer close(errc)
		call(r.reset, pipeID, errc) // reset hook
		var err error
		var m message
		var ok bool
		for {
			// retrieve new message
			select {
			case m, ok = <-in:
				if !ok {
					call(r.flush, pipeID, errc) // flush hook
					return
				}
			case <-cancel:
				call(r.interrupt, pipeID, errc) // interrupt hook
				return
			}

			m.applyTo(componentID)         // apply params
			m.buffer, err = r.fn(m.buffer) // process new buffer
			if err != nil {
				errc <- err
				return
			}

			if meter != nil {
				meter = meter.Message(m.buffer.Size())
			}

			m.feedback.applyTo(componentID) // apply feedback

			// send message further
			select {
			case out <- m:
			case <-cancel:
				call(r.interrupt, pipeID, errc) // interrupt hook
				return
			}
		}
	}()
	return out, errc
}

// bindSink creates the closure. it's separated from run to have pre-run
// logic executed in correct order for all components.
func bindSink(pipeID string, sampleRate, numChannels int, s Sink) (*sinkRunner, error) {
	fn, err := s.Sink(pipeID, sampleRate, numChannels)
	if err != nil {
		return nil, err
	}
	r := sinkRunner{
		fn:    fn,
		hooks: bindHooks(s),
	}
	return &r, nil
}

// run the sink runner.
func (r *sinkRunner) run(pipeID, componentID string, cancel <-chan struct{}, in <-chan message, meter ComponentMetric) <-chan error {
	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		call(r.reset, pipeID, errc) // reset hook
		var m message
		var ok bool
		for {
			// receive new message
			select {
			case m, ok = <-in:
				if !ok {
					call(r.flush, pipeID, errc) // flush hook
					return
				}
			case <-cancel:
				call(r.interrupt, pipeID, errc) // interrupt hook
				return
			}

			m.params.applyTo(componentID) // apply params
			err := r.fn(m.buffer)         // sink a buffer
			if err != nil {
				errc <- err
				return
			}

			if meter != nil {
				meter = meter.Message(m.buffer.Size())
			}

			m.feedback.applyTo(componentID) // apply feedback
		}
	}()

	return errc
}

// call optional function with pipeID argument. if error happens, it will be send to errc.
func call(fn hook, pipeID string, errc chan error) {
	if fn == nil {
		return
	}
	if err := fn(pipeID); err != nil {
		errc <- err
	}
	return
}
