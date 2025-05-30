package mft

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

func getTopHeaderConst() [8]byte { return [8]byte{109, 102, 116, 102, 114, 97, 109, 101} }
func getEndFooterConst() [16]byte {
	return [16]byte{98, 121, 32, 108, 117, 99, 97, 114, 105, 103, 110, 97, 110, 101, 115, 101}
}

const (
	maxMftBodySizeByte  = 5000
	mftHeaderSizeByte   = 11
	mftFooterSizeByte   = 16
	maxMftFrameSizeByte = maxMftBodySizeByte + mftHeaderSizeByte + mftFooterSizeByte
	minMftFrameSizeByte = 1 + mftHeaderSizeByte + mftFooterSizeByte
	mftFrameDelay       = 50 * time.Millisecond
	mftFrameTimeout     = 5 * time.Second
)

func MFT_PAYLOAD_SIZE() int            { return maxMftBodySizeByte }
func MFT_FRAME_DELAY() time.Duration   { return mftFrameDelay }
func MFT_FRAME_TIMEOUT() time.Duration { return mftFrameTimeout }

type MftFrameType byte

const MftFrameType_START MftFrameType = 0
const MftFrameType_TRANSMISSION MftFrameType = 1
const MftFrameType_END MftFrameType = 2

type MftFrame struct {
	header MftHeader
	body   MftBody
	footer MftFooter
}

type MftProgress struct {
	FrameTotal    uint32
	FrameReceived uint32
	Percent       float32
}

type MftHeader struct {
	top       [8]byte
	frameType MftFrameType
	frameNo   uint16
}

type MftBody struct {
	bodyFrame []byte
}

type MftFooter struct {
	end [16]byte
}

func buildMftHeader(frameNo uint16, frameType MftFrameType) MftHeader {
	return MftHeader{top: getTopHeaderConst(), frameType: frameType, frameNo: frameNo}
}

func buildMftFooter() MftFooter {
	return MftFooter{end: getEndFooterConst()}
}

func buildMftBody(b []byte) MftBody {
	return MftBody{bodyFrame: b}
}

func (f *MftFrame) Encode() []byte {
	var byteFrame []byte
	for i := range f.header.top {
		byteFrame = append(byteFrame, f.header.top[i])
	}
	byteFrame = append(byteFrame, byte(f.header.frameType))
	frameNoByte := make([]byte, 2)
	binary.LittleEndian.PutUint16(frameNoByte, f.header.frameNo)
	byteFrame = append(byteFrame, frameNoByte...)
	byteFrame = append(byteFrame, f.body.bodyFrame...)
	for i := range f.footer.end {
		byteFrame = append(byteFrame, f.footer.end[i])
	}
	return byteFrame
}

func (f *MftFrame) GetFrameNo() uint16 {
	return f.header.frameNo
}

func (f *MftFrame) GetFrameType() MftFrameType {
	return f.header.frameType
}

func (f *MftFrame) GetPayload() []byte {
	return f.body.bodyFrame
}

func BuildMftStartFrame(frameNo uint16) *MftFrame {
	emptyBody := make([]byte, 1)
	return &MftFrame{header: buildMftHeader(frameNo, MftFrameType_START), body: buildMftBody(emptyBody), footer: buildMftFooter()}
}

func BuildMftEndFrame(frameNo uint16) *MftFrame {
	emptyBody := make([]byte, 1)
	return &MftFrame{header: buildMftHeader(frameNo, MftFrameType_END), body: buildMftBody(emptyBody), footer: buildMftFooter()}
}

func BuildMftFrame(frameNo uint16, frameBody []byte) (*MftFrame, error) {
	if frameBody == nil {
		return nil, errors.New("body nil not acceptable")
	} else if len(frameBody) > maxMftBodySizeByte {
		return nil, errors.New("body exceeds max size limit")
	} else if len(frameBody) < 1 {
		return nil, errors.New("body exceeds min size limit")
	}
	return &MftFrame{header: buildMftHeader(frameNo, MftFrameType_TRANSMISSION), body: buildMftBody(frameBody), footer: buildMftFooter()}, nil
}

func DecodeMftFrame(frame []byte) (*MftFrame, error) {
	if frame == nil {
		return nil, errors.New("frame nil")
	} else if len(frame) > maxMftFrameSizeByte {
		return nil, errors.New(fmt.Sprintf("frame size is not recognized: %d (max: %d)", len(frame), maxMftFrameSizeByte))
	} else if len(frame) < minMftFrameSizeByte {
		return nil, errors.New(fmt.Sprintf("frame size is not recognized %d (min: %d)", len(frame), minMftFrameSizeByte))
	}
	header, errHeader := decodeMftHeader(frame[:mftHeaderSizeByte])
	if errHeader != nil {
		return nil, errHeader
	}
	footer, errFooter := decodeMftFooter(frame[len(frame)-mftFooterSizeByte:])
	if errFooter != nil {
		return nil, errHeader
	}
	body := frame[mftHeaderSizeByte : len(frame)-mftFooterSizeByte]
	return &MftFrame{header: *header, body: MftBody{bodyFrame: body}, footer: *footer}, nil
}

func decodeMftHeader(b []byte) (*MftHeader, error) {
	topHeaderConst := getTopHeaderConst()
	if len(b) != mftHeaderSizeByte {
		return nil, errors.New("wrong header size")
	}
	for i := range topHeaderConst {
		if topHeaderConst[i] != b[i] {
			return nil, errors.New("invalid header")
		}
	}
	frameType := MftFrameType(b[len(topHeaderConst)])
	if frameType != MftFrameType_START && frameType != MftFrameType_TRANSMISSION && frameType != MftFrameType_END {
		return nil, errors.New("invalid frame type")
	}
	frameNo := binary.LittleEndian.Uint16(b[len(topHeaderConst)+1:])
	return &MftHeader{
		top:       topHeaderConst,
		frameType: frameType,
		frameNo:   frameNo,
	}, nil
}

func decodeMftFooter(b []byte) (*MftFooter, error) {
	endFooterConst := getEndFooterConst()
	if len(b) != mftFooterSizeByte {
		return nil, errors.New("wrong footer size")
	}
	for i := range endFooterConst {
		if endFooterConst[i] != b[i] {
			return nil, errors.New("invalid footer")
		}
	}
	return &MftFooter{
		end: endFooterConst,
	}, nil
}
