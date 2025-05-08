package recorder

import (
	"os"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/pion/rtp"

	"github.com/flynnletford/mediamtx/src/formatprocessor"
	"github.com/flynnletford/mediamtx/src/playback"
	"github.com/flynnletford/mediamtx/src/unit"
)

// MP4Writer writes H264 RTP packets to an MP4 file.
type MP4Writer struct {
	file  *os.File
	muxer *playback.MuxerMP4

	// H264 format processor
	format *rtspformat.H264
	proc   formatprocessor.Processor

	// Track timing
	firstTimestamp *uint32
}

// NewMP4Writer creates a new MP4Writer.
func NewMP4Writer(filepath string) (*MP4Writer, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	// Initialize H264 format
	h264Format := &rtspformat.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
		SPS:               formatprocessor.H264DefaultSPS,
		PPS:               formatprocessor.H264DefaultPPS,
	}

	// Create format processor
	proc, err := formatprocessor.New(1472, h264Format, false, nil)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &MP4Writer{
		file:   file,
		muxer:  &playback.MuxerMP4{W: file},
		format: h264Format,
		proc:   proc,
	}, nil
}

// WriteRTPPacket writes an H264 RTP packet to the MP4 file.
func (w *MP4Writer) WriteRTPPacket(pkt *rtp.Packet) error {
	// Process RTP packet through format processor
	unitVal, err := w.proc.ProcessRTPPacket(pkt, time.Now(), int64(pkt.Timestamp), true)
	if err != nil {
		return err
	}

	h264Unit := unitVal.(*unit.H264)
	if h264Unit.AU == nil {
		return nil
	}

	// Initialize first timestamp
	if w.firstTimestamp == nil {
		w.firstTimestamp = &pkt.Timestamp
	}

	// Initialize track if needed
	if w.muxer.CurTrack == nil {
		init := &fmp4.Init{
			Tracks: []*fmp4.InitTrack{
				{
					ID:        96,
					TimeScale: 90000,
					Codec: &fmp4.CodecH264{
						SPS: w.format.SPS,
						PPS: w.format.PPS,
					},
				},
			},
		}
		w.muxer.WriteInit(init)
		w.muxer.SetTrack(96)
	}

	// Check if this is a keyframe
	isNonSyncSample := !h264.IsRandomAccess(h264Unit.AU)

	// Calculate relative timestamp in milliseconds
	relativeTs := int64(pkt.Timestamp - *w.firstTimestamp)

	// Prepare NAL units for writing
	var nalus [][]byte
	if !isNonSyncSample {
		// For keyframes, ensure SPS/PPS are included
		nalus = append(nalus, w.format.SPS, w.format.PPS)
	}
	nalus = append(nalus, h264Unit.AU...)

	// Calculate total size including start codes
	totalSize := uint32(0)
	for _, nalu := range nalus {
		totalSize += uint32(len(nalu)) + 4 // Add 4 bytes for start code
	}

	// Write the sample
	return w.muxer.WriteSample(
		relativeTs,
		0, // No PTS offset needed
		isNonSyncSample,
		totalSize,
		func() ([]byte, error) {
			// Concatenate all NAL units with start codes
			sample := make([]byte, totalSize)
			offset := 0
			for _, nalu := range nalus {
				// Write start code (0x00 0x00 0x00 0x01)
				sample[offset] = 0
				sample[offset+1] = 0
				sample[offset+2] = 0
				sample[offset+3] = 1
				offset += 4

				// Write NAL unit
				copy(sample[offset:], nalu)
				offset += len(nalu)
			}
			return sample, nil
		},
	)
}

// Close closes the writer and finalizes the MP4 file.
func (w *MP4Writer) Close() error {
	if w.muxer.CurTrack != nil {
		w.muxer.WriteFinalDTS(w.muxer.CurTrack.LastDTS)
	}
	err := w.muxer.Flush()
	if err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}
