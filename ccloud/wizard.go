package ccloud

import (
	"errors"
	"fmt"
	"io"
	"kfee/config"
	"kfee/styles"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
)

// Wizard interactively adds a Confluent Cloud cluster to the kfee configuration.
type Wizard struct {
	cli   CLI
	cfg   *config.Config
	now   func() time.Time
	login func() error
	out   io.Writer
}

// Run starts the wizard using the confluent binary from the PATH.
func Run(cfg *config.Config) error {
	w := &Wizard{
		cli:   NewCLI(ExecRunner("confluent")),
		cfg:   cfg,
		now:   time.Now,
		login: func() error { return Login("confluent") },
		out:   os.Stdout,
	}
	s, err := w.choose()
	if err != nil {
		return err
	}
	return w.apply(s)
}

// setup is what the user chose in the wizard.
type setup struct {
	env      Environment
	cluster  KafkaCluster
	name     string
	color    string
	sr       *SchemaRegistry
	activate bool
	// description of the created API keys
	description string
}

var errAborted = errors.New("aborted, nothing was changed")

func (w *Wizard) choose() (setup, error) {
	var s setup

	envs, err := w.environments()
	if err != nil {
		return s, err
	}
	if s.env, err = choose("Environment", envs, func(e Environment) string {
		return fmt.Sprintf("%s (%s)", e.Name, e.ID)
	}, func(e Environment) bool { return e.IsCurrent }); err != nil {
		return s, err
	}

	fmt.Fprintf(w.out, "Listing Kafka clusters of %s …\n", s.env.Name)
	clusters, err := w.cli.KafkaClusters(s.env.ID)
	if err != nil {
		return s, err
	}
	if len(clusters) == 0 {
		return s, fmt.Errorf("environment %s has no Kafka clusters", s.env.Name)
	}
	if s.cluster, err = choose("Kafka cluster", clusters, func(c KafkaCluster) string {
		return fmt.Sprintf("%s (%s, %s %s)", c.Name, c.ID, c.Cloud, c.Region)
	}, func(c KafkaCluster) bool { return c.IsCurrent }); err != nil {
		return s, err
	}

	var useSR bool
	sr, srErr := w.cli.SchemaRegistry(s.env.ID)
	if srErr == nil {
		useSR = true
	}

	s.name = s.cluster.Name
	s.color = styles.ColorGreen
	s.activate = true
	fields := []huh.Field{
		huh.NewInput().
			Title("Name in kfee").
			Value(&s.name).
			Validate(func(name string) error {
				if strings.TrimSpace(name) == "" {
					return errors.New("name is required")
				}
				return nil
			}),
		huh.NewSelect[string]().
			Title("Color").
			Value(&s.color).
			Options(
				huh.NewOption("green", styles.ColorGreen),
				huh.NewOption("blue", styles.ColorBlue),
				huh.NewOption("orange", styles.ColorOrange),
				huh.NewOption("purple", styles.ColorPurple),
				huh.NewOption("yellow", styles.ColorYellow),
				huh.NewOption("red", styles.ColorRed),
			),
	}
	if srErr == nil {
		fields = append(fields, huh.NewConfirm().
			Title("Configure Schema Registry "+sr.ID+"?").
			Description("Creates a second API key").
			Value(&useSR))
	}
	fields = append(fields, huh.NewConfirm().
		Title("Make it the active cluster?").
		Value(&s.activate))
	// one question at a time, shift+tab goes back
	var groups []*huh.Group
	for _, f := range fields {
		groups = append(groups, huh.NewGroup(f))
	}
	if err := huh.NewForm(groups...).Run(); err != nil {
		return s, formErr(err)
	}
	s.name = strings.TrimSpace(s.name)
	if useSR {
		s.sr = &sr
	}
	s.description = KeyDescription(w.now())

	confirmed := true
	if err := huh.NewForm(huh.NewGroup(huh.NewConfirm().
		Title("Create API key(s) and save the cluster?").
		Description(w.summary(s)).
		Value(&confirmed))).Run(); err != nil {
		return s, formErr(err)
	}
	if !confirmed {
		return s, errAborted
	}
	return s, nil
}

