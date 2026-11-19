package kadmin

import (
	"kfee/serdes"
	"strings"
)

// matchesRecord reports whether the record satisfies the filter. The search term and the value
// filter match the original value or the value transformed by the transformation of the rule
// that decoded the record. The key filter applies to the key only.
func (ka *SaramaKafkaAdmin) matchesRecord(f *Filter, topic, key string, data serdes.DesData, headers []Header) bool {
	if f == nil {
		return true
	}
	if !f.KeyFilter.matches(key, f.KeySearchTerm) {
		return false
	}
	if f.Matches(key, data.Value, headers) {
		return true
	}
	// the original value does not satisfy the filter, the transformed one may
	transformed, ok := ka.transformed(topic, data)
	if !ok {
		return false
	}
	valueOK := f.ValueFilter.matches(data.Value, f.ValueSearchTerm) ||
		f.ValueFilter.matches(transformed, f.ValueSearchTerm)
	if !valueOK {
		return false
	}
	if f.Term == "" {
		return true
	}
	term := strings.ToLower(f.Term)
	return containsTerm(term, key, data.Value, headers) || strings.Contains(strings.ToLower(transformed), term)
}

// transformed returns the transformed value of the record, false when its rule has no transformation.
func (ka *SaramaKafkaAdmin) transformed(topic string, data serdes.DesData) (string, bool) {
	if ka.transformer == nil || data.RuleID == "" {
		return "", false
	}
	out, ok, err := ka.transformer.Transform(topic, data.RuleID, data.Value)
	return out, ok && err == nil
}
