package recorder

import (
	"fmt"
	"os"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/flynnletford/mediamtx/src/logger"
	"github.com/flynnletford/mediamtx/src/stream"
)

// RTPRecorder writes RTP packets to an MP4 file using a Stream.
type RTPRecorder struct {
	file *os.File
	log  logger.Writer
	str  *stream.Stream

	// Media description and format
	media *description.Media
	forma rtspformat.Format

	// MP4 format
	format *formatFMP4
}

// NewRTPRecorder creates a new RTPRecorder.
func NewRTPRecorder(filepath string) (*RTPRecorder, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	// Create H264 format with default configuration
	forma := &rtspformat.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	// Create media description
	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []rtspformat.Format{forma},
	}

	// Create stream with proper configuration
	str := &stream.Stream{
		Desc: &description.Session{
			Medias: []*description.Media{media},
		},
		GenerateRTPPackets: true,
		UDPMaxPayloadSize:  1400, // Standard MTU size minus headers
	}
	err = str.Initialize()
	if err != nil {
		file.Close()
		return nil, err
	}

	// Create MP4 format
	format := &formatFMP4{
		ri: &recorderInstance{
			pathFormat: filepath,
			rec: &Recorder{
				Stream: str,
			},
		},
	}

	// Initialize format with tracks
	if !format.initialize() {
		file.Close()
		return nil, fmt.Errorf("failed to initialize MP4 format")
	}

	return &RTPRecorder{
		file:   file,
		log:    &SimpleLogger{},
		str:    str,
		media:  media,
		forma:  forma,
		format: format,
	}, nil
}

// WriteRTPPacket writes an RTP packet to the MP4 file.
func (r *RTPRecorder) WriteRTPPacket(pkt *rtp.Packet) error {
	// Write the RTP packet to the stream
	r.str.WriteRTPPacket(r.media, r.forma, pkt, time.Now(), int64(pkt.Timestamp))
	return nil
}

// Close closes the recorder.
func (r *RTPRecorder) Close() error {
	if r.str != nil {
		r.str.Close()
	}
	if r.format != nil {
		if r.format.currentSegment != nil {
			r.format.currentSegment.close()
		}
	}
	return r.file.Close()
}
