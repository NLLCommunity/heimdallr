package posts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/NLLCommunity/heimdallr/utils"
)

const DocumentVersion = 1

// Document is the versioned source format stored in Post.ComponentsJSON.
// Messages are explicit: once a document uses this format, validation never
// splits or combines them.
type Document struct {
	Version  int       `json:"version"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Components []any `json:"components"`
}

// NewDocument returns the canonical initial draft shown for a new post.
func NewDocument() Document {
	return Document{Version: DocumentVersion, Messages: []Message{{Components: []any{}}}}
}

// ParseDocument accepts the current document envelope and legacy component
// arrays. Legacy input is split with Plan once to derive explicit boundaries;
// envelope input preserves its message boundaries exactly.
func ParseDocument(raw string) (Document, error) {
	trimmed := bytes.TrimSpace([]byte(raw))
	if len(trimmed) == 0 {
		return Document{}, fmt.Errorf("invalid post document: empty input")
	}
	if trimmed[0] == '[' {
		var components []any
		if err := decodeStrict(trimmed, &components); err != nil {
			return Document{}, fmt.Errorf("invalid legacy components JSON: %w", err)
		}
		chunks, err := Plan(components)
		if err != nil {
			return Document{}, err
		}
		doc := Document{Version: DocumentVersion, Messages: make([]Message, len(chunks))}
		for i, chunk := range chunks {
			doc.Messages[i] = Message{Components: chunk}
		}
		if len(doc.Messages) == 0 {
			doc.Messages = []Message{{Components: []any{}}}
		}
		if err := doc.validateMessages(); err != nil {
			return Document{}, err
		}
		return doc, nil
	}

	type rawMessage struct {
		Components *json.RawMessage `json:"components"`
	}
	type rawDocument struct {
		Version  *int             `json:"version"`
		Messages *json.RawMessage `json:"messages"`
	}
	var envelope rawDocument
	if err := decodeStrict(trimmed, &envelope); err != nil {
		return Document{}, fmt.Errorf("invalid post document: %w", err)
	}
	if envelope.Version == nil || *envelope.Version != DocumentVersion {
		return Document{}, fmt.Errorf("unsupported or missing post document version")
	}
	if envelope.Messages == nil || bytes.Equal(bytes.TrimSpace(*envelope.Messages), []byte("null")) {
		return Document{}, fmt.Errorf("post document messages must be an array")
	}
	var rawMessages []rawMessage
	if err := decodeStrict(*envelope.Messages, &rawMessages); err != nil {
		return Document{}, fmt.Errorf("post document messages must be an array: %w", err)
	}
	doc := Document{Version: DocumentVersion, Messages: make([]Message, len(rawMessages))}
	for i, rawMessage := range rawMessages {
		if rawMessage.Components == nil || bytes.Equal(bytes.TrimSpace(*rawMessage.Components), []byte("null")) {
			return Document{}, fmt.Errorf("message %d components must be an array", i+1)
		}
		var components []any
		if err := decodeStrict(*rawMessage.Components, &components); err != nil {
			return Document{}, fmt.Errorf("message %d components must be an array: %w", i+1, err)
		}
		doc.Messages[i] = Message{Components: components}
	}
	if err := doc.validateMessages(); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func (d Document) validateMessages() error {
	for i, message := range d.Messages {
		chunks, err := Plan(message.Components)
		if err != nil {
			return fmt.Errorf("message %d: %w", i+1, err)
		}
		if len(chunks) > 1 {
			return fmt.Errorf("message %d does not fit in a single Discord message", i+1)
		}
		// The Discord parser normalizes some trees in place (notably nested
		// sections without accessories). Validate a detached JSON copy so merely
		// loading or saving a draft cannot rewrite editor-authored data.
		encoded, err := json.Marshal(message.Components)
		if err != nil {
			return fmt.Errorf("message %d components cannot be encoded: %w", i+1, err)
		}
		var validationCopy []any
		if err := json.Unmarshal(encoded, &validationCopy); err != nil {
			return fmt.Errorf("message %d components cannot be copied for validation: %w", i+1, err)
		}
		if err := utils.ValidateV2Components(validationCopy); err != nil {
			return fmt.Errorf("message %d components fail Discord schema check: %w", i+1, err)
		}
	}
	return nil
}

// PublishChunks returns explicit messages for the sync engine, rejecting
// draft-only empty states before any Discord operation starts.
func (d Document) PublishChunks() ([][]any, error) {
	if len(d.Messages) == 0 {
		return nil, fmt.Errorf("post has no content to publish")
	}
	chunks := make([][]any, len(d.Messages))
	for i, message := range d.Messages {
		if len(message.Components) == 0 {
			return nil, fmt.Errorf("post has no content to publish: message %d is empty", i+1)
		}
		chunks[i] = message.Components
	}
	return chunks, nil
}

func (d Document) JSON() (string, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("marshal post document: %w", err)
	}
	return string(b), nil
}

func decodeStrict(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
