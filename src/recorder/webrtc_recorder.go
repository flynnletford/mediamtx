package recorder

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/rtcpreceiver"
	"github.com/bluenviron/gortsplib/v4/pkg/rtpreorderer"
	"github.com/flynnletford/mediamtx/src/conf"
	"github.com/flynnletford/mediamtx/src/logger"
	"github.com/flynnletford/mediamtx/src/stream"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// NullLogger is a logger that does nothing.
type NullLogger struct{}

// Log implements logger.Writer.
func (l *NullLogger) Log(level logger.Level, format string, args ...interface{}) {
	// Do nothing
}

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
		Parent:          &NullLogger{},

		terminate: make(chan struct{}),
		done:      make(chan struct{}),
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

		if track.ID() != "video" {
			return
		}

		// Only process video tracks
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

		default:
			// Skip non-video tracks
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

		// Initialize stream if not already initialized
		if strm.Desc == nil {
			// Initialize the stream with the current media descriptions
			strm.Desc = &description.Session{
				Medias: medias,
			}
			if err := strm.Initialize(); err != nil {
				r.Log(logger.Error, "failed to initialize stream: %v", err)
				return
			}

			// Create a new recorder instance
			rec := &Recorder{
				PathFormat:        r.PathFormat,
				Format:            r.Format,
				PartDuration:      r.PartDuration,
				SegmentDuration:   r.SegmentDuration,
				PathName:          r.PathName,
				OnSegmentCreate:   r.OnSegmentCreate,
				OnSegmentComplete: r.OnSegmentComplete,
				Parent:            r,
				Stream:            strm,
			}

			// Initialize the recorder
			rec.Initialize()

			// Set the current instance
			r.currentInstance = rec.currentInstance
		}

		// Set up RTCP receiver for accurate timestamps
		rtcpReceiver := &rtcpreceiver.RTCPReceiver{
			ClockRate: int(track.Codec().ClockRate),
			Period:    1 * time.Second,
		}
		if err := rtcpReceiver.Initialize(); err != nil {
			r.Log(logger.Error, "failed to initialize RTCP receiver: %v", err)
			return
		}
		defer rtcpReceiver.Close()

		// Read RTCP packets in a separate goroutine
		go func() {
			buf := make([]byte, 1500)
			for {
				n, _, err := receiver.Read(buf)
				if err != nil {
					return
				}

				pkts, err := rtcp.Unmarshal(buf[:n])
				if err != nil {
					r.Log(logger.Error, "failed to unmarshal RTCP packet: %v", err)
					continue
				}

				for _, pkt := range pkts {
					if sr, ok := pkt.(*rtcp.SenderReport); ok {
						rtcpReceiver.ProcessSenderReport(sr, time.Now())
					}
				}
			}
		}()

		// Handle RTP packets
		reorderer := &rtpreorderer.Reorderer{}
		reorderer.Initialize()

		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}

			// Process packet through reorderer
			packets, lost := reorderer.Process(pkt)
			if lost != 0 {
				r.Log(logger.Warn, "%d RTP packets lost", lost)
			}

			// Process packet through RTCP receiver
			if err := rtcpReceiver.ProcessPacket(pkt, time.Now(), true); err != nil {
				r.Log(logger.Warn, "failed to process RTCP packet: %v", err)
				continue
			}

			// Get NTP timestamp from RTCP receiver
			ntp, avail := rtcpReceiver.PacketNTP(pkt.Timestamp)
			if !avail {
				r.Log(logger.Warn, "received RTP packet without absolute time, skipping it")
				continue
			}

			// Process all packets from reorderer
			for _, pkt := range packets {
				// Skip empty packets
				if len(pkt.Payload) == 0 {
					continue
				}

				// Calculate PTS from NTP timestamp
				pts := int64(ntp.Sub(time.Unix(0, 0)) / time.Nanosecond)

				// Only write packets if the stream is initialized
				if strm.Desc != nil {
					strm.WriteRTPPacket(medi, mediaFormat, pkt, ntp, pts)
				}
			}
		}
	})

	// Wait for the first track
	select {
	case <-trackChan:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("no tracks received within timeout")
	}

	return nil
}
