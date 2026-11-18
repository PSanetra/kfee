package kadmin

import (
	"ktea/serdes"
	"ktea/sradmin"
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
