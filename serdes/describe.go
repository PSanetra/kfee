package serdes

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Describe returns the message type with all message and enum types it uses, in a compact
// .proto-like syntax, for prompting a language model.
func (s *ProtoSchema) Describe(messageType string) (string, error) {
	desc, err := s.files.FindDescriptorByName(protoreflect.FullName(messageType))
	if err != nil {
		return "", fmt.Errorf("protobuf message type %s not found in %s", messageType, s.Dir)
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return "", fmt.Errorf("%s is not a protobuf message type", messageType)
	}

	var b strings.Builder
	seen := map[protoreflect.FullName]bool{}
	var describe func(d protoreflect.Descriptor)
	describe = func(d protoreflect.Descriptor) {
		if seen[d.FullName()] {
			return
		}
		seen[d.FullName()] = true
		switch d := d.(type) {
		case protoreflect.EnumDescriptor:
			fmt.Fprintf(&b, "enum %s {\n", d.FullName())
			values := d.Values()
			for i := 0; i < values.Len(); i++ {
				fmt.Fprintf(&b, "  %s = %d;\n", values.Get(i).Name(), values.Get(i).Number())
			}
			b.WriteString("}\n\n")
		case protoreflect.MessageDescriptor:
			fmt.Fprintf(&b, "message %s {\n", d.FullName())
			fields := d.Fields()
			var used []protoreflect.Descriptor
			for i := 0; i < fields.Len(); i++ {
				f := fields.Get(i)
				if f.IsMap() {
					fmt.Fprintf(&b, "  map<%s, %s> %s = %d;\n", f.MapKey().Kind(), mapValueType(f.MapValue(), &used), f.Name(), f.Number())
					continue
				}
				typeName := f.Kind().String()
				switch f.Kind() {
				case protoreflect.EnumKind:
					typeName = string(f.Enum().FullName())
					used = append(used, f.Enum())
				case protoreflect.MessageKind, protoreflect.GroupKind:
					typeName = string(f.Message().FullName())
					used = append(used, f.Message())
				}
				if f.IsList() {
					typeName = "repeated " + typeName
				}
				fmt.Fprintf(&b, "  %s %s = %d;\n", typeName, f.Name(), f.Number())
			}
			b.WriteString("}\n\n")
			for _, u := range used {
				describe(u)
			}
		}
	}
	describe(md)
	return b.String(), nil
}

// mapValueType is the type name of a map value, message and enum types are collected in used.
func mapValueType(v protoreflect.FieldDescriptor, used *[]protoreflect.Descriptor) string {
	switch v.Kind() {
	case protoreflect.EnumKind:
		*used = append(*used, v.Enum())
		return string(v.Enum().FullName())
	case protoreflect.MessageKind, protoreflect.GroupKind:
		*used = append(*used, v.Message())
		return string(v.Message().FullName())
	}
	return v.Kind().String()
}
