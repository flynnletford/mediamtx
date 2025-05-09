package recorder

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/flynnletford/mediamtx/src/conf"
	"github.com/flynnletford/mediamtx/src/logger"
	"github.com/flynnletford/mediamtx/src/stream"
	"github.com/pion/webrtc/v4"
)

// WebRTCRecorder records from a WebRTC peer connection.
type WebRTCRecorder struct {
	PathFormat        string
	Format            conf.RecordFormat
	PartDuration      time.Duration
	SegmentDuration   time.Duration
	PathName          string
	OnSegmentCreate   OnSegmentCreateFunc
	OnSegmentComplete OnSegmentCompleteFunc
	Parent            logger.Writer

	restartPause time.Duration

	currentInstance *recorderInstance

	terminate chan struct{}
	done      chan struct{}
}

// NewWebRTCRecorder creates a new WebRTCRecorder.
func NewWebRTCRecorder(filePath string) *WebRTCRecorder {
	return &WebRTCRecorder{
		PathFormat:      filePath,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    24 * time.Hour,
		SegmentDuration: 10 * time.Second,
		restartPause:    2 * time.Second,
	}
}

// Initialize initializes the recorder.
func (r *WebRTCRecorder) Initialize() {
	if r.OnSegmentCreate == nil {
		r.OnSegmentCreate = func(string) {}
	}
	if r.OnSegmentComplete == nil {
		r.OnSegmentComplete = func(string, time.Duration) {}
	}

	r.terminate = make(chan struct{})
	r.done = make(chan struct{})

	r.currentInstance = &recorderInstance{
		rec: &Recorder{
			PathFormat:        r.PathFormat,
			Format:            r.Format,
			PartDuration:      r.PartDuration,
			SegmentDuration:   r.SegmentDuration,
			PathName:          r.PathName,
			OnSegmentCreate:   r.OnSegmentCreate,
			OnSegmentComplete: r.OnSegmentComplete,
			Parent:            r,
		},
	}
	r.currentInstance.initialize()

	go r.run()
}

// Log implements logger.Writer.
func (r *WebRTCRecorder) Log(level logger.Level, format string, args ...interface{}) {
	r.Parent.Log(level, "[recorder] "+format, args...)
}

// Close closes the recorder.
func (r *WebRTCRecorder) Close() {
	r.Log(logger.Info, "recording stopped")
	close(r.terminate)
	<-r.done
}

func (r *WebRTCRecorder) run() {
	defer close(r.done)

	for {
		select {
		case <-r.currentInstance.done:
			r.currentInstance.close()
		case <-r.terminate:
			r.currentInstance.close()
			return
		}

		select {
		case <-time.After(r.restartPause):
		case <-r.terminate:
			return
		}

		r.currentInstance = &recorderInstance{
			rec: &Recorder{
				PathFormat:        r.PathFormat,
				Format:            r.Format,
				PartDuration:      r.PartDuration,
				SegmentDuration:   r.SegmentDuration,
				PathName:          r.PathName,
				OnSegmentCreate:   r.OnSegmentCreate,
				OnSegmentComplete: r.OnSegmentComplete,
				Parent:            r,
			},
		}
		r.currentInstance.initialize()
	}
}

