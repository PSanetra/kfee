package kadmin

import (
	"slices"
)

// avroDecoderID is the decoder of an Avro record: the subject of its schema, so the transformations
// of the subject apply to all records of its schema. Empty when the subject cannot be found.
func (ka *SaramaKafkaAdmin) avroDecoderID(topic string, schemaID int) string {
	if ka.sra == nil {
		return ""
	}
	subjects, err := ka.sra.SubjectsOfSchema(schemaID)
	if err != nil || len(subjects) == 0 {
		return ""
	}
	return "avro:" + pickSubject(subjects, topic)
}

// pickSubject chooses the subject of a schema for the topic: the one following the topic name
// strategy, the value subject before the key subject, or the first one.
func pickSubject(subjects []string, topic string) string {
	for _, preferred := range []string{topic + "-value", topic + "-key"} {
		if slices.Contains(subjects, preferred) {
			return preferred
		}
	}
	return subjects[0]
}
