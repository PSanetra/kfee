package kontext

import (
	"kfee/config"
	"kfee/serdes"
	"kfee/transformation"
)

type ProgramKtx struct {
	config *config.Config
	// protoDecoders are the protobuf decoders of the active cluster
	protoDecoders *serdes.ProtobufDecoders
	// transformer applies the saved transformations of the active cluster
	transformer *transformation.Transformer
	cmdLineFlags
	WindowWidth     int
	WindowHeight    int
	AvailableHeight int
}

type cmdLineFlags struct {
	disableNerdFonts *bool
}

func (k *ProgramKtx) HeightUsed(height int) {
	if k.AvailableHeight < height {
		k.AvailableHeight -= k.AvailableHeight
	} else {
		k.AvailableHeight -= height
	}
}

func (k *ProgramKtx) AvailableTableHeight() int {
	// 2 for top and bottom border + 1 for top extra padding
	return k.AvailableHeight - 3
}

func (k *ProgramKtx) Config() *config.Config {
	return k.config
}

// ProtoDecoders returns the protobuf decoders of the active cluster, never nil.
func (k *ProgramKtx) ProtoDecoders() *serdes.ProtobufDecoders {
	if k.protoDecoders == nil {
		k.protoDecoders = serdes.NewProtobufDecoders(nil)
	}
	return k.protoDecoders
}

func (k *ProgramKtx) SetProtoDecoders(decoders *serdes.ProtobufDecoders) {
	k.protoDecoders = decoders
}

// Transformer returns the transformations of the active cluster, nil when there are none.
func (k *ProgramKtx) Transformer() *transformation.Transformer {
	return k.transformer
}

func (k *ProgramKtx) SetTransformer(transformer *transformation.Transformer) {
	k.transformer = transformer
}

func (k *ProgramKtx) RegisterConfig(c *config.Config) {
	k.config = c
	if k.disableNerdFonts != nil {
		k.config.PlainFonts = *k.disableNerdFonts
	}
}

func New(disableNerdFonts *bool) *ProgramKtx {
	return &ProgramKtx{
		cmdLineFlags: cmdLineFlags{
			disableNerdFonts: disableNerdFonts,
		},
	}
}

func WithNewAvailableDimensions(ktx *ProgramKtx) *ProgramKtx {
	ktx.AvailableHeight = ktx.WindowHeight
	return ktx
}
