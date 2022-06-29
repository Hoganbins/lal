// Copyright 2022, Chef.  All rights reserved.
// https://github.com/q191201771/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package gb28181

import (
	"encoding/hex"
	"github.com/q191201771/lal/pkg/h2645"
	"github.com/q191201771/lal/pkg/rtprtcp"

	"github.com/q191201771/lal/pkg/hevc"

	"github.com/q191201771/lal/pkg/avc"

	"github.com/q191201771/naza/pkg/bele"
	"github.com/q191201771/naza/pkg/nazabits"
	"github.com/q191201771/naza/pkg/nazabytes"
	"github.com/q191201771/naza/pkg/nazalog"
)

// TODO(chef): gb28181 package处于开发中阶段，请不使用
// TODO(chef): [opt] rtp排序 202206
// TODO(chef): [test] 保存rtp数据，用于回放分析 202206

type onAudioFn func(payload []byte, dts int64, pts int64)
type onVideoFn func(payload []byte, dts int64, pts int64)

type IPsUnpackerObserver interface {
	OnAudio(payload []byte, dts int64, pts int64)

	// OnVideo
	//
	// @param payload: annexb格式
	//
	OnVideo(payload []byte, dts int64, pts int64)
}

// PsUnpacker 解析ps(Progream Stream)流
//
type PsUnpacker struct {
	buf *nazabytes.Buffer

	videoStreamType byte
	audioStreamType byte

	preVideoPts int64
	preAudioPts int64
	preVideoDts int64
	preAudioDts int64

	preVideoRtpts int64
	preAudioRtpts int64

	videoBuf []byte
	audioBuf []byte

	onAudio onAudioFn
	onVideo onVideoFn
}

func NewPsUnpacker() *PsUnpacker {
	return &PsUnpacker{
		buf:           nazabytes.NewBuffer(psBufInitSize),
		preVideoPts:   -1,
		preAudioPts:   -1,
		preVideoRtpts: -1,
		preAudioRtpts: -1,
		onAudio:       defaultOnAudio,
		onVideo:       defaultOnVideo,
	}
}

// WithOnAudio
//
// TODO(chef): [refactor] 使用AvPacket 202206
//
func (p *PsUnpacker) WithOnAudio(onAudio func(payload []byte, dts int64, pts int64)) *PsUnpacker {
	p.onAudio = onAudio
	return p
}

func (p *PsUnpacker) WithOnVideo(onVideo func(payload []byte, dts int64, pts int64)) *PsUnpacker {
	p.onVideo = onVideo
	return p
}

func (p *PsUnpacker) VideoStreamType() byte {
	return p.videoStreamType
}

func (p *PsUnpacker) AudioStreamType() byte {
	return p.audioStreamType
}

// FeedRtpPacket
//
// @param rtpPacket: rtp包，注意，包含rtp包头部分
//
func (p *PsUnpacker) FeedRtpPacket(rtpPacket []byte) error {
	nazalog.Debugf("> FeedRtpPacket. len=%d", len(rtpPacket))

	h, err := rtprtcp.ParseRtpHeader(rtpPacket)
	if err != nil {
		return err
	}

	p.FeedRtpBody(rtpPacket[rtprtcp.RtpFixedHeaderLength:], h.Timestamp)
	return nil
}

