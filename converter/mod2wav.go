package converter

import "protracker-go/mod"

type ChannelNum int

const (
	Mono ChannelNum = iota + 1
	Stereo
)

type Mod2Wav struct {
	numberOfChannels ChannelNum // 1 - mono, 2 stereo
	stereoSeparation int        // Only relevant when rendering to stereo. stereo separation in percentage, 0 - 100, 0 - channel fully mixed, 100 means channels fully separated.
}

func NewMod2Wav(chNum ChannelNum, stereoSep int) *Mod2Wav {
	return &Mod2Wav{
		numberOfChannels: chNum,
		stereoSeparation: stereoSep,
	}
}

func (m *Mod2Wav) Convert(mod *mod.PTModule) ([]byte, error) {
	panic("implement me")
}
