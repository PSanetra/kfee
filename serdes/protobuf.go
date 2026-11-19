package serdes

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"kfee/config"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Header is a record header as seen by deserializers.
type Header struct {
	Key   string
	Value []byte
}

// ProtoSchema holds the compiled .proto files of a directory.
type ProtoSchema struct {
	Dir   string
	files *protoregistry.Files
	types *dynamicpb.Types
	// MessageTypes are the fully qualified names of all messages declared in the directory, sorted.
	MessageTypes []string
}

// ProtoCompiler compiles all .proto files of dir using the given protoc binary.
type ProtoCompiler func(protoc, dir string) (*ProtoSchema, error)

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// CompileProtoDir compiles all .proto files in dir and its subdirectories with protoc,
// using dir as import path.
func CompileProtoDir(protoc, dir string) (*ProtoSchema, error) {
	absDir, err := filepath.Abs(ExpandHome(dir))
	if err != nil {
		return nil, err
	}

	protoFiles, err := findProtoFiles(absDir)
	if err != nil {
		return nil, err
	}
	if len(protoFiles) == 0 {
		return nil, fmt.Errorf("no .proto files found in %s", absDir)
	}

	descFile, err := os.CreateTemp("", "kfee-*.binpb")
	if err != nil {
		return nil, err
	}
	_ = descFile.Close()
	defer os.Remove(descFile.Name())

	args := append([]string{
		"--proto_path=.",
		"--include_imports",
		"--descriptor_set_out=" + descFile.Name(),
	}, protoFiles...)
	cmd := exec.Command(protoc, args...)
	cmd.Dir = absDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("protoc (%s) not found, install protoc or set protobuf.protoc in the config", protoc)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("protoc failed: %s", msg)
		}
		return nil, fmt.Errorf("protoc failed: %w", err)
	}

	data, err := os.ReadFile(descFile.Name())
	if err != nil {
		return nil, err
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("unable to read descriptor set: %w", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, fmt.Errorf("unable to load descriptor set: %w", err)
	}

	var messageTypes []string
	for _, name := range protoFiles {
		fd, err := files.FindFileByPath(name)
		if err != nil {
			return nil, err
		}
		messageTypes = appendMessageTypes(messageTypes, fd.Messages())
	}
	sort.Strings(messageTypes)

	return &ProtoSchema{
		Dir:          absDir,
		files:        files,
		types:        dynamicpb.NewTypes(files),
		MessageTypes: messageTypes,
	}, nil
}

// findProtoFiles returns the paths, relative to dir, of all .proto files in dir, skipping hidden directories.
func findProtoFiles(dir string) ([]string, error) {
	var protoFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".proto" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		protoFiles = append(protoFiles, filepath.ToSlash(rel))
		return nil
	})
	return protoFiles, err
}

func appendMessageTypes(types []string, messages protoreflect.MessageDescriptors) []string {
	for i := 0; i < messages.Len(); i++ {
		md := messages.Get(i)
		if md.IsMapEntry() {
			continue
		}
		types = append(types, string(md.FullName()))
		types = appendMessageTypes(types, md.Messages())
	}
	return types
}

// Decode decodes data as the given message type and returns it as JSON.
func (s *ProtoSchema) Decode(messageType string, data []byte) (DesData, error) {
	desc, err := s.files.FindDescriptorByName(protoreflect.FullName(messageType))
	if err != nil {
		return DesData{}, fmt.Errorf("protobuf message type %s not found in %s", messageType, s.Dir)
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return DesData{}, fmt.Errorf("%s is not a protobuf message type", messageType)
	}

	msg := dynamicpb.NewMessage(md)
	if err := (proto.UnmarshalOptions{Resolver: s.types}).Unmarshal(data, msg); err != nil {
		return DesData{}, fmt.Errorf("unable to decode record as %s: %w", messageType, err)
	}
	jsonData, err := protojson.MarshalOptions{Resolver: s.types}.Marshal(msg)
	if err != nil {
		return DesData{}, fmt.Errorf("unable to convert %s to json: %w", messageType, err)
	}
	return DesData{Value: string(jsonData), MessageType: messageType}, nil
}

// ProtobufDecoders decodes the records of the topics having a protobuf decoder configured.
// It is safe for concurrent use.
type ProtobufDecoders struct {
	mu      sync.RWMutex
	protoc  string
	compile ProtoCompiler
	topics  map[string]*protoTopic
	version atomic.Uint64
}

type protoTopic struct {
	cfg   config.ProtobufTopicConfig
	rules []protoRule
	// err is set when the configuration is invalid
	err error

	mu        sync.Mutex
	compiled  bool
	schema    *ProtoSchema
	schemaErr error
}

