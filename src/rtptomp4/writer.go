package rtptomp4

import (
	"fmt"
	"os"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/pion/rtp"

	"github.com/flynnletford/mediamtx/src/formatprocessor"
	"github.com/flynnletford/mediamtx/src/logger"
	"github.com/flynnletford/mediamtx/src/stream"
	"github.com/flynnletford/mediamtx/src/unit"
)

type track struct {
	initTrack *fmp4.InitTrack
	nextID    int
}

// MP4Writer writes RTP packets to an MP4 file.
type MP4Writer struct {
	outputPath string
	stream     *stream.Stream
	format     format.Format
	processor  formatprocessor.Processor
	file       *os.File
	track      *track
	log        logger.Writer
	mdat       []byte
	media      *description.Media
	encoder    *rtph264.Encoder
}

// Log implements logger.Writer.
func (w *MP4Writer) Log(level logger.Level, format string, args ...interface{}) {
	w.log.Log(level, format, args...)
}

// NewMP4Writer creates a new MP4Writer.
func NewMP4Writer(outputPath string, format format.Format) (*MP4Writer, error) {
	// Create the output file
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	// Initialize the format processor
	log, err := logger.New(logger.Info, nil, "", "")
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	processor, err := formatprocessor.New(1500, format, false, log)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to create format processor: %w", err)
	}

	// Create track
	track := &track{
		initTrack: &fmp4.InitTrack{
			TimeScale: uint32(format.ClockRate()),
			ID:        1,
		},
	}

	// Initialize track codec based on format type
	switch format := format.(type) {
	case *rtspformat.H264:
		track.initTrack.Codec = &fmp4.CodecH264{
			SPS: format.SPS,
			PPS: format.PPS,
		}
	// Add other format types as needed
	default:
		file.Close()
		return nil, fmt.Errorf("unsupported format type: %T", format)
	}

	// Create media description
	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []rtspformat.Format{format},
	}
	desc := &description.Session{
		Medias: []*description.Media{media},
	}

	// Create and initialize stream
	stream := &stream.Stream{
		WriteQueueSize: 1500,
		Desc:           desc,
		Parent:         log,
	}
	if err := stream.Initialize(); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to initialize stream: %w", err)
	}

	writer := &MP4Writer{
		outputPath: outputPath,
		stream:     stream,
		format:     format,
		processor:  processor,
		file:       file,
		track:      track,
		log:        log,
		mdat:       make([]byte, 0),
		media:      media,
	}

	// Initialize H264 encoder if needed
	if h264Format, ok := format.(*rtspformat.H264); ok {
		writer.encoder = &rtph264.Encoder{
			PayloadMaxSize:    1500 - 12, // Standard MTU - RTP header
			PayloadType:       h264Format.PayloadTyp,
			PacketizationMode: h264Format.PacketizationMode,
		}
		if err := writer.encoder.Init(); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to initialize H264 encoder: %w", err)
		}
	}

	// Add a reader to the stream that will write to our file
	stream.AddReader(writer, media, format, func(u unit.Unit) error {
		// Convert the unit into an fMP4 sample based on format type
		var sampl fmp4.PartSample

		switch u := u.(type) {
		case *unit.H264:
			err := sampl.FillH264(0, u.AU) // Use 0 as duration, it will be updated later
			if err != nil {
				return fmt.Errorf("failed to fill H264 sample: %w", err)
			}
		// Add other unit types as needed
		default:
			return fmt.Errorf("unsupported unit type: %T", u)
		}

		// Append the sample to the mdat box
		writer.mdat = append(writer.mdat, sampl.Payload...)

		return nil
	})

	return writer, nil
}

// WriteRTP writes an RTP packet to the MP4 file.
func (w *MP4Writer) WriteRTP(pkt *rtp.Packet) error {
	// Convert RTP timestamp to NTP time
	// RTP timestamps are in the same units as the clock rate
	// We need to convert this to a duration and add it to a base NTP time
	ntp := time.Now().Add(time.Duration(pkt.Timestamp) * time.Second / time.Duration(w.format.ClockRate()))

	// Calculate PTS from RTP timestamp
	// PTS should be in the same units as the clock rate
	pts := int64(pkt.Timestamp)

	// Use the stream's WriteRTPPacket functionality with the correct timestamp and media
	w.stream.WriteRTPPacket(w.media, w.format, pkt, ntp, pts)
	return nil
}

// Close closes the MP4Writer and finalizes the MP4 file.
func (w *MP4Writer) Close() error {
	// Close the stream
	w.stream.Close()

	// Write the init segment
	init := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{w.track.initTrack},
	}

	var buf seekablebuffer.Buffer
	err := init.Marshal(&buf)
	if err != nil {
		return fmt.Errorf("failed to write init segment: %w", err)
	}

	_, err = w.file.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write init segment: %w", err)
	}

	// Write the mdat box
	_, err = w.file.Write(w.mdat)
	if err != nil {
		return fmt.Errorf("failed to write mdat box: %w", err)
	}

	// Close the file
	return w.file.Close()
}
