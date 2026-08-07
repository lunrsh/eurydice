package sync

// This file is HEAVILY based on the go-astiav transcoding example, and thus might not reflect Eurydice's typical code style.
// Disregarding that, thank you go-astiav team!

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"git.lunr.sh/luna/eurydice/state/syncstate"

	"errors"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

type stream struct {
	buffersinkContext *astiav.BuffersinkFilterContext
	buffersrcContext  *astiav.BuffersrcFilterContext
	decCodec          *astiav.Codec
	decCodecContext   *astiav.CodecContext
	decFrame          *astiav.Frame
	decLastPTS        *int64
	encCodec          *astiav.Codec
	encCodecContext   *astiav.CodecContext
	encPkt            *astiav.Packet
	filterFrame       *astiav.Frame
	filterGraph       *astiav.FilterGraph
	inputStream       *astiav.Stream
	outputStream      *astiav.Stream
	audioFifo         *astiav.AudioFifo
}

func openInputFile(c *astikit.Closer, streams map[int]*stream, inputFile string) (inputFormatContext *astiav.FormatContext, err error) {
	// Allocate input format context
	if inputFormatContext = astiav.AllocFormatContext(); inputFormatContext == nil {
		err = errors.New("input format context is nil")
		return
	}

	c.Add(inputFormatContext.Free)

	// Open input
	if err = inputFormatContext.OpenInput(inputFile, nil, nil); err != nil {
		err = fmt.Errorf("opening input failed: %w", err)
		return
	}

	c.Add(inputFormatContext.CloseInput)

	// Find stream info
	if err = inputFormatContext.FindStreamInfo(nil); err != nil {
		err = fmt.Errorf("finding stream info failed: %w", err)
		return
	}

	// Loop through streams
	for _, is := range inputFormatContext.Streams() {
		// Only process audio or video
		if is.CodecParameters().MediaType() != astiav.MediaTypeAudio {
			continue
		}

		// Create stream
		s := &stream{inputStream: is}

		// Find decoder
		if s.decCodec = astiav.FindDecoder(is.CodecParameters().CodecID()); s.decCodec == nil {
			err = errors.New("codec is nil")
			return
		}

		// Allocate codec context
		if s.decCodecContext = astiav.AllocCodecContext(s.decCodec); s.decCodecContext == nil {
			err = errors.New("codec context is nil")
			return
		}
		c.Add(s.decCodecContext.Free)

		// Update codec context
		if err = is.CodecParameters().ToCodecContext(s.decCodecContext); err != nil {
			err = fmt.Errorf("updating codec context failed: %w", err)
			return
		}

		// Open codec context
		if err = s.decCodecContext.Open(s.decCodec, nil); err != nil {
			err = fmt.Errorf("opening codec context failed: %w", err)
			return
		}

		// Set time base
		s.decCodecContext.SetTimeBase(is.TimeBase())

		// Allocate frame
		s.decFrame = astiav.AllocFrame()
		c.Add(s.decFrame.Free)

		// Store stream
		streams[is.Index()] = s
	}

	return
}