// environments lists the environments and offers to log in when that fails.
func (w *Wizard) environments() ([]Environment, error) {
	fmt.Fprintln(w.out, "Listing Confluent Cloud environments …")
	envs, err := w.cli.Environments()
	if err != nil {
		fmt.Fprintln(w.out, err)
		login := true
		if err := huh.NewForm(huh.NewGroup(huh.NewConfirm().
			Title("Run confluent login now?").
			Value(&login))).Run(); err != nil {
			return nil, formErr(err)
		}
		if !login {
			return nil, errAborted
		}
		if err := w.login(); err != nil {
			return nil, fmt.Errorf("confluent login failed: %w", err)
		}
		if envs, err = w.cli.Environments(); err != nil {
			return nil, err
		}
	}
	if len(envs) == 0 {
		return nil, errors.New("no Confluent Cloud environments found")
	}
	return envs, nil
}

func (w *Wizard) summary(s setup) string {
	desc := s.description
	lines := []string{
		fmt.Sprintf("API key %q for Kafka cluster %s", desc, s.cluster.ID),
	}
	if s.sr != nil {
		lines = append(lines, fmt.Sprintf("API key %q for Schema Registry %s", desc, s.sr.ID))
	}
	if w.cfg.FindClusterByName(s.name) != nil {
		lines = append(lines, fmt.Sprintf("Updates the existing kfee cluster %q", s.name))
	} else {
		lines = append(lines, fmt.Sprintf("Adds the kfee cluster %q", s.name))
	}
	return strings.Join(lines, "\n")
}

// choose lets the user choose one of the items, unless there is only one.
func choose[T any](title string, items []T, label func(T) string, preselect func(T) bool) (T, error) {
	if len(items) == 1 {
		return items[0], nil
	}
	selected := 0
	var options []huh.Option[int]
	for i, item := range items {
		options = append(options, huh.NewOption(label(item), i))
		if preselect(item) {
			selected = i
		}
	}
	err := huh.NewForm(huh.NewGroup(huh.NewSelect[int]().
		Title(title).
		Options(options...).
		Height(min(len(options)+2, 15)).
		Value(&selected))).Run()
	return items[selected], formErr(err)
}

func formErr(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return errAborted
	}
	return err
}

// apply creates the API keys and stores the cluster in the configuration.
func (w *Wizard) apply(s setup) error {
	description := s.description
	if description == "" {
		description = KeyDescription(w.now())
	}

	fmt.Fprintf(w.out, "Creating API key %q for %s …\n", description, s.cluster.ID)
	key, err := w.cli.CreateAPIKey(s.env.ID, s.cluster.ID, description)
	if err != nil {
		return err
	}
	fmt.Fprintf(w.out, "Created API key %s\n", key.Key)

	details := config.RegistrationDetails{
		Name:       s.name,
		Color:      s.color,
		Host:       s.cluster.BootstrapServer(),
		AuthMethod: config.AuthMethodSASLPlaintext,
		Username:   key.Key,
		Password:   key.Secret,
		TLSConfig:  config.TLSConfig{Enable: true},
	}

	if s.sr != nil {
		fmt.Fprintf(w.out, "Creating API key %q for %s …\n", description, s.sr.ID)
		srKey, err := w.cli.CreateAPIKey(s.env.ID, s.sr.ID, description)
		if err != nil {
			fmt.Fprintf(w.out, "Skipping Schema Registry: %v\n", err)
		} else {
			fmt.Fprintf(w.out, "Created API key %s\n", srKey.Key)
			details.SchemaRegistry = &config.SchemaRegistryDetails{
				Url:      s.sr.EndpointURL,
				Username: srKey.Key,
				Password: srKey.Secret,
			}
		}
	}

	// keep the Kafka Connect clusters of an existing cluster with the same name
	if existing := w.cfg.FindClusterByName(s.name); existing != nil {
		for _, kc := range existing.KafkaConnectClusters {
			details.KafkaConnectClusters = append(details.KafkaConnectClusters, config.KafkaConnectClusterDetails{
				Name:     kc.Name,
				Url:      kc.Url,
				Username: kc.Username,
				Password: kc.Password,
			})
		}
	}

	w.cfg.RegisterCluster(details)
	if s.activate {
		w.cfg.SwitchCluster(s.name)
	}

	fmt.Fprintf(w.out, "Saved cluster %q, its connection settings and API keys are in %s (readable by you only)\n", s.name, config.ConnectionsPath())
	fmt.Fprintln(w.out, "New API keys can take a minute or two to become active.")
	return nil
}