type protoRule struct {
	config.ProtobufRule
	regex *regexp.Regexp
}

// NewProtobufDecoders creates the decoders for the given configuration, which may be nil.
// The .proto files of a topic are compiled when its first record is decoded.
func NewProtobufDecoders(cfg *config.ProtobufConfig) *ProtobufDecoders {
	return newProtobufDecoders(cfg, CompileProtoDir)
}

func newProtobufDecoders(cfg *config.ProtobufConfig, compile ProtoCompiler) *ProtobufDecoders {
	d := &ProtobufDecoders{
		protoc:  cfg.ProtocBinary(),
		compile: compile,
		topics:  map[string]*protoTopic{},
	}
	if cfg != nil {
		for _, t := range cfg.Topics {
			d.topics[t.Topic] = newProtoTopic(t)
		}
	}
	return d
}

func newProtoTopic(cfg config.ProtobufTopicConfig) *protoTopic {
	t := &protoTopic{cfg: cfg}
	for _, r := range cfg.Rules {
		rule := protoRule{ProtobufRule: r}
		if !r.IsFallback() {
			regex, err := regexp.Compile(r.ValueRegex)
			if err != nil {
				t.err = fmt.Errorf("invalid protobuf value regex for header %s of topic %s: %w", r.Header, cfg.Topic, err)
				break
			}
			rule.regex = regex
		}
		t.rules = append(t.rules, rule)
	}
	return t
}

// Protoc returns the protoc binary used to compile .proto files.
func (d *ProtobufDecoders) Protoc() string {
	if d == nil {
		return "protoc"
	}
	return d.protoc
}

// Version changes whenever the decoder configuration changes.
func (d *ProtobufDecoders) Version() uint64 {
	if d == nil {
		return 0
	}
	return d.version.Load()
}

// SetTopic configures the decoder of a topic. When schema is not nil, it is used instead of
// compiling the topic's proto directory.
func (d *ProtobufDecoders) SetTopic(cfg config.ProtobufTopicConfig, schema *ProtoSchema) {
	t := newProtoTopic(cfg)
	if schema != nil {
		t.compiled = true
		t.schema = schema
	}
	d.mu.Lock()
	d.topics[cfg.Topic] = t
	d.mu.Unlock()
	d.version.Add(1)
}

// DeleteTopic removes the decoder of a topic.
func (d *ProtobufDecoders) DeleteTopic(topic string) {
	d.mu.Lock()
	delete(d.topics, topic)
	d.mu.Unlock()
	d.version.Add(1)
}

// Describe describes the message type of a schema rule of the topic for prompting a language model.
func (d *ProtobufDecoders) Describe(topic, messageType string) (string, error) {
	if d == nil {
		return "", errors.New("no protobuf decoders configured")
	}
	d.mu.RLock()
	t := d.topics[topic]
	d.mu.RUnlock()
	if t == nil {
		return "", fmt.Errorf("no protobuf decoder configured for %s", topic)
	}
	schema, err := t.compiledSchema(d.compile, d.protoc)
	if err != nil {
		return "", err
	}
	return schema.Describe(messageType)
}

// Decode decodes the record when a protobuf rule of its topic matches it,
// handled reports whether that was the case.
func (d *ProtobufDecoders) Decode(topic string, headers []Header, value []byte) (data DesData, handled bool, err error) {
	if d == nil {
		return DesData{}, false, nil
	}
	d.mu.RLock()
	t := d.topics[topic]
	d.mu.RUnlock()
	if t == nil {
		return DesData{}, false, nil
	}
	if t.err != nil {
		return DesData{}, true, t.err
	}

	rule, ok := t.match(headers)
	if !ok {
		return DesData{}, false, nil
	}
	if len(value) == 0 {
		return DesData{}, true, nil
	}

	if rule.Raw {
		data, err = DecodeRaw(value)
		data.RuleID = rule.ID
		return data, true, err
	}

	schema, err := t.compiledSchema(d.compile, d.protoc)
	if err != nil {
		return DesData{}, true, err
	}
	data, err = schema.Decode(rule.MessageType, value)
	data.RuleID = rule.ID
	return data, true, err
}

func (t *protoTopic) match(headers []Header) (protoRule, bool) {
	for _, r := range t.rules {
		if r.IsFallback() {
			return r, true
		}
		for _, h := range headers {
			if h.Key == r.Header && r.regex.Match(h.Value) {
				return r, true
			}
		}
	}
	return protoRule{}, false
}

func (t *protoTopic) compiledSchema(compile ProtoCompiler, protoc string) (*ProtoSchema, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.compiled {
		t.schema, t.schemaErr = compile(protoc, t.cfg.ProtoDir)
		t.compiled = true
	}
	return t.schema, t.schemaErr
}
