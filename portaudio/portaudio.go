package portaudio

import (
	"github.com/gordonklaus/portaudio"
)

type (
	// Sink represets portaudio sink which allows to play audio using default device.
	Sink struct {
		buf    []float32
		stream *portaudio.Stream
	}
)

// NewSink returns new initialized sink which allows to play pipe.
func NewSink() *Sink {
	return &Sink{}
}

// Sink writes the buffer of data to portaudio stream.
// It aslo initilizes a portaudio api with default stream.
func (s *Sink) Sink(sourceID string, sampleRate, numChannels, bufferSize int) (func([][]float64) error, error) {
	s.buf = make([]float32, bufferSize*numChannels)
	err := portaudio.Initialize()
	if err != nil {
		return nil, err
	}
	s.stream, err = portaudio.OpenDefaultStream(0, numChannels, float64(sampleRate), bufferSize, &s.buf)
	if err != nil {
		return nil, err
	}
	err = s.stream.Start()
	if err != nil {
		return nil, err
	}
	return func(b [][]float64) error {
		for i := range b[0] {
			for j := range b {
				s.buf[i*numChannels+j] = float32(b[j][i])
			}
		}
		return s.stream.Write()
	}, nil
}

// Flush terminates portaudio structures.
func (s *Sink) Flush(string) error {
	err := s.stream.Stop()
	if err != nil {
		return err
	}
	err = s.stream.Close()
	if err != nil {
		return err
	}
	return portaudio.Terminate()
}