// FeedRtpBody
//
// 注意，pes 没有pts时使用外部传入，rtpBody 需要传入两个ps header之间的数据
//
func (p *PsUnpacker) FeedRtpBody(rtpBody []byte, rtpts uint32) {
	//p.buf.Reset()
	p.buf.Write(rtpBody)
	// ISO/IEC iso13818-1
	//
	// 2.5 Program Stream bitstream requirements
	//
	// 2.5.3.5 System header
	// Table 2-32 - Program Stream system header
	//
	// 2.5.4 Program Stream map
	// Table 2-35 - Program Stream map
	//
	// TODO(chef): [fix] 有些没做有效长度判断
	for p.buf.Len() != 0 {
		rb := p.buf.Bytes()
		i := 0
		code := bele.BeUint32(rb[i:])
		i += 4
		switch code {
		case psPackStartCodePackHeader:
			nazalog.Debugf("----------pack header----------")
			consumed := parsePackHeader(rb, i)
			if consumed < 0 {
				return
			}
			p.buf.Skip(i + consumed)
		case psPackStartCodeSystemHeader:
			nazalog.Debugf("----------system header----------")
			p.skipPackStreamBody(rb, i)
		case psPackStartCodeProgramStreamMap:
			nazalog.Debugf("----------program stream map----------")
			p.parsePsm(rb, i)
		case psPackStartCodeAudioStream:
			nazalog.Debugf("----------audio stream----------")
			consumed := p.parseAvStream(int(code), rtpts, rb, i)
			if consumed < 0 {
				return
			}
			p.buf.Skip(i + consumed)
		case psPackStartCodeVideoStream:
			nazalog.Debugf("----------video stream----------")
			consumed := p.parseAvStream(int(code), rtpts, rb, i)
			if consumed < 0 {
				return
			}
			p.buf.Skip(i + consumed)
		case psPackStartCodePesPrivate2:
			fallthrough
		case psPackStartCodePesEcm:
			fallthrough
		case psPackStartCodePesEmm:
			fallthrough
		case psPackStartCodePesPadding:
			fallthrough
		case psPackStartCodePackEnd:
			nazalog.Errorf("skip. %s", hex.Dump(nazabytes.Prefix(rb[i-4:], 32)))
			p.buf.Skip(i)
		case psPackStartCodeHikStream:
			fallthrough
		case psPackStartCodePesPsd:
			fallthrough
		default:
			nazalog.Errorf("default. %s", hex.Dump(nazabytes.Prefix(rb[i-4:], 32)))
			p.skipPackStreamBody(rb, i)
			return

		}
	}
}
func (p *PsUnpacker) skipPackStreamBody(rb []byte, index int) {
	consumed := parsePackStreamBody(rb, index)
	if consumed != -1 {
		p.buf.Skip(index + consumed)
	}
}

func (p *PsUnpacker) parsePsm(rb []byte, index int) {
	i := index

	if len(rb[i:]) < 6 {
		return
	}

	// skip program_stream_map_length
	// skip current_next_indicator
	// skip reserved
	// skip program_stream_map_version
	// skip reverved
	// skip marked_bit
	i += 4 // 2 + 1 + 1

	// program_stream_info_length
	l := int(bele.BeUint16(rb[i:]))
	i += 2

	if len(rb[i:]) < l+2 {
		return
	}

	i += l

	// elementary_stream_map_length
	esml := int(bele.BeUint16(rb[i:]))
	nazalog.Debugf("l=%d, esml=%d", l, esml)
	i += 2

	if len(rb[i:]) < esml+4 {
		return
	}

	for esml > 0 {
		streamType := rb[i]
		i += 1
		streamId := rb[i]
		i += 1
		// elementary_stream_info_length
		if streamId >= 0xe0 && streamId <= 0xef {
			p.videoStreamType = streamType
		} else if streamId >= 0xc0 && streamId <= 0xdf {
			p.audioStreamType = streamType
		}
		esil := int(bele.BeUint16(rb[i:]))
		nazalog.Debugf("streamType=%d, streamId=%d, esil=%d", streamType, streamId, esil)
		i += 2 + esil
		esml = esml - 4 - esil
	}
	// skip
	i += 4
	nazalog.Debugf("CHEFERASEME i=%d, esml=%d, %s", i, esml, hex.Dump(nazabytes.Prefix(p.buf.Bytes(), 128)))
	p.buf.Skip(i)
}

