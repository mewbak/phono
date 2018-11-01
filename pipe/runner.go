package pipe

import (
	"github.com/dudk/phono"
)

// pumpRunner is pump's runner.
type pumpRunner struct {
	phono.Pump
	measurable
	Flusher
	fn  phono.PumpFunc
	out chan message
}

// processRunner represents processor's runner.
type processRunner struct {
	phono.Processor
	measurable
	Flusher
	fn  phono.ProcessFunc
	in  <-chan message
	out chan message
}

// sinkRunner represents sink's runner.
type sinkRunner struct {
	phono.Sink
	measurable
	Flusher
	fn phono.SinkFunc
	in <-chan message
}

// Flusher owns resource that has to be flushed in the end of execution.
type Flusher interface {
	Flush(string) error
}

// FlushFunc represents clean up function which is executed after loop is finished.
type FlushFunc func(string) error

// counters is a structure for metrics initialization.
var counters = struct {
	pump      []string
	processor []string
	sink      []string
}{
	pump:      []string{OutputCounter},
	processor: []string{OutputCounter},
	sink:      []string{OutputCounter},
}

const (
	// OutputCounter is a key for output counter within metric.
	// It calculates regular total output per component.
	OutputCounter = "Output"
)

// flusher checks if interface implements Flusher and if so, return it.
func flusher(i interface{}) Flusher {
	if v, ok := i.(Flusher); ok {
		return v
	}
	return nil
}

// build creates the closure. it's separated from run to have pre-run
// logic executed in correct order for all components.
func (r *pumpRunner) build(sourceID string) (err error) {
	r.fn, err = r.Pump.Pump(sourceID)
	if err != nil {
		return err
	}
	return nil
}

// run the Pump runner.
func (r *pumpRunner) run(cancel chan struct{}, sourceID string, newMessage newMessageFunc) (<-chan message, <-chan error) {
	out := make(chan message)
	errc := make(chan error, 1)
	r.measurable.Reset()
	go func() {
		defer close(out)
		defer close(errc)
		defer func() {
			if r.Flusher != nil {
				err := r.Flusher.Flush(sourceID)
				if err != nil {
					errc <- err
				}
			}
		}()
		defer r.measurable.FinishMeasure()
		r.measurable.Latency()
		var err error
		var m message
		for {
			select {
			case m = <-newMessage():
			case <-cancel:
				return
			}
			m.applyTo(r.ID())
			m.Buffer, err = r.fn()
			if err != nil {
				if err != phono.ErrEOP {
					errc <- err
				}
				return
			}
			r.Counter(OutputCounter).Advance(m.Buffer)
			r.Latency()
			m.feedback.applyTo(r.ID())
			select {
			case <-cancel:
				return
			default:
				out <- m
			}
		}
	}()
	return out, errc
}

// build creates the closure. it's separated from run to have pre-run
// logic executed in correct order for all components.
func (r *processRunner) build(sourceID string) (err error) {
	r.fn, err = r.Processor.Process(sourceID)
	if err != nil {
		return err
	}
	return nil
}

// run the Processor runner.
func (r *processRunner) run(sourceID string, in <-chan message) (<-chan message, <-chan error) {
	errc := make(chan error, 1)
	r.in = in
	r.out = make(chan message)
	r.measurable.Reset()
	go func() {
		defer close(r.out)
		defer close(errc)
		defer func() {
			if r.Flusher != nil {
				err := r.Flusher.Flush(sourceID)
				if err != nil {
					errc <- err
				}
			}
		}()
		defer r.measurable.FinishMeasure()
		r.measurable.Latency()
		var err error
		for in != nil {
			select {
			case m, ok := <-in:
				if !ok {
					return
				}
				m.applyTo(r.Processor.ID())
				m.Buffer, err = r.fn(m.Buffer)
				if err != nil {
					errc <- err
					return
				}
				r.Counter(OutputCounter).Advance(m.Buffer)
				r.Latency()
				m.feedback.applyTo(r.ID())
				r.out <- m
			}
		}
	}()
	return r.out, errc
}

// build creates the closure. it's separated from run to have pre-run
// logic executed in correct order for all components.
func (r *sinkRunner) build(sourceID string) (err error) {
	r.fn, err = r.Sink.Sink(sourceID)
	if err != nil {
		return err
	}
	return nil
}

// run the sink runner.
func (r *sinkRunner) run(sourceID string, in <-chan message) <-chan error {
	errc := make(chan error, 1)
	r.measurable.Reset()
	go func() {
		defer close(errc)
		defer func() {
			if r.Flusher != nil {
				err := r.Flusher.Flush(sourceID)
				if err != nil {
					errc <- err
				}
			}
		}()
		defer r.measurable.FinishMeasure()
		r.measurable.Latency()
		for in != nil {
			select {
			case m, ok := <-in:
				if !ok {
					return
				}
				m.params.applyTo(r.Sink.ID())
				err := r.fn(m.Buffer)
				if err != nil {
					errc <- err
					return
				}
				r.Counter(OutputCounter).Advance(m.Buffer)
				r.Latency()
				m.feedback.applyTo(r.ID())
			}
		}
	}()

	return errc
}
