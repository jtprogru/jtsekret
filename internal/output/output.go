package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/jtprogru/jtsekret/internal/domain"
)

type OutputFormat string

const (
	FormatPlain OutputFormat = "plain"
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
	FormatAuto  OutputFormat = "auto"
)

type Outputter interface {
	PrintSecret(w io.Writer, secret *domain.Secret) error
	PrintSecretList(w io.Writer, secrets []domain.Secret) error
	PrintPayload(w io.Writer, payload *domain.Payload) error
	PrintEntry(w io.Writer, key string, value []byte) error
	PrintMessage(w io.Writer, msg string) error
}

func NewOutputter(format OutputFormat) Outputter {
	switch format {
	case FormatJSON:
		return &JSONOutputter{}
	case FormatTable:
		return &TableOutputter{}
	case FormatPlain:
		fallthrough
	default:
		return &PlainOutputter{}
	}
}

func DetectFormat() OutputFormat {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return FormatPlain
	}
	return FormatTable
}

type PlainOutputter struct{}

func (p *PlainOutputter) PrintSecret(w io.Writer, secret *domain.Secret) error {
	fmt.Fprintf(w, "Name: %s\n", secret.Name)
	fmt.Fprintf(w, "ID: %s\n", secret.ID)
	if secret.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", secret.Description)
	}
	if len(secret.Labels) > 0 {
		fmt.Fprintf(w, "Labels: %v\n", secret.Labels)
	}
	if len(secret.EntryKeys) > 0 {
		fmt.Fprintf(w, "Keys: %v\n", secret.EntryKeys)
	}
	return nil
}

func (p *PlainOutputter) PrintSecretList(w io.Writer, secrets []domain.Secret) error {
	for _, s := range secrets {
		fmt.Fprintf(w, "%s\t%s\t%v\n", s.Name, s.ID, s.EntryKeys)
	}
	return nil
}

func (p *PlainOutputter) PrintPayload(w io.Writer, payload *domain.Payload) error {
	for _, e := range payload.Entries {
		fmt.Fprintf(w, "%s=%s\n", e.Key, e.Value)
	}
	return nil
}

func (p *PlainOutputter) PrintEntry(w io.Writer, key string, value []byte) error {
	fmt.Fprintf(w, "%s", value)
	return nil
}

func (p *PlainOutputter) PrintMessage(w io.Writer, msg string) error {
	fmt.Fprintln(w, msg)
	return nil
}

type TableOutputter struct{}

func (t *TableOutputter) PrintSecret(w io.Writer, secret *domain.Secret) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	fmt.Fprintf(tw, "Name:\t%s\n", secret.Name)
	fmt.Fprintf(tw, "ID:\t%s\n", secret.ID)
	if secret.Description != "" {
		fmt.Fprintf(tw, "Description:\t%s\n", secret.Description)
	}
	if len(secret.Labels) > 0 {
		fmt.Fprintf(tw, "Labels:\t%v\n", secret.Labels)
	}
	if len(secret.EntryKeys) > 0 {
		fmt.Fprintf(tw, "Keys:\t%v\n", secret.EntryKeys)
	}
	return nil
}

func (t *TableOutputter) PrintSecretList(w io.Writer, secrets []domain.Secret) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	fmt.Fprintln(tw, "NAME\tID\tKEYS\tDESCRIPTION")
	for _, s := range secrets {
		desc := s.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%v\t%s\n", s.Name, s.ID, s.EntryKeys, desc)
	}
	return nil
}

func (t *TableOutputter) PrintPayload(w io.Writer, payload *domain.Payload) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	fmt.Fprintln(tw, "KEY\tVALUE")
	for _, e := range payload.Entries {
		fmt.Fprintf(tw, "%s\t%s\n", e.Key, e.Value)
	}
	return nil
}

func (t *TableOutputter) PrintEntry(w io.Writer, key string, value []byte) error {
	fmt.Fprintf(w, "%s", value)
	return nil
}

func (t *TableOutputter) PrintMessage(w io.Writer, msg string) error {
	fmt.Fprintln(w, msg)
	return nil
}

type JSONOutputter struct{}

func (j *JSONOutputter) PrintSecret(w io.Writer, secret *domain.Secret) error {
	return json.NewEncoder(w).Encode(secret)
}

func (j *JSONOutputter) PrintSecretList(w io.Writer, secrets []domain.Secret) error {
	return json.NewEncoder(w).Encode(secrets)
}

func (j *JSONOutputter) PrintPayload(w io.Writer, payload *domain.Payload) error {
	type payloadEntry struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	out := make([]payloadEntry, len(payload.Entries))
	for i, e := range payload.Entries {
		out[i] = payloadEntry{Key: e.Key, Value: string(e.Value)}
	}
	return json.NewEncoder(w).Encode(out)
}

func (j *JSONOutputter) PrintEntry(w io.Writer, key string, value []byte) error {
	return json.NewEncoder(w).Encode(map[string]string{key: string(value)})
}

func (j *JSONOutputter) PrintMessage(w io.Writer, msg string) error {
	return json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