func (p *PsUnpacker) parseAvStream(code int, rtpts uint32, rb []byte, index int) int {
	i := index

	// 注意，由于length是两字节，所以存在一个帧分成多个pes包的情况
	length := int(bele.BeUint16(rb[i:]))
	i += 2

	nazalog.Debugf("parseAvStream. code=%d, expected=%d, actual=%d", code, length, len(rb)-i)

	if len(rb)-i < length {
		//if code == psPackStartCodeAudioStream {
		//	p.audioBuf = append(p.audioBuf, rb[i:]...)
		//} else {
		//	p.videoBuf = append(p.videoBuf, rb[i:]...)
		//}
		return -1
	}

	ptsDtsFlag := rb[i+1] >> 6
	phdl := int(rb[i+2]) // pes header data length
	i += 3

	var pts int64 = -1
	var dts int64 = -1
	j := 0
	if ptsDtsFlag&0x2 != 0 {
		_, pts = readPts(rb[i:])
		j += 5
	}
	if ptsDtsFlag&0x1 != 0 {
		_, dts = readPts(rb[i+j:])
	} else {
		dts = pts
	}

	i += phdl

	nazalog.Debugf("parseAvStream. code=%d, length=%d, pts=%d, dts=%d", code, length, pts, dts)

	if code == psPackStartCodeAudioStream {
		// 注意，处理音频的逻辑和处理视频的类似，参考处理视频的注释
		if p.audioStreamType == StreamTypeAAC {
			nazalog.Debugf("audio code=%d, length=%d, ptsDtsFlag=%d, phdl=%d, pts=%d, dts=%d,type=%d", code, length, ptsDtsFlag, phdl, pts, dts, p.audioStreamType)
			if pts == -1 {
				if p.preAudioPts == -1 {
					if p.preAudioRtpts == -1 {
						// noop
					} else {
						if p.preAudioRtpts != int64(rtpts) {
							p.onAudio(p.audioBuf, p.preAudioDts, p.preAudioPts)
							p.audioBuf = nil
						} else {
							// noop
						}
					}
				} else {
					pts = p.preAudioPts
				}
			} else {
				if pts != p.preAudioPts && p.preAudioPts >= 0 {
					p.onAudio(p.audioBuf, p.preAudioDts, p.preAudioPts)
					p.audioBuf = nil
				} else {
					// noop
				}
			}
			p.audioBuf = append(p.audioBuf, rb[i:i+length-3-phdl]...)
			p.preAudioRtpts = int64(rtpts)
			p.preAudioPts = pts
			p.preAudioDts = dts
		} else {
			nazalog.Errorf("unknown audio stream type. ast=%d", p.audioStreamType)
		}
	} else if code == psPackStartCodeVideoStream {
		// 判断出当前pes是否是新的帧，然后将缓存中的帧回调给上层

		if pts == -1 {
			// 当前pes包没有pts

			if p.preVideoPts == -1 {
				if p.preVideoRtpts == -1 {
					// 整个流的第一帧，啥也不干
				} else {
					// 整个流没有pts字段，退化成使用rtpts

					if p.preVideoRtpts != int64(rtpts) {
						// 使用rtp的时间戳回调，但是，并不用rtp的时间戳更新pts
						p.iterateNaluByStartCode(code, p.preVideoRtpts, p.preVideoRtpts)
						p.videoBuf = nil
					} else {
						// 同一帧，啥也不干
					}
				}
			} else {
				// 当前pes包没有pts，而前一个有
				// 这种情况我们认为是同一帧，啥也不干
				pts = p.preVideoPts
			}
		} else {
			// 当前pes包有pts

			if pts != p.preVideoPts && p.preVideoPts >= 0 {
				// 当前pes包是新的帧，将缓存中的帧回调给上层

				p.iterateNaluByStartCode(code, p.preVideoPts, p.preVideoPts)
				p.videoBuf = nil
			} else {
				// 两种情况：
				// 1. pts != prev && prev == -1 也即第一帧
				// 2. pts == prev && prev != -1 也即同一帧（前后两个pes包的时间戳相同）
				// 这两种情况，都啥也不干
			}
		}

		// 注意，是处理完之前的数据后，再将当前pes包存入缓存中
		p.videoBuf = append(p.videoBuf, rb[i:i+length-3-phdl]...)
		p.preVideoRtpts = int64(rtpts)
		p.preVideoPts = pts
		p.preVideoDts = dts
	}

	return 2 + length
}

// parsePackHeader 注意，`rb[index:]`为待解析的内存块
//
func parsePackHeader(rb []byte, index int) int {
	// 2.5.3.3 Pack layer of Program Stream
	// Table 2-33 - Program Stream pack header

	i := index
	// TODO(chef): 这里按MPEG-2处理，还需要处理MPEG-1 202206

	// skip system clock reference(SCR)
	// skip PES program mux rate
	i += 6 + 3
	if len(rb) <= i {
		nazalog.Warnf("[p] expected=%d, actual=%d", i, len(rb))
		return -1
	}

	// skip stuffing
	l := int(rb[i] & 0x7)
	i += 1 + l

	return i - index
}

func parsePackStreamBody(rb []byte, index int) int {
	i := index

	if len(rb) < i+2 {
		return -1
	}
	l := int(bele.BeUint16(rb[i:]))
	i += 2 + l
	if len(rb) < i {
		return -1
	}

	return i - index
}

