package sradmin

import (
	tea "charm.land/bubbletea/v2"
	"strconv"
)

type SchemaFetcher interface {
	GetSchemaById(id int) tea.Msg
}

type GettingSchemaByIdMsg struct {
	SchemaChan chan Schema
	ErrChan    chan error
}

type SchemaByIdReceived struct {
	Schema Schema
}

type FailedToFetchLatestSchemaBySubject struct {
	Err error
}

func (msg *GettingSchemaByIdMsg) AwaitCompletion() tea.Msg {
	select {
	case schema := <-msg.SchemaChan:
		return SchemaByIdReceived{
			Schema: schema,
		}
	case err := <-msg.ErrChan:
		return FailedToFetchLatestSchemaBySubject{Err: err}
	}
}

func (s *DefaultSrClient) GetSchemaById(id int) tea.Msg {
	if cached, ok := s.cache.schema(id); ok {
		return SchemaByIdReceived{Schema: Schema{
			Id:      strconv.Itoa(id),
			Value:   cached.Schema,
			Version: cached.Version,
		}}
	}
	schemaChan := make(chan Schema)
	errChan := make(chan error)

	go s.doGetSchemaById(id, schemaChan, errChan)
	return GettingSchemaByIdMsg{
		SchemaChan: schemaChan,
		ErrChan:    errChan,
	}
}

func (s *DefaultSrClient) doGetSchemaById(id int, schemaChan chan Schema, errChan chan error) {
	schema, err := s.client.GetSchema(id)
	if err != nil {
		errChan <- err
		return
	}
	s.cache.putSchema(id, schema.Schema(), schema.Version())
	schemaChan <- Schema{
		Id:      strconv.Itoa(schema.ID()),
		Value:   schema.Schema(),
		Version: schema.Version(),
		Err:     nil,
	}
}

// SubjectsOfSchema returns the subjects using the schema with the ID, in the order the registry
// lists them. The registry is only queried when the subjects are not cached yet.
func (s *DefaultSrClient) SubjectsOfSchema(id int) ([]string, error) {
	if subjects, ok := s.cache.subjects(id); ok {
		return subjects, nil
	}
	pairs, err := s.client.GetSubjectVersionsById(id)
	if err != nil {
		return nil, err
	}
	var subjects []string
	seen := map[string]bool{}
	for _, pair := range pairs {
		if !seen[pair.Subject] {
			seen[pair.Subject] = true
			subjects = append(subjects, pair.Subject)
		}
	}
	s.cache.putSubjects(id, subjects)
	return subjects, nil
}
