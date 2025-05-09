package recorder

import (
	"os"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
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
	forma format.Format
}

// NewRTPRecorder creates a new RTPRecorder.
func NewRTPRecorder(filepath string) (*RTPRecorder, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	// Create H264 format
	forma := &format.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	// Create media description
	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{forma},
	}

	// Create stream
	str := &stream.Stream{
		Desc: &description.Session{
			Medias: []*description.Media{media},
		},
		GenerateRTPPackets: true,
	}
	err = str.Initialize()
	if err != nil {
		file.Close()
		return nil, err
	}

	return &RTPRecorder{
		file:  file,
		log:   &SimpleLogger{},
		str:   str,
		media: media,
		forma: forma,
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
	return r.file.Close()
}