func openOutputFile(audioFormat int32, inputFormatContext *astiav.FormatContext, c *astikit.Closer, streams map[int]*stream, outputFile string) (outputFormatContext *astiav.FormatContext, err error) {
	// Allocate output format context
	if outputFormatContext, err = astiav.AllocOutputFormatContext(nil, "", outputFile); err != nil {
		err = fmt.Errorf("allocating output format context failed: %w", err)
		return
	} else if outputFormatContext == nil {
		err = errors.New("output format context is nil")
		return
	}

	c.Add(outputFormatContext.Free)

	// Loop through streams
	for _, is := range inputFormatContext.Streams() {
		// Get stream
		s, ok := streams[is.Index()]
		if !ok {
			continue
		}

		// Create output stream
		if s.outputStream = outputFormatContext.NewStream(nil); s.outputStream == nil {
			err = errors.New("output stream is nil")
			return
		}

		// Get codec id
		var codecID astiav.CodecID

		switch int(audioFormat) {
		case syncstate.AudioLowQuality:
			codecID = astiav.CodecIDMp3
		case syncstate.AudioMediumQuality:
			codecID = astiav.CodecIDMp3
		case syncstate.AudioHighQuality:
			codecID = astiav.CodecIDMp3
		case syncstate.AudioLosslessQuality:
			codecID = astiav.CodecIDFlac
		}

		// Find encoder
		if s.encCodec = astiav.FindEncoder(codecID); s.encCodec == nil {
			err = errors.New("codec is nil")
			return
		}

		// Allocate codec context
		if s.encCodecContext = astiav.AllocCodecContext(s.encCodec); s.encCodecContext == nil {
			err = errors.New("codec context is nil")
			return
		}

		c.Add(s.encCodecContext.Free)

		// Update codec context
		switch int(audioFormat) {
		case syncstate.AudioLowQuality:
			s.encCodecContext.SetBitRate(128 * 1000)
		case syncstate.AudioMediumQuality:
			s.encCodecContext.SetBitRate(192 * 1000)
		case syncstate.AudioHighQuality:
			s.encCodecContext.SetBitRate(320 * 1000)
		}

		// Set up channel layout
		var channelLayout astiav.ChannelLayout
		isLayoutSupported := false

		for _, supportedLayout := range s.encCodec.SupportedChannelLayouts() {
			if supportedLayout.Equal(s.decCodecContext.ChannelLayout()) {
				isLayoutSupported = true
				channelLayout = supportedLayout

				break
			}
		}

		if isLayoutSupported {
			s.encCodecContext.SetChannelLayout(channelLayout)
		} else {
			s.encCodecContext.SetChannelLayout(s.encCodec.SupportedChannelLayouts()[0])
		}

		// Update sample rate
		supportedSampleRates := s.encCodec.SupportedSampleRates()

		if len(supportedSampleRates) == 0 {
			s.encCodecContext.SetSampleRate(s.decCodecContext.SampleRate())
		} else {
			targetSampleRate := s.decCodecContext.SampleRate()
			bestMatchingSampleRate := supportedSampleRates[0]
			bestDiffSampleRate := math.Abs(float64(bestMatchingSampleRate - targetSampleRate))

			for _, currentlySelectedSampleRate := range supportedSampleRates {
				currentDiff := math.Abs(float64(currentlySelectedSampleRate - targetSampleRate))

				if currentDiff < bestDiffSampleRate {
					bestMatchingSampleRate = currentlySelectedSampleRate
					bestDiffSampleRate = currentDiff
				}
			}

			s.encCodecContext.SetSampleRate(bestMatchingSampleRate)
		}

		if v := s.encCodec.SupportedSampleFormats(); len(v) > 0 {
			s.encCodecContext.SetSampleFormat(v[0])
		} else {
			s.encCodecContext.SetSampleFormat(s.decCodecContext.SampleFormat())
		}

		s.encCodecContext.SetTimeBase(astiav.NewRational(1, s.encCodecContext.SampleRate()))

		// Update flags
		if outputFormatContext.OutputFormat().Flags().Has(astiav.IOFormatFlagGlobalheader) {
			s.encCodecContext.SetFlags(s.encCodecContext.Flags().Add(astiav.CodecContextFlagGlobalHeader))
		}

		// Open codec context
		if err = s.encCodecContext.Open(s.encCodec, nil); err != nil {
			err = fmt.Errorf("opening codec context failed: %w", err)
			return
		}

		// Update codec parameters
		if err = s.outputStream.CodecParameters().FromCodecContext(s.encCodecContext); err != nil {
			err = fmt.Errorf("updating codec parameters failed: %w", err)
			return
		}

		// Update stream
		s.outputStream.SetTimeBase(s.encCodecContext.TimeBase())
	}

	// If this is a file, we need to use an io context
	if !outputFormatContext.OutputFormat().Flags().Has(astiav.IOFormatFlagNofile) {
		// Open io context
		var ioContext *astiav.IOContext

		if ioContext, err = astiav.OpenIOContext(outputFile, astiav.NewIOContextFlags(astiav.IOContextFlagWrite), nil, nil); err != nil {
			err = fmt.Errorf("opening io context failed: %w", err)
			return
		}

		c.AddWithError(ioContext.Close)

		// Update output format context
		outputFormatContext.SetPb(ioContext)
	}

	// Write header
	if err = outputFormatContext.WriteHeader(nil); err != nil {
		err = fmt.Errorf("writing header failed: %w", err)
		return
	}

	return
}

