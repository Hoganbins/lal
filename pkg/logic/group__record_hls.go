// Copyright 2022, Chef.  All rights reserved.
// https://github.com/q191201771/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package logic

import "github.com/q191201771/lal/pkg/hls"

func (group *Group) IsHlsMuxerAlive() bool {
	group.mutex.Lock()
	defer group.mutex.Unlock()
	return group.hlsMuxer != nil
}

// startHlsIfNeeded 必要时启动hls
//
func (group *Group) startHlsIfNeeded() {
	// TODO(chef): [refactor] ts依赖hls
	if !group.config.HlsConfig.Enable {
		return
	}
	if group.hlsMuxer != nil {
		Log.Errorf("[%s] hls muxer exist while addIn. muxer=%+v", group.UniqueKey, group.hlsMuxer)
	}
	enable := group.config.HlsConfig.Enable || group.config.HlsConfig.EnableHttps
	group.hlsMuxer = hls.NewMuxer(group.streamName, enable, &group.config.HlsConfig.MuxerConfig, group)
	group.hlsMuxer.Start()
}

func (group *Group) stopHlsIfNeeded() {
	if !group.config.HlsConfig.Enable {
		return
	}
	if group.hlsMuxer != nil {
		group.hlsMuxer.Dispose()
		group.observer.CleanupHlsIfNeeded(group.appName, group.streamName, group.hlsMuxer.OutPath())
		group.hlsMuxer = nil
	}
}
