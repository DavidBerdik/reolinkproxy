package main

import (
	"encoding/binary"
	"net/http/httptest"
	"testing"

	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/pion/rtp"

	"github.com/shareed2k/reolinkproxy/pkg/baichuan"
)

func TestFixH265AggregationTemporalID(t *testing.T) {
	t.Parallel()

	firstNALU := []byte{0x40, 0x01, 0xaa, 0xbb}
	payload := make([]byte, 2+2+len(firstNALU))
	payload[0] = 48 << 1
	payload[1] = 0x00
	binary.BigEndian.PutUint16(payload[2:4], uint16(len(firstNALU)))
	copy(payload[4:], firstNALU)

	pkt := &rtp.Packet{Payload: payload}
	fixH265AggregationTemporalID([]*rtp.Packet{pkt})

	if got, want := pkt.Payload[0], (firstNALU[0]&0x81)|(48<<1); got != want {
		t.Fatalf("payload[0] = %#x, want %#x", got, want)
	}
	if got, want := pkt.Payload[1], firstNALU[1]; got != want {
		t.Fatalf("payload[1] = %#x, want %#x", got, want)
	}
}

func TestParseAACAccessUnits(t *testing.T) {
	t.Parallel()

	raw, err := mpeg4audio.ADTSPackets{
		&mpeg4audio.ADTSPacket{
			Type:         mpeg4audio.ObjectTypeAACLC,
			SampleRate:   16000,
			ChannelCount: 1,
			AU:           []byte{0x11, 0x22, 0x33},
		},
		&mpeg4audio.ADTSPacket{
			Type:         mpeg4audio.ObjectTypeAACLC,
			SampleRate:   16000,
			ChannelCount: 1,
			AU:           []byte{0x44, 0x55},
		},
	}.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	aus, cfg, err := parseAACAccessUnits(raw)
	if err != nil {
		t.Fatalf("parseAACAccessUnits() error = %v", err)
	}
	if got, want := len(aus), 2; got != want {
		t.Fatalf("len(aus) = %d, want %d", got, want)
	}
	if got, want := cfg.SampleRate, 16000; got != want {
		t.Fatalf("cfg.SampleRate = %d, want %d", got, want)
	}
	if got, want := cfg.ChannelCount, 1; got != want {
		t.Fatalf("cfg.ChannelCount = %d, want %d", got, want)
	}
}

func TestTimestampUnwrapperWrapsForward(t *testing.T) {
	t.Parallel()

	var timestamps timestampUnwrapper
	if got, want := timestamps.unwrap(0xfffffff0), uint64(0xfffffff0); got != want {
		t.Fatalf("first unwrap = %d, want %d", got, want)
	}
	if got, want := timestamps.unwrap(20), uint64(0x100000014); got != want {
		t.Fatalf("wrapped unwrap = %d, want %d", got, want)
	}
}

func TestAudioTimestampForPacketUsesFallbackWithoutUpdatingAudioClock(t *testing.T) {
	t.Parallel()

	var audioTimestamps timestampUnwrapper
	fallback := mediaTimestamp{
		Microseconds:  3_000_000_000,
		Valid:         true,
		Authoritative: true,
	}

	got := audioTimestampForPacket(baichuan.MediaPacket{Kind: baichuan.MediaPacketAAC}, &audioTimestamps, fallback)
	if got != fallback {
		t.Fatalf("audioTimestampForPacket() = %+v, want %+v", got, fallback)
	}
	if audioTimestamps.highest != 0 {
		t.Fatalf("audioTimestamps.highest = %d, want 0", audioTimestamps.highest)
	}
}

func TestAudioTimestampForPacketUsesAuthoritativePacketTimestamp(t *testing.T) {
	t.Parallel()

	var audioTimestamps timestampUnwrapper
	fallback := mediaTimestamp{
		Microseconds:  3_000_000_000,
		Valid:         true,
		Authoritative: true,
	}
	packet := baichuan.MediaPacket{
		Kind:               baichuan.MediaPacketAAC,
		TimestampMicrosecs: 1234,
		HasTimestamp:       true,
	}

	got := audioTimestampForPacket(packet, &audioTimestamps, fallback)
	want := mediaTimestamp{
		Microseconds:  1234,
		Valid:         true,
		Authoritative: true,
	}
	if got != want {
		t.Fatalf("audioTimestampForPacket() = %+v, want %+v", got, want)
	}
	if audioTimestamps.highest != 1234 {
		t.Fatalf("audioTimestamps.highest = %d, want 1234", audioTimestamps.highest)
	}
}

func TestSOAPActionPrefersExactElement(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "http://example.test/onvif/media_service", nil)
	body := `<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl"><soap:Body><trt:GetProfiles/></soap:Body></soap:Envelope>`

	got := soapAction(req, body, []string{"GetProfile", "GetProfiles"})
	if got != "GetProfiles" {
		t.Fatalf("soapAction() = %q, want %q", got, "GetProfiles")
	}
}