func initFilters(c *astikit.Closer, streams map[int]*stream) (err error) {
	// Loop through audio streams
	for _, s := range streams {
		// Allocate graph
		if s.filterGraph = astiav.AllocFilterGraph(); s.filterGraph == nil {
			err = errors.New("graph is nil")
			return
		}

		c.Add(s.filterGraph.Free)

		// Allocate outputs
		outputs := astiav.AllocFilterInOut()

		if outputs == nil {
			err = errors.New("outputs is nil")
			return
		}

		c.Add(outputs.Free)

		// Allocate inputs
		inputs := astiav.AllocFilterInOut()

		if inputs == nil {
			err = errors.New("inputs is nil")
			return
		}

		c.Add(inputs.Free)

		// Create buffersrc context parameters
		buffersrcContextParameters := astiav.AllocBuffersrcFilterContextParameters()
		defer buffersrcContextParameters.Free()

		buffersrc := astiav.FindFilterByName("abuffer")
		buffersrcContextParameters.SetChannelLayout(s.decCodecContext.ChannelLayout())
		buffersrcContextParameters.SetSampleFormat(s.decCodecContext.SampleFormat())
		buffersrcContextParameters.SetSampleRate(s.decCodecContext.SampleRate())
		buffersrcContextParameters.SetTimeBase(s.decCodecContext.TimeBase())
		buffersink := astiav.FindFilterByName("abuffersink")

		content := fmt.Sprintf("aformat=sample_fmts=%s:channel_layouts=%s:sample_rates=%d", s.encCodecContext.SampleFormat().Name(), s.encCodecContext.ChannelLayout().String(), s.encCodecContext.SampleRate())

		// Check filters
		if buffersrc == nil {
			err = errors.New("buffersrc is nil")
			return
		}

		if buffersink == nil {
			err = errors.New("buffersink is nil")
			return
		}

		// Create filter contexts
		if s.buffersrcContext, err = s.filterGraph.NewBuffersrcFilterContext(buffersrc, "in"); err != nil {
			err = fmt.Errorf("creating buffersrc context failed: %w", err)
			return
		}

		if s.buffersinkContext, err = s.filterGraph.NewBuffersinkFilterContext(buffersink, "out"); err != nil {
			err = fmt.Errorf("creating buffersink context failed: %w", err)
			return
		}

		// Set buffersrc context parameters
		if err = s.buffersrcContext.SetParameters(buffersrcContextParameters); err != nil {
			err = fmt.Errorf("setting buffersrc context parameters failed: %w", err)
			return
		}

		// Initialize buffersrc context
		if err = s.buffersrcContext.Initialize(nil); err != nil {
			err = fmt.Errorf("initializing buffersrc context failed: %w", err)
			return
		}

		// Update outputs
		outputs.SetName("in")
		outputs.SetFilterContext(s.buffersrcContext.FilterContext())
		outputs.SetPadIdx(0)
		outputs.SetNext(nil)

		// Update inputs
		inputs.SetName("out")
		inputs.SetFilterContext(s.buffersinkContext.FilterContext())
		inputs.SetPadIdx(0)
		inputs.SetNext(nil)

		// Parse
		if err = s.filterGraph.Parse(content, inputs, outputs); err != nil {
			err = fmt.Errorf("parsing filter failed: %w", err)
			return
		}

		// Configure
		if err = s.filterGraph.Configure(); err != nil {
			err = fmt.Errorf("configuring filter failed: %w", err)
			return
		}

		// Allocate frame
		s.filterFrame = astiav.AllocFrame()
		c.Add(s.filterFrame.Free)

		// Allocate packet
		s.encPkt = astiav.AllocPacket()
		c.Add(s.encPkt.Free)
	}

	return
}

