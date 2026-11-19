package kadmin

import (
	"kfee/serdes"
	"kfee/sradmin"
)

type SraSetter interface {
	SetSra(sra sradmin.Client)
}

func (ka *SaramaKafkaAdmin) SetSra(sra sradmin.Client) {
	ka.sra = sra
}

type ProtoDecodersSetter interface {
	SetProtoDecoders(decoders *serdes.ProtobufDecoders)
}

func (ka *SaramaKafkaAdmin) SetProtoDecoders(decoders *serdes.ProtobufDecoders) {
	ka.protoDecoders = decoders
}

// Transformer applies the transformation saved for a decoder rule to a decoded value.
type Transformer interface {
	// Transform returns the transformed value and whether the rule has a transformation.
	Transform(topic, ruleID, value string) (string, bool, error)
}

type TransformerSetter interface {
	SetTransformer(transformer Transformer)
}

func (ka *SaramaKafkaAdmin) SetTransformer(transformer Transformer) {
	ka.transformer = transformer
}
