package playback

import (
	"io"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
)

type MuxerMP4Track struct {
	pmp4.Track
	LastDTS int64
}

func findTrackMP4(tracks []*MuxerMP4Track, id int) *MuxerMP4Track {
	for _, track := range tracks {
		if track.ID == id {
			return track
		}
	}
	return nil
}

type MuxerMP4 struct {
	W io.Writer

	Tracks   []*MuxerMP4Track
	CurTrack *MuxerMP4Track
}

func (w *MuxerMP4) WriteInit(init *fmp4.Init) {
	w.Tracks = make([]*MuxerMP4Track, len(init.Tracks))

	for i, track := range init.Tracks {
		w.Tracks[i] = &MuxerMP4Track{
			Track: pmp4.Track{
				ID:        track.ID,
				TimeScale: track.TimeScale,
				Codec:     track.Codec,
			},
		}
	}
}

func (w *MuxerMP4) SetTrack(trackID int) {
	w.CurTrack = findTrackMP4(w.Tracks, trackID)
}

func (w *MuxerMP4) WriteSample(
	dts int64,
	ptsOffset int32,
	isNonSyncSample bool,
	payloadSize uint32,
	getPayload func() ([]byte, error),
) error {
	// remove GOPs before the GOP of the first frame
	if (dts < 0 || (dts >= 0 && w.CurTrack.LastDTS < 0)) && !isNonSyncSample {
		w.CurTrack.Samples = nil
	}

	if w.CurTrack.Samples == nil {
		w.CurTrack.TimeOffset = int32(dts)
	} else {
		diff := dts - w.CurTrack.LastDTS
		if diff < 0 {
			diff = 0
		}
		w.CurTrack.Samples[len(w.CurTrack.Samples)-1].Duration = uint32(diff)
	}

	// prevent warning "edit list: 1 Missing key frame while searching for timestamp: 0"
	if !isNonSyncSample {
		ptsOffset = 0
	}

	w.CurTrack.Samples = append(w.CurTrack.Samples, &pmp4.Sample{
		PTSOffset:       ptsOffset,
		IsNonSyncSample: isNonSyncSample,
		PayloadSize:     payloadSize,
		GetPayload:      getPayload,
	})
	w.CurTrack.LastDTS = dts

	return nil
}

func (w *MuxerMP4) WriteFinalDTS(dts int64) {
	diff := dts - w.CurTrack.LastDTS
	if diff < 0 {
		diff = 0
	}
	w.CurTrack.Samples[len(w.CurTrack.Samples)-1].Duration = uint32(diff)
}

func (w *MuxerMP4) Flush() error {
	h := pmp4.Presentation{
		Tracks: make([]*pmp4.Track, len(w.Tracks)),
	}

	for i, track := range w.Tracks {
		h.Tracks[i] = &track.Track
	}

	return h.Marshal(w.W)
}
