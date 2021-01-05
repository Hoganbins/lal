// Copyright 2020, Chef.  All rights reserved.
// https://github.com/q191201771/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package base

import "github.com/q191201771/naza/pkg/unique"

const (
	UKPRTMPServerSession        = "RTMPPUBSUB"
	UKPRTMPPushSession          = "RTMPPUSH"
	UKPRTMPPullSession          = "RTMPPULL"
	UKPRTSPServerCommandSession = "RTSPSRVCMD"
	UKPRTSPPubSession           = "RTSPPUB"
	UKPRTSPSubSession           = "RTSPSUB"
	UKPRTSPPushSession          = "RTSPPUSH"
	UKPRTSPPullSession          = "RTSPPULL"
	UKPFLVSubSession            = "FLVSUB"
	UKPTSSubSession             = "TSSUB"
	UKPFLVPullSession           = "FLVPULL"

	UKPGroup    = "GROUP"
	UKPHLSMuxer = "HLSMUXER"
	UKPStreamer = "STREAMER"
)

func GenUniqueKey(prefix string) string {
	return unique.GenUniqueKey(prefix)
}
