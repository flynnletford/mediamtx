package recorder

import (
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/flynnletford/mediamtx/src/conf"
	"github.com/flynnletford/mediamtx/src/logger"
	"github.com/flynnletford/mediamtx/src/protocols/webrtc"
	"github.com/flynnletford/mediamtx/src/stream"
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
	// Create a stream from the peer connection
	var strm *stream.Stream
	medias, err := webrtc.ToStream(pc, &strm)
	if err != nil {
		return err
	}

	// Set up the stream
	strm = &stream.Stream{
		WriteQueueSize:    512,
		UDPMaxPayloadSize: 1472,
		Desc: &description.Session{
			Medias: medias,
		},
		GenerateRTPPackets: false,
		Parent:             r,
	}
	err = strm.Initialize()
	if err != nil {
		return err
	}

	// Set the stream in the recorder
	r.currentInstance.rec.Stream = strm

	return nil
}
