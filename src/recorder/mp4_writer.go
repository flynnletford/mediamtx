package recorder

import (
	"errors"
	"os"

	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/pion/rtp"

	"github.com/flynnletford/mediamtx/src/formatprocessor"
	"github.com/flynnletford/mediamtx/src/playback"
)

// MP4Writer writes RTP packets to an MP4 file.
type MP4Writer struct {
	file  *os.File
	muxer *playback.MuxerMP4

	// DTS extractors for codecs that support B-frames
	h264DTSExtractor *h264.DTSExtractor
	h265DTSExtractor *h265.DTSExtractor
}

// NewMP4Writer creates a new MP4Writer.
func NewMP4Writer(filepath string) (*MP4Writer, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	return &MP4Writer{
		file:  file,
		muxer: &playback.MuxerMP4{W: file},
	}, nil
}

// WriteRTPPacket writes an RTP packet to the MP4 file.
func (w *MP4Writer) WriteRTPPacket(pkt *rtp.Packet) error {
	// Extract codec information from the RTP packet
	payloadType := pkt.PayloadType
	clockRate := getClockRate(payloadType)

	// Create or get track
	trackID := int(payloadType)
	if w.muxer.CurTrack == nil || w.muxer.CurTrack.ID != trackID {
		// Initialize track with codec information
		init := &fmp4.Init{
			Tracks: []*fmp4.InitTrack{
				{
					ID:        trackID,
					TimeScale: uint32(clockRate),
					Codec:     getCodecForPayloadType(payloadType),
				},
			},
		}
		w.muxer.WriteInit(init)
		w.muxer.SetTrack(trackID)
	}

	// For codecs that support B-frames (H264, H265), we need to extract DTS
	var dts int64
	var isNonSyncSample bool
	var ptsOffset int32

	// Use RTP timestamp as PTS
	pts := int64(pkt.Timestamp)

	switch payloadType {
	case 96: // H264
		// For H264, we need to check if this is a keyframe
		// Create a decoder to extract NAL units
		decoder := &rtph264.Decoder{}
		err := decoder.Init()
		if err != nil {
			return err
		}

		// Decode the RTP packet into NAL units
		nalus, err := decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtph264.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		// Check if this is a keyframe
		isNonSyncSample = !h264.IsRandomAccess(nalus)

		// Initialize DTS extractor if not already done
		if w.h264DTSExtractor == nil {
			if !h264.IsRandomAccess(nalus) {
				return nil
			}
			w.h264DTSExtractor = &h264.DTSExtractor{}
			w.h264DTSExtractor.Initialize()
		}

		// Extract DTS
		dts, err = w.h264DTSExtractor.Extract(nalus, pts)
		if err != nil {
			return err
		}

		// Calculate PTS offset
		ptsOffset = int32(pts - dts)

	case 97: // H265
		// For H265, we need to check if this is a keyframe
		// Create a decoder to extract NAL units
		decoder := &rtph265.Decoder{}
		err := decoder.Init()
		if err != nil {
			return err
		}

		// Decode the RTP packet into NAL units
		nalus, err := decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtph265.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtph265.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		// Check if this is a keyframe
		isNonSyncSample = !h265.IsRandomAccess(nalus)

		// Initialize DTS extractor if not already done
		if w.h265DTSExtractor == nil {
			if !h265.IsRandomAccess(nalus) {
				return nil
			}
			w.h265DTSExtractor = &h265.DTSExtractor{}
			w.h265DTSExtractor.Initialize()
		}

		// Extract DTS
		dts, err = w.h265DTSExtractor.Extract(nalus, pts)
		if err != nil {
			return err
		}

		// Calculate PTS offset
		ptsOffset = int32(pts - dts)

	default:
		// For other codecs, use PTS as DTS
		dts = pts
		isNonSyncSample = false
		ptsOffset = 0
	}

	// Write the sample
	return w.muxer.WriteSample(
		dts,
		ptsOffset,
		isNonSyncSample,
		uint32(len(pkt.Payload)),
		func() ([]byte, error) {
			return pkt.Payload, nil
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

// getClockRate returns the clock rate for a given payload type.
func getClockRate(payloadType uint8) int {
	switch payloadType {
	case 96: // H264
		return 90000
	case 97: // H265
		return 90000
	case 98: // VP8
		return 90000
	case 99: // VP9
		return 90000
	case 100: // MPEG4 Video
		return 90000
	case 101: // MPEG1 Video
		return 90000
	case 102: // MJPEG
		return 90000
	case 103: // MPEG1 Audio
		return 90000
	case 104: // MPEG2 Audio
		return 90000
	case 105: // AAC
		return 48000
	case 106: // AC3
		return 48000
	case 107: // G711
		return 8000
	case 108: // G722
		return 8000
	case 109: // G723
		return 8000
	case 110: // G726
		return 8000
	case 111: // G729
		return 8000
	case 112: // G729D
		return 8000
	case 113: // G729E
		return 8000
	case 114: // GSM
		return 8000
	case 115: // GSM-EFR
		return 8000
	case 116: // GSM-HR
		return 8000
	case 117: // L8
		return 8000
	case 118: // L16
		return 44100
	case 119: // L24
		return 48000
	case 120: // LPC
		return 8000
	case 121: // MPA
		return 90000
	case 122: // PCMA
		return 8000
	case 123: // PCMU
		return 8000
	case 124: // QCELP
		return 8000
	case 125: // VDVI
		return 8000
	default:
		return 90000
	}
}

// getCodecForPayloadType returns the codec configuration for a given payload type.
func getCodecForPayloadType(payloadType uint8) fmp4.Codec {
	switch payloadType {
	case 96: // H264
		return &fmp4.CodecH264{
			SPS: formatprocessor.H264DefaultSPS,
			PPS: formatprocessor.H264DefaultPPS,
		}
	case 97: // H265
		return &fmp4.CodecH265{
			VPS: formatprocessor.H265DefaultVPS,
			SPS: formatprocessor.H265DefaultSPS,
			PPS: formatprocessor.H265DefaultPPS,
		}
	case 98: // VP8
		// Use H264 as a fallback for VP8 since VP8 is not directly supported
		return &fmp4.CodecH264{
			SPS: formatprocessor.H264DefaultSPS,
			PPS: formatprocessor.H264DefaultPPS,
		}
	case 99: // VP9
		// Use H264 as a fallback for VP9 since VP9 is not directly supported
		return &fmp4.CodecH264{
			SPS: formatprocessor.H264DefaultSPS,
			PPS: formatprocessor.H264DefaultPPS,
		}
	case 100: // MPEG4 Video
		return &fmp4.CodecMPEG4Video{
			Config: formatprocessor.MPEG4VideoDefaultConfig,
		}
	case 101: // MPEG1 Video
		return &fmp4.CodecMPEG1Video{}
	case 102: // MJPEG
		return &fmp4.CodecMJPEG{}
	case 103: // MPEG1 Audio
		return &fmp4.CodecMPEG1Audio{}
	case 104: // MPEG2 Audio
		return &fmp4.CodecMPEG1Audio{} // Use MPEG1 Audio as fallback
	case 105: // AAC
		return &fmp4.CodecMPEG4Audio{}
	case 106: // AC3
		return &fmp4.CodecAC3{}
	case 107: // G711
		return &fmp4.CodecLPCM{}
	case 108: // G722
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 109: // G723
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 110: // G726
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 111: // G729
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 112: // G729D
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 113: // G729E
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 114: // GSM
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 115: // GSM-EFR
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 116: // GSM-HR
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 117: // L8
		return &fmp4.CodecLPCM{}
	case 118: // L16
		return &fmp4.CodecLPCM{}
	case 119: // L24
		return &fmp4.CodecLPCM{}
	case 120: // LPC
		return &fmp4.CodecLPCM{}
	case 121: // MPA
		return &fmp4.CodecMPEG1Audio{} // Use MPEG1 Audio as fallback
	case 122: // PCMA
		return &fmp4.CodecLPCM{}
	case 123: // PCMU
		return &fmp4.CodecLPCM{}
	case 124: // QCELP
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	case 125: // VDVI
		return &fmp4.CodecLPCM{} // Use LPCM as fallback
	default:
		// Default to H264
		return &fmp4.CodecH264{
			SPS: formatprocessor.H264DefaultSPS,
			PPS: formatprocessor.H264DefaultPPS,
		}
	}
}
