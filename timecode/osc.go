package timecode

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

func ListenOSC(ctx context.Context, address string, coordinator *Coordinator) error {
	packet, err := net.ListenPacket("udp", address)
	if err != nil {
		return err
	}
	defer packet.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = packet.Close() })
	defer stopClose()
	buffer := make([]byte, 2048)
	for {
		n, _, err := packet.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		position, err := ParseOSC(buffer[:n], coordinator.FrameRate())
		if err == nil {
			_ = coordinator.Update(SourceOSC, position, true)
		}
	}
}

func ParseOSC(packet []byte, frameRate float64) (time.Duration, error) {
	address, offset, err := oscString(packet, 0)
	if err != nil || (address != "/timecode" && address != "/cusus/timecode") {
		return 0, errors.New("unsupported OSC timecode address")
	}
	types, offset, err := oscString(packet, offset)
	if err != nil || len(types) != 2 || types[0] != ',' {
		return 0, errors.New("OSC timecode requires one argument")
	}
	switch types[1] {
	case 'f':
		if offset+4 > len(packet) {
			return 0, errors.New("short OSC float")
		}
		seconds := math.Float32frombits(binary.BigEndian.Uint32(packet[offset:]))
		return time.Duration(float64(seconds) * float64(time.Second)), nil
	case 'd':
		if offset+8 > len(packet) {
			return 0, errors.New("short OSC double")
		}
		seconds := math.Float64frombits(binary.BigEndian.Uint64(packet[offset:]))
		return time.Duration(seconds * float64(time.Second)), nil
	case 'i':
		if offset+4 > len(packet) {
			return 0, errors.New("short OSC integer")
		}
		return time.Duration(int32(binary.BigEndian.Uint32(packet[offset:]))) * time.Millisecond, nil
	case 's':
		value, _, err := oscString(packet, offset)
		if err != nil {
			return 0, err
		}
		return parseTimecodeString(value, frameRate)
	default:
		return 0, errors.New("unsupported OSC timecode argument")
	}
}

func parseTimecodeString(value string, frameRate float64) (time.Duration, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 {
		return 0, errors.New("timecode must be HH:MM:SS:FF")
	}
	values := [4]int{}
	for index := range parts {
		parsed, err := strconv.Atoi(parts[index])
		if err != nil {
			return 0, err
		}
		values[index] = parsed
	}
	return framePosition(values[0], values[1], values[2], values[3], frameRate)
}

func oscString(packet []byte, offset int) (string, int, error) {
	if offset >= len(packet) {
		return "", 0, errors.New("short OSC string")
	}
	end := offset
	for end < len(packet) && packet[end] != 0 {
		end++
	}
	if end == len(packet) {
		return "", 0, errors.New("unterminated OSC string")
	}
	next := (end + 4) &^ 3
	if next > len(packet) {
		return "", 0, errors.New("short OSC padding")
	}
	return string(packet[offset:end]), next, nil
}