func filterEncodeWriteFrame(f *astiav.Frame, s *stream, outputFormatContext *astiav.FormatContext) error {
	// Initialize FIFO if needed
	if s.audioFifo == nil {
		s.audioFifo = astiav.AllocAudioFifo(
			s.encCodecContext.SampleFormat(),
			s.encCodecContext.ChannelLayout().Channels(),
			s.encCodecContext.FrameSize()*2,
		)
	}

	// Send frame
	if err := s.buffersrcContext.AddFrame(
		f,
		astiav.NewBuffersrcFlags(astiav.BuffersrcFlagKeepRef),
	); err != nil {
		return fmt.Errorf("adding frame to filter failed: %w", err)
	}

	// Loop
	for {
		err := s.buffersinkContext.GetFrame(
			s.filterFrame,
			astiav.NewBuffersinkFlags(),
		)

		if err != nil {
			if errors.Is(err, astiav.ErrEagain) || errors.Is(err, astiav.ErrEof) {
				break
			}

			return fmt.Errorf("getting filtered frame failed: %w", err)
		}

		_, err = s.audioFifo.Write(s.filterFrame)
		s.filterFrame.Unref()

		if err != nil {
			return fmt.Errorf("writing audio fifo failed: %w", err)
		}

		if err := drainFifo(s, outputFormatContext, false); err != nil {
			return err
		}
	}

	return nil
}

func drainFifo(s *stream, outputFormatContext *astiav.FormatContext, final bool) error {
	frameSize := s.encCodecContext.FrameSize()

	for s.audioFifo.Size() >= frameSize || (final && s.audioFifo.Size() > 0) {
		samples := frameSize

		if final && s.audioFifo.Size() < frameSize {
			samples = s.audioFifo.Size()
		}

		out := astiav.AllocFrame()

		out.SetNbSamples(samples)
		out.SetChannelLayout(s.encCodecContext.ChannelLayout())
		out.SetSampleFormat(s.encCodecContext.SampleFormat())
		out.SetSampleRate(s.encCodecContext.SampleRate())

		if err := out.AllocBuffer(0); err != nil {
			out.Free()
			return fmt.Errorf("allocating fifo frame failed: %w", err)
		}

		_, err := s.audioFifo.Read(out)

		if err != nil {
			out.Free()
			return fmt.Errorf("reading fifo failed: %w", err)
		}

		err = encodeWriteFrame(out, s, outputFormatContext)
		out.Free()

		if err != nil {
			return err
		}
	}

	return nil
}

func encodeWriteFrame(f *astiav.Frame, s *stream, outputFormatContext *astiav.FormatContext) (err error) {
	// Send frame
	if err = s.encCodecContext.SendFrame(f); err != nil {
		err = fmt.Errorf("sending frame failed: %w", err)
		return
	}

	// Loop
	for {
		// We use a closure to ease unreferencing the packet
		if stop, err := func() (bool, error) {
			// Receive packet
			if err := s.encCodecContext.ReceivePacket(s.encPkt); err != nil {
				if errors.Is(err, astiav.ErrEof) || errors.Is(err, astiav.ErrEagain) {
					return true, nil
				}
				return false, fmt.Errorf("receiving packet failed: %w", err)
			}

			// Make sure to unreference packet
			defer s.encPkt.Unref()

			// Update pkt
			s.encPkt.SetStreamIndex(s.outputStream.Index())
			s.encPkt.RescaleTs(s.encCodecContext.TimeBase(), s.outputStream.TimeBase())

			// Write frame
			if err := outputFormatContext.WriteInterleavedFrame(s.encPkt); err != nil {
				return false, fmt.Errorf("writing frame failed: %w", err)
			}
			return false, nil
		}(); err != nil {
			return err
		} else if stop {
			break
		}
	}
	return
}

