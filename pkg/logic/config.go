// Copyright 2019, Chef.  All rights reserved.
// https://github.com/q191201771/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package logic

import "github.com/q191201771/lal/pkg/hls"

type Config struct {
	RTMP    RTMP    `json:"rtmp"`
	HTTPFLV HTTPFLV `json:"httpflv"`
	HLS     HLS     `json:"hls"`
}

type RTMP struct {
	Addr   string `json:"addr"`
	GOPNum int    `json:"gop_num"`
}

type HTTPFLV struct {
	SubListenAddr string `json:"sub_listen_addr"`
	GOPNum        int    `json:"gop_num"`
}

type HLS struct {
	SubListenAddr string `json:"sub_listen_addr"`
	*hls.MuxerConfig
}