// RecordFromPeerConnection starts recording from a WebRTC peer connection.
func (r *WebRTCRecorder) RecordFromPeerConnection(pc *webrtc.PeerConnection) error {
	// Create a stream
	strm := &stream.Stream{
		WriteQueueSize:     512,
		UDPMaxPayloadSize:  1472,
		GenerateRTPPackets: false,
		Parent:             r,
	}

	// Create a channel to wait for the first track
	trackChan := make(chan struct{})
	var medias []*description.Media

	// Handle incoming tracks
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		var typ description.MediaType
		var mediaFormat rtspformat.Format

		switch strings.ToLower(track.Codec().MimeType) {
		case strings.ToLower(webrtc.MimeTypeAV1):
			typ = description.MediaTypeVideo
			mediaFormat = &rtspformat.AV1{
				PayloadTyp: uint8(track.PayloadType()),
			}

		case strings.ToLower(webrtc.MimeTypeVP9):
			typ = description.MediaTypeVideo
			mediaFormat = &rtspformat.VP9{
				PayloadTyp: uint8(track.PayloadType()),
			}

		case strings.ToLower(webrtc.MimeTypeVP8):
			typ = description.MediaTypeVideo
			mediaFormat = &rtspformat.VP8{
				PayloadTyp: uint8(track.PayloadType()),
			}

		case strings.ToLower(webrtc.MimeTypeH265):
			typ = description.MediaTypeVideo
			mediaFormat = &rtspformat.H265{
				PayloadTyp: uint8(track.PayloadType()),
			}

		case strings.ToLower(webrtc.MimeTypeH264):
			typ = description.MediaTypeVideo
			mediaFormat = &rtspformat.H264{
				PayloadTyp:        uint8(track.PayloadType()),
				PacketizationMode: 1,
			}

		case strings.ToLower(webrtc.MimeTypeOpus):
			typ = description.MediaTypeAudio
			mediaFormat = &rtspformat.Opus{
				PayloadTyp: uint8(track.PayloadType()),
				ChannelCount: func() int {
					if strings.Contains(track.Codec().SDPFmtpLine, "stereo=1") {
						return 2
					}
					return 1
				}(),
			}

		case strings.ToLower(webrtc.MimeTypeG722):
			typ = description.MediaTypeAudio
			mediaFormat = &rtspformat.G722{}

		case strings.ToLower(webrtc.MimeTypePCMU):
			channels := int(track.Codec().Channels)
			if channels == 0 {
				channels = 1
			}

			typ = description.MediaTypeAudio
			mediaFormat = &rtspformat.G711{
				PayloadTyp: func() uint8 {
					if channels > 1 {
						return 118
					}
					return 0
				}(),
				MULaw:        true,
				SampleRate:   8000,
				ChannelCount: channels,
			}

		case strings.ToLower(webrtc.MimeTypePCMA):
			channels := int(track.Codec().Channels)
			if channels == 0 {
				channels = 1
			}

			typ = description.MediaTypeAudio
			mediaFormat = &rtspformat.G711{
				PayloadTyp: func() uint8 {
					if channels > 1 {
						return 119
					}
					return 8
				}(),
				MULaw:        false,
				SampleRate:   8000,
				ChannelCount: channels,
			}

		default:
			r.Log(logger.Warn, "unsupported codec: %+v", track.Codec().RTPCodecCapability)
			return
		}

		medi := &description.Media{
			Type:    typ,
			Formats: []rtspformat.Format{mediaFormat},
		}

		medias = append(medias, medi)

		// Signal that we have received a track
		select {
		case trackChan <- struct{}{}:
		default:
		}

		// Handle RTP packets
		var lastPTS time.Duration
		var lastRTPTime uint32
		clockRate := float64(track.Codec().ClockRate)

		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}

			// Calculate PTS from RTP timestamp
			if lastRTPTime == 0 {
				lastRTPTime = pkt.Timestamp
			}

			// Handle RTP timestamp wrap-around
			var diff uint32
			if pkt.Timestamp >= lastRTPTime {
				diff = pkt.Timestamp - lastRTPTime
			} else {
				diff = (0xFFFFFFFF - lastRTPTime) + pkt.Timestamp + 1
			}

			lastRTPTime = pkt.Timestamp
			lastPTS += time.Duration(float64(diff) / clockRate * float64(time.Second))

			// Convert PTS to int64 (nanoseconds)
			pts := int64(lastPTS / time.Nanosecond)

			strm.WriteRTPPacket(medi, mediaFormat, pkt, time.Now(), pts)
		}
	})

	// Wait for the first track
	select {
	case <-trackChan:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("no tracks received within timeout")
	}

	// Initialize the stream
	strm.Desc = &description.Session{
		Medias: medias,
	}
	err := strm.Initialize()
	if err != nil {
		return err
	}

	// Create a new recorder instance
	r.currentInstance = &recorderInstance{
		rec: &Recorder{
			PathFormat:        r.PathFormat,
			Format:            r.Format,
			PartDuration:      r.PartDuration,
			SegmentDuration:   r.SegmentDuration,
			PathName:          r.PathName,
			OnSegmentCreate:   r.OnSegmentCreate,
			OnSegmentComplete: r.OnSegmentComplete,
			Parent:            r,
			Stream:            strm,
		},
	}

	// Initialize the recorder instance
	r.currentInstance.initialize()

	return nil
}