// Transcodes a song from the source path to the target path
func transcodeSong(state *stateStructs.ApplicationState, sourcePath, targetPath string) error {
	// Fast path: if the source and target are both FLACs, and we're encoding as a flac, we just copy the song
	if filepath.Ext(sourcePath) == ".flac" && filepath.Ext(targetPath) == ".flac" && state.PageStates.Sync.AudioQuality == int32(syncstate.AudioLosslessQuality) {
		return copySong(sourcePath, targetPath)
	}

	var (
		c                   = astikit.NewCloser()
		inputFormatContext  *astiav.FormatContext
		outputFormatContext *astiav.FormatContext
		streams             = make(map[int]*stream) // Indexed by input stream index
	)

	// Handle ffmpeg logs
	astiav.SetLogLevel(astiav.LogLevelDebug)

	astiav.SetLogCallback(func(c astiav.Classer, l astiav.LogLevel, fmt, msg string) {
		state.Logger.Debugf("ffmpeg: %s", strings.TrimSpace(msg))
	})

	// We use an astikit.Closer to free all resources properly
	defer c.Close()

	// Open input file
	var err error

	if inputFormatContext, err = openInputFile(c, streams, sourcePath); err != nil {
		return fmt.Errorf("opening input file failed: %w", err)
	}

	// Open output file
	if outputFormatContext, err = openOutputFile(
		state.PageStates.Sync.AudioQuality,
		inputFormatContext,
		c,
		streams,
		targetPath); err != nil {
		return fmt.Errorf("opening output file failed: %w", err)
	}

	// Init filters
	if err := initFilters(c, streams); err != nil {
		return fmt.Errorf("initializing filters failed: %w", err)
	}

	// Allocate packet
	pkt := astiav.AllocPacket()
	c.Add(pkt.Free)

	// Loop through packets
	for {
		// We use a closure to ease unreferencing the packet
		if stop := func() bool {
			// Read frame
			if err := inputFormatContext.ReadFrame(pkt); err != nil {
				if !errors.Is(err, astiav.ErrEof) {
					state.Logger.Errorf("Reading frame failed: %v", err)
				}
				return true
			}

			// Make sure to unreference the packet
			defer pkt.Unref()

			// Get stream
			s, ok := streams[pkt.StreamIndex()]
			if !ok {
				return false
			}

			// Update packet
			pkt.RescaleTs(s.inputStream.TimeBase(), s.decCodecContext.TimeBase())

			// Send packet
			if err := s.decCodecContext.SendPacket(pkt); err != nil {
				state.Logger.Errorf("Sending packet failed: %v", err)
				return true
			}

			// Loop
			for {
				// We use a closure to ease unreferencing the frame
				if stop := func() bool {
					// Receive frame
					if err := s.decCodecContext.ReceiveFrame(s.decFrame); err != nil {
						if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
							state.Logger.Errorf("Recieving frame failed: %v", err)
						}

						return true
					}

					// Make sure to unreference the frame
					defer s.decFrame.Unref()

					// Ignore frames with non monotonic PTS
					if s.decLastPTS != nil && *s.decLastPTS >= s.decFrame.Pts() {
						return false
					}

					s.decLastPTS = astikit.Int64Ptr(s.decFrame.Pts())

					// Filter, encode and write frame
					if err := filterEncodeWriteFrame(s.decFrame, s, outputFormatContext); err != nil {
						state.Logger.Errorf("Filtering, encoding and writing frame failed: %v", err)
						return true
					}

					return false
				}(); stop {
					break
				}
			}
			return false
		}(); stop {
			break
		}
	}

	// Loop through streams
	for _, s := range streams {
		// Flush filter
		if err := filterEncodeWriteFrame(nil, s, outputFormatContext); err != nil {
			return fmt.Errorf("writing trailer failed: %v", err)
		}

		// Flush encoder
		if err := encodeWriteFrame(nil, s, outputFormatContext); err != nil {
			return fmt.Errorf("encoding and writing frame failed: %v", err)
		}

		// Free FIFO
		if s.audioFifo != nil {
			s.audioFifo.Free()
		}
	}

	// Write trailer
	if err := outputFormatContext.WriteTrailer(); err != nil {
		return fmt.Errorf("writing trailer failed: %v", err)
	}

	// Done
	return nil
}
