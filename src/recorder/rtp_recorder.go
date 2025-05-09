package recorder

import (
	"os"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/pion/rtp"

	"github.com/flynnletford/mediamtx/src/logger"
	"github.com/flynnletford/mediamtx/src/playback"
)

// RTPRecorder writes RTP packets to an MP4 file.
type RTPRecorder struct {
	file *os.File
	log  logger.Writer

	// H264 specific
	sps          []byte
	pps          []byte
	dtsExtractor *h264.DTSExtractor

	// MP4 muxer
	muxer *playback.MuxerMP4
	track *playback.MuxerMP4Track
}

// NewRTPRecorder creates a new RTPRecorder.
func NewRTPRecorder(filepath string, log logger.Writer) (*RTPRecorder, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	return &RTPRecorder{
		file: file,
		log:  log,
	}, nil
}

// WriteRTPPacket writes an RTP packet to the MP4 file.
func (r *RTPRecorder) WriteRTPPacket(pkt *rtp.Packet) error {
	// Extract SPS and PPS from the packet if present
	if len(pkt.Payload) > 0 {
		typ := h264.NALUType(pkt.Payload[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS:
			r.sps = pkt.Payload
			return nil
		case h264.NALUTypePPS:
			r.pps = pkt.Payload
			return nil
		}
	}

	// Initialize muxer if not done yet
	if r.muxer == nil {
		if r.sps == nil || r.pps == nil {
			return nil // Wait for SPS and PPS
		}

		// Create H264 codec
		codec := &fmp4.CodecH264{
			SPS: r.sps,
			PPS: r.pps,
		}

		// Create init segment
		init := &fmp4.Init{
			Tracks: []*fmp4.InitTrack{
				{
					ID:        1,
					TimeScale: 90000, // H264 uses 90kHz clock
					Codec:     codec,
				},
			},
		}

		// Create muxer
		r.muxer = &playback.MuxerMP4{
			W: r.file,
		}
		r.muxer.WriteInit(init)
		r.muxer.SetTrack(1)

		// Initialize DTS extractor
		r.dtsExtractor = &h264.DTSExtractor{}
		r.dtsExtractor.Initialize()
	}

	// Extract NALUs from RTP packet
	nalus := [][]byte{pkt.Payload} // For now, assume single NALU per packet

	// Extract DTS
	dts, err := r.dtsExtractor.Extract(nalus, int64(pkt.Timestamp))
	if err != nil {
		return err
	}

	// Write sample
	return r.muxer.WriteSample(
		int64(pkt.Timestamp),
		int32(int64(pkt.Timestamp)-dts),
		!h264.IsRandomAccess(nalus),
		uint32(len(pkt.Payload)),
		func() ([]byte, error) {
			return pkt.Payload, nil
		},
	)
}

// Close closes the recorder.
func (r *RTPRecorder) Close() error {
	if r.muxer != nil {
		r.muxer.WriteFinalDTS(r.muxer.CurTrack.LastDTS)
	}
	return r.file.Close()
}
