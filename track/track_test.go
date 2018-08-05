package track_test

import (
	"fmt"
	"testing"

	"github.com/dudk/phono/asset"
	"github.com/dudk/phono/mock"
	"github.com/dudk/phono/pipe"
	"github.com/dudk/phono/track"
	"github.com/dudk/phono/wav"

	"github.com/stretchr/testify/assert"

	"github.com/dudk/phono"
)

var (
	bufferSize  = phono.BufferSize(512)
	sampleRate  = phono.SampleRate(44100)
	numChannels = phono.NumChannels(1)
	inFile      = "../_testdata/in1.wav"
	outFile     = "../_testdata/track_test1.wav"

	asset1 = &asset.Asset{
		Buffer: [][]float64{[]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
	}
	asset2 = &asset.Asset{
		Buffer: [][]float64{[]float64{2, 2, 2, 2, 2, 2, 2, 2, 2, 2}},
	}
	overlapTests = []struct {
		phono.BufferSize
		clips   []*phono.Frame
		clipsAt []int64
		result  []float64
		msg     string
	}{
		{
			BufferSize: 2,
			clips: []*phono.Frame{
				asset1.Frame(3, 1),
				asset2.Frame(5, 3),
			},
			clipsAt: []int64{3, 4},
			result:  []float64{0, 0, 0, 1, 2, 2, 2, 0},
			msg:     "Sequence",
		},
		{
			BufferSize: 3,
			clips: []*phono.Frame{
				asset1.Frame(3, 1),
				asset2.Frame(5, 3),
			},
			clipsAt: []int64{3, 4},
			result:  []float64{0, 0, 0, 1, 2, 2, 2, 0, 0},
			msg:     "Sequence increased bufferSize",
		},
		{
			BufferSize: 2,
			clips: []*phono.Frame{
				asset1.Frame(3, 1),
				asset2.Frame(5, 3),
			},
			clipsAt: []int64{2, 3},
			result:  []float64{0, 0, 1, 2, 2, 2},
			msg:     "Sequence shifted left",
		},
		{
			BufferSize: 2,
			clips: []*phono.Frame{
				asset1.Frame(3, 1),
				asset2.Frame(5, 3),
			},
			clipsAt: []int64{2, 4},
			result:  []float64{0, 0, 1, 0, 2, 2, 2, 0},
			msg:     "Sequence with interval",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 3),
				asset2.Frame(5, 2),
			},
			clipsAt: []int64{3, 2},
			result:  []float64{0, 0, 2, 2, 1, 1},
			msg:     "Overlap previous",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 3),
				asset2.Frame(5, 2),
			},
			clipsAt: []int64{2, 4},
			result:  []float64{0, 0, 1, 1, 2, 2},
			msg:     "Overlap next",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 5),
				asset2.Frame(5, 2),
			},
			clipsAt: []int64{2, 4},
			result:  []float64{0, 0, 1, 1, 2, 2, 1, 0},
			msg:     "Overlap single in the middle",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 2),
				asset1.Frame(3, 2),
				asset2.Frame(5, 2),
			},
			clipsAt: []int64{2, 5, 4},
			result:  []float64{0, 0, 1, 1, 2, 2, 1, 0},
			msg:     "Overlap two in the middle",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 2),
				asset1.Frame(5, 2),
				asset2.Frame(3, 2),
			},
			clipsAt: []int64{2, 5, 3},
			result:  []float64{0, 0, 1, 2, 2, 1, 1, 0},
			msg:     "Overlap two in the middle shifted",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 2),
				asset2.Frame(3, 5),
			},
			clipsAt: []int64{2, 2},
			result:  []float64{0, 0, 2, 2, 2, 2, 2, 0},
			msg:     "Overlap single completely",
		},
		{
			clips: []*phono.Frame{
				asset1.Frame(3, 2),
				asset1.Frame(5, 2),
				asset2.Frame(1, 8),
			},
			clipsAt: []int64{2, 5, 1},
			result:  []float64{0, 2, 2, 2, 2, 2, 2, 2, 2, 0},
			msg:     "Overlap two completely",
		},
	}
)

func TestTrackWavSlices(t *testing.T) {
	wavPump, err := wav.NewPump(inFile, bufferSize)
	assert.Nil(t, err)
	asset := &asset.Asset{
		SampleRate: sampleRate,
	}

	p1 := pipe.New(
		pipe.WithPump(wavPump),
		pipe.WithSinks(asset),
	)
	_ = p1.Do(pipe.Run)

	wavSink, err := wav.NewSink(
		outFile,
		wavPump.WavSampleRate(),
		wavPump.WavNumChannels(),
		wavPump.WavBitDepth(),
		wavPump.WavAudioFormat(),
	)
	track := track.New(bufferSize, asset.NumChannels())

	track.AddFrame(198450, asset.Frame(0, 44100))
	track.AddFrame(66150, asset.Frame(44100, 44100))
	track.AddFrame(132300, asset.Frame(0, 44100))

	p2 := pipe.New(
		pipe.WithPump(track),
		pipe.WithSinks(wavSink),
	)
	_ = p2.Do(pipe.Run)
}

func TestSliceOverlaps(t *testing.T) {
	sink := &mock.Sink{}
	bufferSize := phono.BufferSize(2)
	track := track.New(bufferSize, asset1.NumChannels())
	for _, test := range overlapTests {
		fmt.Printf("Starting: %v\n", test.msg)
		track.Reset()

		for i, clip := range test.clips {
			track.AddFrame(test.clipsAt[i], clip)
		}

		p := pipe.New(
			pipe.WithPump(track),
			pipe.WithSinks(sink),
		)
		if test.BufferSize > 0 {
			p.Push(phono.NewParams(
				track.BufferSizeParam(test.BufferSize),
			))
		}

		_ = p.Do(pipe.Run)
		assert.Equal(t, len(test.result), len(sink.Buffer[0]), test.msg)
		for i, v := range sink.Buffer[0] {
			assert.Equal(t, test.result[i], v, "Test: %v Index: %v Full expected: %v Full result:%v", test.msg, i, test.result, sink.Buffer[0])
		}
	}

}