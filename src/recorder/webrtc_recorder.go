package recorder

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/flynnletford/mediamtx/src/conf"
	"github.com/flynnletford/mediamtx/src/logger"
	webrtcprotocol "github.com/flynnletford/mediamtx/src/protocols/webrtc"
	"github.com/flynnletford/mediamtx/src/stream"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// SimpleLogger is a simple logger implementation.
type SimpleLogger struct{}

// Log implements logger.Writer.
func (l *SimpleLogger) Log(level logger.Level, format string, args ...interface{}) {
	log.Printf("[%s] %s", level, fmt.Sprintf(format, args...))
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
			Parent:            &SimpleLogger{},
		},
	}
	r.currentInstance.initialize()

	go r.run()
}

// Close closes the recorder.
func (r *WebRTCRecorder) Close() {
	log.Printf("recording stopped")
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
				Parent:            &SimpleLogger{},
			},
		}
		r.currentInstance.initialize()
	}
}

// RecordFromPeerConnection starts recording from a WebRTC peer connection.
func (r *WebRTCRecorder) RecordFromPeerConnection(pc *pionwebrtc.PeerConnection) error {
	// Create a stream
	strm := &stream.Stream{
		WriteQueueSize:     512,
		UDPMaxPayloadSize:  1472,
		GenerateRTPPackets: false,
		Parent:             &SimpleLogger{},
	}

	// Create our internal PeerConnection type
	internalPC := &webrtcprotocol.PeerConnection{
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            false,
		Log:                &SimpleLogger{},
		PeerConnection:     pc,
	}

	// Initialize the PeerConnection
	if err := internalPC.Start(); err != nil {
		return fmt.Errorf("failed to initialize peer connection: %v", err)
	}
	defer internalPC.Close()

	// // Get the offer from the Pion WebRTC PeerConnection
	// offer := pc.LocalDescription()
	// if offer == nil {
	// 	return fmt.Errorf("no local description available")
	// }

	// // Create a full answer using the offer
	// ctx := context.Background()
	// answer, err := internalPC.CreateFullAnswer(ctx, offer)
	// if err != nil {
	// 	return fmt.Errorf("failed to create full answer: %v", err)
	// }

	// // Set the answer on the Pion WebRTC PeerConnection
	// if err := pc.SetRemoteDescription(*answer); err != nil {
	// 	return fmt.Errorf("failed to set remote description: %v", err)
	// }

	// Handle incoming tracks
	if err := internalPC.GatherIncomingTracks(context.Background()); err != nil {
		return fmt.Errorf("failed to gather incoming tracks: %v", err)
	}

	// Map the WebRTC connection to a MediaMTX stream
	medias, err := webrtcprotocol.ToStream(internalPC, &strm)
	if err != nil {
		return fmt.Errorf("failed to map WebRTC connection to stream: %v", err)
	}

	// Initialize the stream with the media descriptions
	strm.Desc = &description.Session{
		Medias: medias,
	}
	if err := strm.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize stream: %v", err)
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
		Stream:            strm,
		Parent:            &SimpleLogger{},
	}

	// Initialize the recorder
	rec.Initialize()

	// Set the current instance
	r.currentInstance = rec.currentInstance

	return nil
}
