package recorder

import (
	"encoding/json"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/flynnletford/mediamtx/src/stream"
	"github.com/pion/rtp"
)

const streamJSONString string = `{
	"WriteQueueSize": 512,
	"UDPMaxPayloadSize": 1472,
	"Desc": {
	  "BaseURL": null,
	  "Title": "No Name",
	  "Multicast": false,
	  "FECGroups": null,
	  "Medias": [
		{
		  "Type": "video",
		  "ID": "",
		  "IsBackChannel": false,
		  "Control": "streamid=0",
		  "Formats": [
			{
			  "PayloadTyp": 96,
			  "SPS": "Z/QAHpGWgKA9sBEAAAMAAQAAAwAyjxYuoA==",
			  "PPS": "aM4PGSA=",
			  "PacketizationMode": 1
			}
		  ]
		}
	  ]
	},
	"GenerateRTPPackets": false,
	"Parent": {}
  }`

const mediMarshalledString string = `{
	"Type": "video",
	"ID": "",
	"IsBackChannel": false,
	"Control": "streamid=0",
	"Formats": [
	  {
		"PayloadTyp": 96,
		"SPS": "Z/QAHpGWgKA9sBEAAAMAAQAAAwAyjxYuoA==",
		"PPS": "aM4PGSA=",
		"PacketizationMode": 1
	  }
	]
}`

//   pktMarshalled: {
// 	"Version": 2,
// 	"Padding": false,
// 	"Extension": false,
// 	"Marker": true,
// 	"PayloadType": 96,
// 	"SequenceNumber": 4198,
// 	"Timestamp": 833071433,
// 	"SSRC": 1106789997,
// 	"CSRC": [],
// 	"ExtensionProfile": 0,
// 	"Extensions": null,
// 	"PayloadOffset": 0,
// 	"Payload": "GAAHQZqAJoBQwAAIQQFCagCaAZMACUEAtJqAJoBQwAAJQQBBJqAJoBQwAAlBAFUmoAmgFDACpkEAaSagCaBTBRLe//wUeCCJ1qtf/e73+JigYOVxQMcrijFGWMUYrPxW/1qKxWKxWKxRisUfiYoxRijFGKDFBigxQdaqqqKxQYrFBijFYoxXggif3v/3v+CjwUSa1/8EEu9//xMUGKMUGKMUGKMUGKNa1VVFYrFYrFGKMUYo8EET9761//wUeCCJ/61rX/4KPxNVVVVVFMUMUxQxcXFxf3vwQRP/WtOn/8FHgok/e/4KInTp1r///iYoAxQAYoAxQAYoAxQBigDFAGLi4uLi4uLi4uLi4uKYpimKeCjwUSVJ1Z//BRJrX6dPBR+JigDFAGKAMUAYoAMUAYoAMUAYuLi4uLi4uLi4pi4pi4pi4p4KPBR4KJf/e+CiJ///btp0/iYpihimKGKYoYpih3u7u4uLi4uLimLingokvf/4KPBB4IPxN3d3d3FYoxWKMUYrFGK/rXxN3d3d3FYrFYrFGKMUYo1rWvxN3d3d3FGKxRisVisVita61wUSfrX8EEn73/BxE/vdVVYo4o//xF73v//9BP//8VrX61+x///go8Rl+E6F73+96H//5fWv1r+I3Ec+Dswhwn//+K3v97/P///n2J4TkU//+v//mX8zEzHzMTMYTmU//+JqP//B67u7+7u/xWpfFTMOFBQQnIx73+9///x9B97/e8VDFM8FERWta///hOn//6f///vf73hOX//+h//+VR61+tc8vIyxYKP7iOE///xW9/vf6///go4T///B19VVfVVX2L//8FHCf//7f3v97sf//z7EZieXkZz90fsc867mYLEaiNnzqHKeRSueZTz/PMwK89Doc/eE6H//6F//6a1vetb3z0088vQ88vsQw8v5+6P2MIHkUrnmU8/zzMc9Doc/eehoSc9NPPL0ONPL+AGVQQAgiagCawUf/cvv/8Td89P/BDNFGKNf55EUGKDJ3y9koukSirpCjFBj7V+CjFb7XssFEn3T/wUzRQYoz4TrIUYoNeyKv5dJVSQMUYrH/VCsVj7J9+aKDFBr/PwUYo34nZaqpeKMVr2T8FYrXtX3go8FM0UGKMf8v8KMUadln56yUijFZyCnZYUYo1/nkcEH/+I8FM1VISYRVclBTFMmKGQuKahqo87/4KP//4KZooAxQAYGrcsNXywoAxQBi65YXXLC4uQgqGouLyTBTFxjyDLCmKbKKPYIPEf/4KZooAMUBguuWF1ywoAxQBjV8sNW5YXF0iEEXF1DJli4uS1DIXFOGS8FH//8FM0UxQwZmrLFDFDUNSmava3dxcXGGrMFMXUNVDLBR/wUzRQxTDGWKZqKYuoaw7nJL/X2YuLkIqPC6xBjLeam+74oxA85BTssWxW+32q9aSXmu58XsneqKxR1B9lhRitf6g6kp1QNLBR///3cUdqKP+f5/n+f5/nxfPi8ER/n+f5/n+fwRn+f5/n+fwSQ",
// 	"PaddingSize": 0,
// 	"Raw": null
//   }
//   ntpMarshalled: "2025-05-12T09:28:44.435813+12:00"
//   ptsMarshalled: 590400

type RTPRecorder2 struct {
	Stream *stream.Stream
	Media  *description.Media
}

func NewRTPRecorder2() *RTPRecorder2 {

	stream := &stream.Stream{}
	if err := json.Unmarshal([]byte(streamJSONString), stream); err != nil {
		panic(err)
	}

	media := &description.Media{}
	if err := json.Unmarshal([]byte(mediMarshalledString), media); err != nil {
		panic(err)
	}

	return &RTPRecorder2{
		Stream: stream,
		Media:  media,
	}
}

func (r *RTPRecorder2) WriteRTPPacket(pkt *rtp.Packet) {

	// H264 clock rate.
	clockRate := uint32(90000)
	// Reference time for NTP calculation.
	// Calculate NTP time from RTP timestamp.
	ntpTime := RTPToNTP(pkt.Timestamp, clockRate, time.Now())

	pts := RTPToPTS(pkt.Timestamp, clockRate)

	r.Stream.WriteRTPPacket(r.Media, r.Media.Formats[0], pkt, ntpTime, pts)
}

// Function to calculate NTP Time from RTP Timestamp
func RTPToNTP(rtpTimestamp uint32, clockRate uint32, referenceTime time.Time) time.Time {
	// Calculate the NTP offset (seconds since the reference time)
	ntpSeconds := float64(rtpTimestamp) / float64(clockRate)
	ntpTime := referenceTime.Add(time.Duration(ntpSeconds * float64(time.Second)))

	return ntpTime
}

// Function to calculate PTS from RTP Timestamp
func RTPToPTS(rtpTimestamp uint32, clockRate uint32) int64 {
	// PTS is typically the RTP timestamp scaled by the clock rate
	return int64(rtpTimestamp)
}
