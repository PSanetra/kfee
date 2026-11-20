package config

// defaultTransformationHistory is the number of applied transformations kept per decoder rule.
const defaultTransformationHistory = 10

// LLMConfig configures the command that generates transformations from a description.
type LLMConfig struct {
	// Command runs without a shell, e.g. ["claude", "-p"] or ["ollama", "run", "llama3"].
	Command []string `yaml:"command,omitempty"`
	// PromptAsArgument passes the prompt as the last argument instead of writing it to stdin,
	// for tools that do not read a prompt from stdin.
	PromptAsArgument bool `yaml:"promptAsArgument,omitempty"`
	// SendSample includes a decoded record of the topic in the prompt. Off by default, as
	// records may contain sensitive data.
	SendSample bool `yaml:"sendSample,omitempty"`
}

// LLMPreset is a known LLM tool with the command running it non-interactively.
type LLMPreset struct {
	Name   string
	Config LLMConfig
}

// LLMPresets are the LLM tools offered on first use.
var LLMPresets = []LLMPreset{
	{Name: "Claude Code", Config: LLMConfig{Command: []string{"claude", "-p"}}},
	{Name: "pi", Config: LLMConfig{Command: []string{"pi", "--print"}}},
	{Name: "opencode", Config: LLMConfig{Command: []string{"opencode", "run"}, PromptAsArgument: true}},
}

// Configured reports whether a command is set.
func (l *LLMConfig) Configured() bool {
	return l != nil && len(l.Command) > 0
}

// SetLLMCommand stores the command generating transformations, keeping the other LLM settings.
func (c *Config) SetLLMCommand(command []string, promptAsArgument bool) {
	llm := LLMConfig{Command: command, PromptAsArgument: promptAsArgument}
	if c.LLM != nil {
		llm.SendSample = c.LLM.SendSample
	}
	c.LLM = &llm
	c.flush()
}

// TransformationHistoryLimit returns how many applied transformations are kept per decoder rule.
func (c *Config) TransformationHistoryLimit() int {
	if c.TransformationHistory <= 0 {
		return defaultTransformationHistory
	}
	return c.TransformationHistory
}