// iterateNaluByStartCode 通过nal start code分隔缓存数据，将nal回调给上层
//
func (p *PsUnpacker) iterateNaluByStartCode(code int, pts, dts int64) {
	leading, preLeading, startPos := 0, 0, 0
	startPos, preLeading = h2645.IterateNaluStartCode(p.videoBuf, 0)
	if startPos < 0 {
		nazalog.Errorf("CHEFNOTICEME %s", hex.Dump(nazabytes.Prefix(p.videoBuf, 32)))
		return
	}

	nextPos := startPos
	nalu := p.videoBuf[:0]
	for startPos >= 0 {
		nextPos, leading = h2645.IterateNaluStartCode(p.videoBuf, startPos+preLeading)
		// 找到下一个，则取两个start code之间的内容
		// 找不到下一个，则取startcode到末尾的内容
		// 不管是否找到下一个，都回调
		if nextPos >= 0 {
			nalu = p.videoBuf[startPos:nextPos]
		} else {
			nalu = p.videoBuf[startPos:]
		}
		startPos = nextPos
		if p.videoStreamType == StreamTypeH265 {
			nazalog.Debugf("Video code=%d, length=%d,pts=%d, dts=%d, type=%s", code, len(nalu), pts, dts, hevc.ParseNaluTypeReadable(nalu[preLeading]))
		} else {
			nazalog.Debugf("Video code=%d, length=%d,pts=%d, dts=%d, type=%s", code, len(nalu), pts, dts, avc.ParseNaluTypeReadable(nalu[preLeading]))
		}
		p.onVideo(nalu, dts, pts)
		if nextPos >= 0 {
			preLeading = leading
		}
	}
}

// ---------------------------------------------------------------------------------------------------------------------

// TODO(chef): [refactor] 以下代码拷贝来自package mpegts，重复了

// Pes -----------------------------------------------------------
// <iso13818-1.pdf>
// <2.4.3.6 PES packet> <page 49/174>
// <Table E.1 - PES packet header example> <page 142/174>
// <F.0.2 PES packet> <page 144/174>
// packet_start_code_prefix  [24b] *** always 0x00, 0x00, 0x01
// stream_id                 [8b]  *
// PES_packet_length         [16b] **
// '10'                      [2b]
// PES_scrambling_control    [2b]
// PES_priority              [1b]
// data_alignment_indicator  [1b]
// copyright                 [1b]
// original_or_copy          [1b]  *
// PTS_DTS_flags             [2b]
// ESCR_flag                 [1b]
// ES_rate_flag              [1b]
// DSM_trick_mode_flag       [1b]
// additional_copy_info_flag [1b]
// PES_CRC_flag              [1b]
// PES_extension_flag        [1b]  *
// PES_header_data_length    [8b]  *
// -----------------------------------------------------------
type Pes struct {
	pscp       uint32
	sid        uint8
	ppl        uint16
	pad1       uint8
	ptsDtsFlag uint8
	pad2       uint8
	phdl       uint8
	pts        int64
	dts        int64
}

func ParsePes(b []byte) (pes Pes, length int) {
	br := nazabits.NewBitReader(b)
	//pes.pscp, _ = br.ReadBits32(24)
	//pes.sid, _ = br.ReadBits8(8)
	//pes.ppl, _ = br.ReadBits16(16)

	pes.pad1, _ = br.ReadBits8(8)
	pes.ptsDtsFlag, _ = br.ReadBits8(2)
	pes.pad2, _ = br.ReadBits8(6)
	pes.phdl, _ = br.ReadBits8(8)

	_, _ = br.ReadBytes(uint(pes.phdl))
	length = 9 + int(pes.phdl)

	// 处理得不是特别标准
	if pes.ptsDtsFlag&0x2 != 0 {
		_, pes.pts = readPts(b[9:])
	}
	if pes.ptsDtsFlag&0x1 != 0 {
		_, pes.dts = readPts(b[14:])
	} else {
		pes.dts = pes.pts
	}
	//pes.pts = (pes.pts - delay) / 90
	//pes.dts = (pes.dts - delay) / 90

	return
}

// read pts or dts
func readPts(b []byte) (fb uint8, pts int64) {
	fb = b[0] >> 4
	pts |= int64((b[0]>>1)&0x07) << 30
	pts |= (int64(b[1])<<8 | int64(b[2])) >> 1 << 15
	pts |= (int64(b[3])<<8 | int64(b[4])) >> 1
	return
}

func defaultOnAudio(payload []byte, dts int64, pts int64) {
	// noop
}
func defaultOnVideo(payload []byte, dts int64, pts int64) {
	// noop
}
